package voiceengine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPRenderProofCannotBeInferred(t *testing.T) {
	pin := WorkerIdentity{EngineCosyVoice, testRevision}
	for _, tc := range []struct {
		name            string
		engine, version []string
	}{
		{"other family", []string{"voxcpm"}, []string{testRevision}},
		{"legacy combined header", nil, []string{"cosyvoice-" + testRevision}},
		{"missing revision", []string{"cosyvoice"}, nil},
		{"ambiguous revision", []string{"cosyvoice"}, []string{testRevision, "other"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req GenerateRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ExpectedWorker != pin {
					t.Error("no model precondition on wire")
				}
				for _, v := range tc.engine {
					w.Header().Add("X-Engine", v)
				}
				for _, v := range tc.version {
					w.Header().Add("X-Engine-Version", v)
				}
				w.Write([]byte("RIFF-wrong"))
			}))
			t.Cleanup(srv.Close)
			p := NewCosyVoiceProvider(srv.URL, "test")
			req := GenerateRequest{ExpectedWorker: pin}
			if result, err := p.Generate(context.Background(), req); result != nil || Classify(err) != ClassIdentity {
				t.Fatalf("accepted render: %v", err)
			}
			if stream, err := p.GenerateStream(context.Background(), req); stream != nil || Classify(err) != ClassIdentity {
				t.Fatalf("accepted stream: %v", err)
			}
		})
	}
}

func TestWorkerMismatchDoesNotSubstitute(t *testing.T) {
	o, primary, fallback, models, refs := setup()
	primary.caps.EngineVersion = "" // even an approved fallback must not hide a misrouted worker
	_, _, err := o.Generate(context.Background(), approvedGrant(), models, refs, req())
	if Classify(err) != ClassIdentity || primary.calls+fallback.calls != 0 {
		t.Fatalf("missing identity: err=%v calls=%d/%d", err, primary.calls, fallback.calls)
	}
}

func TestPinnedPlanCannotReuseLegacyOrChangedKeys(t *testing.T) {
	// Simulate a pre-verification row whose labels already happened to match.
	input := HashInput{Engine: EngineCosyVoice, EngineVersion: testRevision, AudioVersion: AudioPipelineVersion, Language: "en", Text: "Peace."}
	legacyJSON, _ := json.Marshal(input)
	old := sha256.Sum256(legacyJSON)
	if ContentHash(input) == hex.EncodeToString(old[:]) {
		t.Fatal("pre-verification cache key reused")
	}
	for _, change := range []string{"owner", "dictionary", "selected reference", "lost reference"} {
		t.Run(change, func(t *testing.T) {
			ctx := context.Background()
			o, p, fb, models, refs := setup()
			r := req()
			r.CacheOwnerID = "owner-a"
			queued, err := o.Resolve(ctx, approvedGrant(), models, refs, r)
			if err != nil {
				t.Fatal(err)
			}
			r.QueuedContentHash = queued.ContentHash
			switch change {
			case "owner":
				r.CacheOwnerID = "owner-b"
			case "dictionary":
				o.DictVer = "next"
			case "selected reference":
				refs[0].ID = "new-reference"
			case "lost reference":
				refs = nil
			}
			if _, res, err := o.Generate(ctx, approvedGrant(), models, refs, r); Classify(err) != ClassIdentity || res != nil || p.calls+fb.calls != 0 {
				t.Fatalf("changed queued plan reached a worker: err=%v", err)
			}
		})
	}
}

// Non-HTTP providers implement the same contract: capabilities are not a
// receipt. Verify the master/stream before returning either to a caller.
type wrongResultProvider struct {
	*fakeProvider
	body *unreadStream
}

func (p wrongResultProvider) Generate(ctx context.Context, r GenerateRequest) (*GenerateResult, error) {
	res, err := p.fakeProvider.Generate(ctx, r)
	if err == nil {
		res.Engine = EngineDevTone
	}
	return res, err
}

func (p wrongResultProvider) GenerateStream(context.Context, GenerateRequest) (*Stream, error) {
	return &Stream{Body: p.body, Engine: EngineCosyVoice, EngineVersion: ""}, nil
}

type unreadStream struct{ closed bool }

func (*unreadStream) Read([]byte) (int, error) { panic("read a stream before verifying its worker") }
func (s *unreadStream) Close() error           { s.closed = true; return nil }

func TestOrchestratorRejectsUnverifiedResults(t *testing.T) {
	ctx := context.Background()
	o, p, fb, models, refs := setup()
	p.caps.Streaming = true
	wrong := wrongResultProvider{p, &unreadStream{}}
	if err := o.Registry.Register(EngineCosyVoice, wrong); err != nil {
		t.Fatal(err)
	}
	if _, res, err := o.Generate(ctx, approvedGrant(), models, refs, req()); Classify(err) != ClassIdentity || res != nil || fb.calls != 0 {
		t.Fatalf("accepted or substituted on wrong render family: %v", err)
	}
	if _, st, err := o.Stream(ctx, approvedGrant(), models, refs, req()); Classify(err) != ClassIdentity || st != nil || !wrong.body.closed {
		t.Fatalf("handed off invalid stream: %v", err)
	}
}

func TestWorkerPinFormat(t *testing.T) {
	for _, pin := range []WorkerIdentity{
		{"", testRevision}, {"cosyvoice, voxcpm", testRevision},
		{EngineCosyVoice, "dev"}, {EngineCosyVoice, "3.1"}, {EngineDevTone, testRevision},
		{EngineCosyVoice, "v2+sha256." + strings.Repeat("a", 63)},
	} {
		if pin.Valid() {
			t.Fatalf("invalid pin passed: %+v", pin)
		}
	}
}
