// Package voicedata holds the pure rules for turning licensed recordings into
// training data: intake quality screening, deterministic dataset splits,
// immutable manifests, fine-tune readiness, and training-run transitions.
//
// It has no I/O. Nothing here approves a transcript or a segment: automated
// screening can only reject or flag. A human verifies every transcript that
// enters a dataset.
package voicedata

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// SourceType is where a recording came from. There is deliberately no
// "scraped" or "youtube" type: platform audio is never downloaded; a source
// URL is provenance only, and the file itself must be supplied by the
// rights holder.
type SourceType string

const (
	SourceUpload            SourceType = "upload"
	SourceAuthorizedArchive SourceType = "authorized_archive"
	SourceStudioSession     SourceType = "studio_session"
)

// ValidSource reports whether s is an accepted source type.
func ValidSource(s SourceType) bool {
	switch s {
	case SourceUpload, SourceAuthorizedArchive, SourceStudioSession:
		return true
	}
	return false
}

// Thresholds are the automated screening floors. They are configuration and
// should be tuned empirically per voice; the defaults are conservative
// starting points, not claims about what "good" data is.
type Thresholds struct {
	MinSegmentMS         int
	MaxSegmentMS         int
	MinSNRDB             float64
	MaxClippingRatio     float64
	MinQuality           float64
	MinSpeakerConfidence float64
}

// DefaultThresholds returns starting-point screening floors.
func DefaultThresholds() Thresholds {
	return Thresholds{MinSegmentMS: 1500, MaxSegmentMS: 15000, MinSNRDB: 20, MaxClippingRatio: 0.001,
		MinQuality: 0.6, MinSpeakerConfidence: 0.75}
}

// Segment is one candidate utterance produced by the ingestion worker.
type Segment struct {
	StartMS           int      `json:"start_ms"`
	EndMS             int      `json:"end_ms"`
	AudioKey          string   `json:"audio_key"`
	RawTranscript     string   `json:"raw_transcript"`
	ASRConfidence     *float64 `json:"asr_confidence"`
	SNRDB             *float64 `json:"snr_db"`
	ClippingRatio     float64  `json:"clipping_ratio"`
	SpeakerConfidence *float64 `json:"speaker_confidence"`
	Quality           float64  `json:"quality"`
}

// Screen returns the reasons a segment should be auto-rejected. An empty
// result means "eligible for human review", never "approved".
func Screen(s Segment, t Thresholds) []string {
	var r []string
	d := s.EndMS - s.StartMS
	if d < t.MinSegmentMS {
		r = append(r, fmt.Sprintf("too_short:%dms", d))
	}
	if t.MaxSegmentMS > 0 && d > t.MaxSegmentMS {
		r = append(r, fmt.Sprintf("too_long:%dms", d))
	}
	if s.SNRDB != nil && *s.SNRDB < t.MinSNRDB {
		r = append(r, fmt.Sprintf("low_snr:%.1fdB", *s.SNRDB))
	}
	if s.ClippingRatio > t.MaxClippingRatio {
		r = append(r, fmt.Sprintf("clipping:%.4f", s.ClippingRatio))
	}
	if s.Quality < t.MinQuality {
		r = append(r, fmt.Sprintf("low_quality:%.2f", s.Quality))
	}
	// Unknown speaker confidence (diarization not run) is not a rejection;
	// the reviewer must confirm the speaker by listening.
	if s.SpeakerConfidence != nil && *s.SpeakerConfidence < t.MinSpeakerConfidence {
		r = append(r, fmt.Sprintf("other_speaker:%.2f", *s.SpeakerConfidence))
	}
	return r
}

// Split is a dataset partition.
type Split string

const (
	SplitTrain      Split = "train"
	SplitValidation Split = "validation"
	SplitTest       Split = "test"
)

// AssignSplit deterministically partitions ids by hashing each id with the
// seed, so rebuilding a dataset from the same segments yields the same split
// and test utterances never leak into training across versions. Roughly
// 90/5/5; with ten or more ids at least one lands in each holdout.
func AssignSplit(ids []string, seed string) map[string]Split {
	type h struct {
		id  string
		key string
	}
	hs := make([]h, len(ids))
	for i, id := range ids {
		sum := sha256.Sum256([]byte(seed + "|" + id))
		hs[i] = h{id, hex.EncodeToString(sum[:])}
	}
	sort.Slice(hs, func(i, j int) bool { return hs[i].key < hs[j].key })
	out := make(map[string]Split, len(ids))
	n := len(hs)
	hold := n / 20
	if n >= 10 && hold == 0 {
		hold = 1
	}
	for i, x := range hs {
		switch {
		case i < hold:
			out[x.id] = SplitTest
		case i < 2*hold:
			out[x.id] = SplitValidation
		default:
			out[x.id] = SplitTrain
		}
	}
	return out
}

// ManifestEntry is one line of a frozen dataset.
type ManifestEntry struct {
	SegmentID   string  `json:"segment_id"`
	RecordingID string  `json:"recording_id"`
	AudioKey    string  `json:"audio_key"`
	Transcript  string  `json:"transcript"`
	Split       Split   `json:"split"`
	Style       string  `json:"style,omitempty"`
	DurationMS  int     `json:"duration_ms"`
	Quality     float64 `json:"quality"`
}

// Manifest is the immutable description of a dataset version.
type Manifest struct {
	VoiceID        string          `json:"voice_id"`
	DatasetVersion string          `json:"dataset_version"`
	GrantVersion   int             `json:"grant_version"`
	Entries        []ManifestEntry `json:"entries"`
}

// Encode returns canonical JSON and its SHA-256. Entries are sorted so the
// hash depends only on content.
func (m Manifest) Encode() ([]byte, string, error) {
	sort.Slice(m.Entries, func(i, j int) bool { return m.Entries[i].SegmentID < m.Entries[j].SegmentID })
	b, err := json.Marshal(m)
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(b)
	return b, hex.EncodeToString(sum[:]), nil
}

// Stats summarizes a manifest.
type Stats struct {
	Segments      int     `json:"segments"`
	UsableSeconds int     `json:"usableSeconds"`
	AvgQuality    float64 `json:"avgQuality"`
	TrainSegments int     `json:"trainSegments"`
	TestSegments  int     `json:"testSegments"`
}

// Summarize computes Stats.
func Summarize(entries []ManifestEntry) Stats {
	var s Stats
	var ms int
	var q float64
	for _, e := range entries {
		s.Segments++
		ms += e.DurationMS
		q += e.Quality
		switch e.Split {
		case SplitTrain:
			s.TrainSegments++
		case SplitTest:
			s.TestSegments++
		}
	}
	s.UsableSeconds = ms / 1000
	if s.Segments > 0 {
		s.AvgQuality = q / float64(s.Segments)
	}
	return s
}

// NextVersion returns dataset_v001-style names after the highest existing.
func NextVersion(existing []string) string {
	max := 0
	for _, v := range existing {
		var n int
		if _, err := fmt.Sscanf(v, "dataset_v%d", &n); err == nil && n > max {
			max = n
		}
	}
	return fmt.Sprintf("dataset_v%03d", max+1)
}

// FineTunePolicy encodes "zero-shot first, fine-tune only when needed".
type FineTunePolicy struct {
	// MinUsableSeconds is the smallest dataset worth fine-tuning on.
	MinUsableSeconds int
	// ZeroShotSufficientScore: if the best evaluated zero-shot model already
	// reaches this ICF_VOICE_SCORE, fine-tuning needs an explicit override.
	ZeroShotSufficientScore float64
}

// DefaultFineTunePolicy returns starting-point values (configurable).
func DefaultFineTunePolicy() FineTunePolicy {
	return FineTunePolicy{MinUsableSeconds: 600, ZeroShotSufficientScore: 0.85}
}

// ErrZeroShotFirst is returned when no zero-shot baseline has been evaluated.
var ErrZeroShotFirst = errors.New("evaluate a zero-shot model on the golden set before requesting fine-tuning")

// CheckFineTune decides whether a fine-tune request may proceed.
// bestZeroShot is nil when no zero-shot model has been evaluated.
func CheckFineTune(p FineTunePolicy, st Stats, bestZeroShot *float64, justification string, override bool) error {
	if bestZeroShot == nil {
		return ErrZeroShotFirst
	}
	if strings.TrimSpace(justification) == "" {
		return errors.New("a justification is required: say what the zero-shot model gets wrong")
	}
	if st.UsableSeconds < p.MinUsableSeconds {
		return fmt.Errorf("dataset has %ds of usable audio; policy minimum is %ds", st.UsableSeconds, p.MinUsableSeconds)
	}
	if st.TestSegments == 0 {
		return errors.New("dataset has no held-out test segments")
	}
	if *bestZeroShot >= p.ZeroShotSufficientScore && !override {
		return fmt.Errorf("zero-shot already scores %.3f (sufficient at %.3f); set override to fine-tune anyway", *bestZeroShot, p.ZeroShotSufficientScore)
	}
	return nil
}

// RunStatus is a training run state.
type RunStatus string

const (
	RunQueued     RunStatus = "QUEUED"
	RunRunning    RunStatus = "RUNNING"
	RunFailed     RunStatus = "FAILED"
	RunCompleted  RunStatus = "COMPLETED"
	RunEvaluating RunStatus = "EVALUATING"
	RunApproved   RunStatus = "APPROVED"
	RunRejected   RunStatus = "REJECTED"
	RunArchived   RunStatus = "ARCHIVED"
)

// RunStatuses must match the voice_training_runs CHECK constraint.
var RunStatuses = []RunStatus{RunQueued, RunRunning, RunFailed, RunCompleted, RunEvaluating, RunApproved, RunRejected, RunArchived}

var runEdges = map[RunStatus][]RunStatus{
	RunQueued:     {RunRunning, RunFailed, RunArchived},
	RunRunning:    {RunCompleted, RunFailed, RunArchived},
	RunCompleted:  {RunEvaluating, RunArchived},
	RunEvaluating: {RunApproved, RunRejected},
	RunApproved:   {RunArchived},
	RunRejected:   {RunArchived},
	RunFailed:     {RunArchived},
}

// CanTransitionRun reports whether from→to is allowed.
func CanTransitionRun(from, to RunStatus) bool {
	for _, t := range runEdges[from] {
		if t == to {
			return true
		}
	}
	return false
}
