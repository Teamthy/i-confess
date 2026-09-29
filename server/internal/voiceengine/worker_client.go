package voiceengine

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/Teamthy/i-confess/internal/voicedata"
)

// ---------------------------------------------------------------------------
// Intake and training: the non-synthesis half of the worker protocol.
//
//	POST /v1/ingest              <- IngestRequest  -> IngestResult (synchronous)
//	POST /v1/train               <- TrainRequest   -> 202
//	GET  /v1/train/{run_id}      -> TrainStatus
//	POST /v1/train/{run_id}/cancel
//
// Ingestion is long but bounded by recording length, so it is a single call
// with a generous timeout. Training runs for hours, so it is submitted and
// then polled by a delayed queue job; no HTTP request stays open that long.
// ---------------------------------------------------------------------------

// WorkerClient talks to an intake/training worker pool.
type WorkerClient struct {
	p *HTTPProvider
}

// NewWorkerClient returns a client for baseURL. timeout bounds one ingest call.
func NewWorkerClient(baseURL, token string, timeout time.Duration) *WorkerClient {
	if timeout == 0 {
		timeout = 30 * time.Minute
	}
	return &WorkerClient{p: NewHTTPProvider(HTTPProviderConfig{Engine: "worker", BaseURL: baseURL, Token: token, Timeout: timeout})}
}

// IngestRequest asks the worker to process one recording.
type IngestRequest struct {
	RecordingID string `json:"recording_id"`
	VoiceID     string `json:"voice_id"`
	// AudioKey is a private object key the worker resolves locally.
	AudioKey string `json:"audio_key"`
	Language string `json:"language"`
	// ReferenceKey, when set, is a verified clip of the licensed speaker used
	// to score speaker confidence for each segment.
	ReferenceKey string `json:"reference_key,omitempty"`
	// SegmentPrefix is where the worker writes per-segment audio.
	SegmentPrefix string `json:"segment_prefix"`
}

// IngestResult is the worker's report.
type IngestResult struct {
	DurationMS int                 `json:"duration_ms"`
	Segments   []voicedata.Segment `json:"segments"`
	// Report records which stages actually ran (vad, diarization, asr) and
	// with what backend, so a reviewer knows whether "speaker_confidence"
	// is a measurement or absent.
	Report json.RawMessage `json:"report"`
}

// Ingest runs VAD, cleaning, speaker scoring, ASR and quality scoring.
func (c *WorkerClient) Ingest(ctx context.Context, req IngestRequest) (*IngestResult, error) {
	resp, err := c.p.do(ctx, http.MethodPost, "/v1/ingest", req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, c.p.decodeError(resp)
	}
	var out IngestResult
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(&out); err != nil {
		return nil, &Error{Class: ClassTransient, Engine: "worker", Msg: "decode ingest result: " + err.Error()}
	}
	return &out, nil
}

// TrainRequest submits a fine-tuning run. Manifest entries carry private
// object keys; the worker must already have (or fetch) that audio.
type TrainRequest struct {
	RunID           string                    `json:"run_id"`
	VoiceID         string                    `json:"voice_id"`
	Engine          string                    `json:"engine"`
	BaseModel       string                    `json:"base_model"`
	DatasetVersion  string                    `json:"dataset_version"`
	ManifestSHA256  string                    `json:"manifest_sha256"`
	Entries         []voicedata.ManifestEntry `json:"entries"`
	Hyperparameters json.RawMessage           `json:"hyperparameters"`
	CheckpointKey   string                    `json:"checkpoint_key"`
}

// TrainStatus is the worker's view of a run.
type TrainStatus struct {
	Status           string   `json:"status"` // RUNNING | COMPLETED | FAILED
	Progress         *float64 `json:"progress"`
	Loss             *float64 `json:"loss"`
	CheckpointKey    string   `json:"checkpoint_key"`
	CheckpointSHA256 string   `json:"checkpoint_sha256"`
	Hardware         string   `json:"hardware"`
	DurationSeconds  int      `json:"duration_seconds"`
	Error            string   `json:"error"`
}

// StartTraining submits a run. Submitting the same run twice is idempotent
// on the worker.
func (c *WorkerClient) StartTraining(ctx context.Context, req TrainRequest) error {
	resp, err := c.p.do(ctx, http.MethodPost, "/v1/train", req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		return c.p.decodeError(resp)
	}
	return nil
}

// TrainingStatus polls a run.
func (c *WorkerClient) TrainingStatus(ctx context.Context, runID string) (*TrainStatus, error) {
	resp, err := c.p.do(ctx, http.MethodGet, "/v1/train/"+url.PathEscape(runID), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, c.p.decodeError(resp)
	}
	var st TrainStatus
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		return nil, &Error{Class: ClassTransient, Engine: "worker", Msg: err.Error()}
	}
	return &st, nil
}

// CancelTraining asks the worker to stop a run.
func (c *WorkerClient) CancelTraining(ctx context.Context, runID string) error {
	resp, err := c.p.do(ctx, http.MethodPost, "/v1/train/"+url.PathEscape(runID)+"/cancel", map[string]any{})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusNotFound {
		return c.p.decodeError(resp)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Streaming synthesis.
// ---------------------------------------------------------------------------

// Stream is an in-progress streamed render. Body yields a WAV stream whose
// header declares an unknown length; close it to stop inference.
type Stream struct {
	Body        io.ReadCloser
	ContentType string
	SampleRate  int
	Engine      Engine
	Model       Model
}

// StreamingProvider is implemented by providers whose worker can stream.
type StreamingProvider interface {
	GenerateStream(ctx context.Context, req GenerateRequest) (*Stream, error)
}

// GenerateStream implements StreamingProvider. It uses a client without an
// overall timeout (a long passage legitimately streams for minutes); the
// caller's context bounds it and cancels inference on disconnect.
func (p *HTTPProvider) GenerateStream(ctx context.Context, req GenerateRequest) (*Stream, error) {
	req.Params = p.mapper(req.Prosody, req.Style)
	resp, err := p.doWith(ctx, p.streamClient, http.MethodPost, "/v1/synthesize/stream", req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, p.decodeError(resp)
	}
	sr, _ := strconv.Atoi(resp.Header.Get("X-Sample-Rate"))
	return &Stream{Body: resp.Body, ContentType: resp.Header.Get("Content-Type"), SampleRate: sr, Engine: p.engine}, nil
}

// EncodedVariant is one delivery encoding returned by the worker.
type EncodedVariant struct {
	Data        []byte
	SHA256      string
	ContentType string
	Ext         string
}

// Encode asks the worker to derive delivery encodings (aac, opus, mp3) from a
// WAV master. The bytes travel both ways, so the worker needs no access to
// the API's object store; the caller uploads the results.
func (c *WorkerClient) Encode(ctx context.Context, master []byte, formats []string) (map[string]EncodedVariant, error) {
	resp, err := c.p.do(ctx, http.MethodPost, "/v1/encode", map[string]any{
		"audio_b64": base64.StdEncoding.EncodeToString(master), "formats": formats})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, c.p.decodeError(resp)
	}
	var out struct {
		Variants map[string]struct {
			B64         string `json:"b64"`
			SHA256      string `json:"sha256"`
			ContentType string `json:"content_type"`
			Ext         string `json:"ext"`
		} `json:"variants"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 256<<20)).Decode(&out); err != nil {
		return nil, &Error{Class: ClassTransient, Engine: "worker", Msg: "decode encode result: " + err.Error()}
	}
	res := make(map[string]EncodedVariant, len(out.Variants))
	for f, v := range out.Variants {
		data, err := base64.StdEncoding.DecodeString(v.B64)
		if err != nil {
			return nil, &Error{Class: ClassPermanent, Engine: "worker", Msg: "variant " + f + " is not base64"}
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != v.SHA256 {
			return nil, &Error{Class: ClassTransient, Engine: "worker", Msg: "variant " + f + " checksum mismatch"}
		}
		res[f] = EncodedVariant{Data: data, SHA256: v.SHA256, ContentType: v.ContentType, Ext: v.Ext}
	}
	return res, nil
}
