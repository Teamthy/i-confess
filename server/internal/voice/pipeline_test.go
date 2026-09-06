package voice

import (
	"context"
	"encoding/binary"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/rights"
	"github.com/Teamthy/i-confess/internal/storage"
)

var testNow = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

// spyProvider records whether it was ever called. The central safety property
// of this package is that an unauthorized request never reaches a provider, so
// most tests assert on `calls == 0`.
type spyProvider struct {
	calls  int
	lastID string
	result *SynthesisResult
	err    error
}

func (s *spyProvider) Name() string { return "spy" }
func (s *spyProvider) Synthesize(_ context.Context, req SynthesisRequest) (*SynthesisResult, error) {
	s.calls++
	s.lastID = req.ProviderVoiceID
	if s.err != nil {
		return nil, s.err
	}
	if s.result != nil {
		return s.result, nil
	}
	// Real audio, deliberately misreported as 30s when the payload is 3s. The
	// pipeline measures rather than trusts, so a stub that returned arbitrary
	// bytes would no longer represent a provider that worked - and a stub that
	// reported honestly would not prove the measurement is the one used.
	return &SynthesisResult{
		Audio: testWAV(3), ContentType: "audio/wav",
		Codec: "pcm", BitrateKbps: 128, SampleRate: 8000, DurationSeconds: 30,
	}, nil
}

// testWAV builds a genuine WAV payload. 8 kHz mono keeps the fixture small
// while still being parseable by the inspector.
func testWAV(seconds int) []byte {
	const sampleRate, channels, bits = 8000, 1, 16
	byteRate := uint32(sampleRate * channels * bits / 8)
	dataSize := byteRate * uint32(seconds)

	b := []byte("RIFF")
	b = binary.LittleEndian.AppendUint32(b, 4+24+8+dataSize)
	b = append(b, "WAVE"...)
	b = append(b, "fmt "...)
	b = binary.LittleEndian.AppendUint32(b, 16)
	b = binary.LittleEndian.AppendUint16(b, 1)
	b = binary.LittleEndian.AppendUint16(b, uint16(channels))
	b = binary.LittleEndian.AppendUint32(b, uint32(sampleRate))
	b = binary.LittleEndian.AppendUint32(b, byteRate)
	b = binary.LittleEndian.AppendUint16(b, uint16(channels*bits/8))
	b = binary.LittleEndian.AppendUint16(b, bits)
	b = append(b, "data"...)
	b = binary.LittleEndian.AppendUint32(b, dataSize)
	return append(b, make([]byte, dataSize)...)
}

type memAudit struct{ entries []AuditEntry }

func (m *memAudit) RecordGeneration(_ context.Context, e AuditEntry) {
	m.entries = append(m.entries, e)
}

func newPipeline(t *testing.T, p Provider) (*Pipeline, *memAudit) {
	t.Helper()
	store, err := storage.NewLocalStorage(&storage.StorageConfig{
		LocalRootPath: t.TempDir(), CDNDomain: "/media", SigningSecret: "sec",
	})
	if err != nil {
		t.Fatal(err)
	}
	audit := &memAudit{}
	return &Pipeline{Provider: p, Store: store, Audit: audit, Now: func() time.Time { return testNow }}, audit
}

func grant() *rights.License {
	exp := testNow.Add(365 * 24 * time.Hour)
	return &rights.License{
		VoiceID: "voice-1", Status: rights.StatusActive, Expiry: &exp,
		CommercialUse: true, AIGenerationAllowed: true, UserContentAllowed: true,
		ProviderVoiceID: "el_pastor_a",
	}
}

func req() GenerateRequest {
	return GenerateRequest{
		ConfessionID: "conf-1", VariantID: "var-1", VoiceID: "voice-1",
		Language: "en", Text: "I am healed by His stripes.", RequestedBy: "admin-1",
	}
}

func TestGenerateHappyPath(t *testing.T) {
	spy := &spyProvider{}
	p, audit := newPipeline(t, spy)

	res, err := p.Generate(context.Background(), grant(), req())
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if spy.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", spy.calls)
	}
	// The provider id must come from the licence, never from caller input.
	if spy.lastID != "el_pastor_a" {
		t.Fatalf("provider voice id = %q, want the one on the licence", spy.lastID)
	}
	if res.Reused {
		t.Fatal("first generation should not be a reuse")
	}
	if res.Key == "" || res.Checksum == "" || res.SizeBytes == 0 {
		t.Fatalf("incomplete result: %+v", res)
	}
	if len(audit.entries) != 1 || !audit.entries[0].Allowed || audit.entries[0].Reason != "generated" {
		t.Fatalf("unexpected audit trail: %+v", audit.entries)
	}
}

// The load-bearing test for §13: no rights, no network call.
func TestGenerateRefusesWithoutAIGrant(t *testing.T) {
	spy := &spyProvider{}
	p, audit := newPipeline(t, spy)

	lic := grant()
	lic.AIGenerationAllowed = false

	_, err := p.Generate(context.Background(), lic, req())
	if err == nil {
		t.Fatal("generation allowed without an AI grant")
	}
	var denied *ErrRightsDenied
	if !errors.As(err, &denied) {
		t.Fatalf("error type = %T, want *ErrRightsDenied", err)
	}
	if denied.Decision.Reason != rights.ReasonNoAIGeneration {
		t.Fatalf("reason = %q", denied.Decision.Reason)
	}
	// The critical assertion: the provider was never contacted.
	if spy.calls != 0 {
		t.Fatalf("provider was called %d times despite refusal", spy.calls)
	}
	// The refusal must still be audited.
	if len(audit.entries) != 1 || audit.entries[0].Allowed {
		t.Fatalf("refusal not audited: %+v", audit.entries)
	}
	if audit.entries[0].Reason != string(rights.ReasonNoAIGeneration) {
		t.Fatalf("audit reason = %q", audit.entries[0].Reason)
	}
}

func TestGenerateRefusesForEveryRightsFailure(t *testing.T) {
	past := testNow.Add(-time.Hour)
	cases := []struct {
		name   string
		mutate func(*rights.License)
		use    rights.Use
	}{
		{"nil licence", nil, rights.UseSynthesis},
		{"revoked", func(l *rights.License) { l.Status = rights.StatusRevoked }, rights.UseSynthesis},
		{"expired", func(l *rights.License) { l.Expiry = &past }, rights.UseSynthesis},
		{"non-commercial", func(l *rights.License) { l.CommercialUse = false }, rights.UseSynthesis},
		{"no provider voice id", func(l *rights.License) { l.ProviderVoiceID = "" }, rights.UseSynthesis},
		{"user content without grant", func(l *rights.License) { l.UserContentAllowed = false }, rights.UseUserContent},
		{"territory excluded", func(l *rights.License) { l.Territories = []string{"NG"} }, rights.UseSynthesis},
		{"language excluded", func(l *rights.License) { l.Languages = []string{"de"} }, rights.UseSynthesis},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spy := &spyProvider{}
			p, _ := newPipeline(t, spy)

			var lic *rights.License
			if tc.mutate != nil {
				lic = grant()
				tc.mutate(lic)
			}
			r := req()
			r.Use = tc.use
			if tc.name == "territory excluded" {
				r.Territory = "US"
			}

			if _, err := p.Generate(context.Background(), lic, r); err == nil {
				t.Fatal("expected refusal")
			}
			if spy.calls != 0 {
				t.Fatalf("provider called %d times despite a rights refusal", spy.calls)
			}
		})
	}
}

// Never pay twice for identical audio (§60).
func TestGenerateDedupesExistingAsset(t *testing.T) {
	spy := &spyProvider{}
	p, _ := newPipeline(t, spy)

	if _, err := p.Generate(context.Background(), grant(), req()); err != nil {
		t.Fatal(err)
	}
	res, err := p.Generate(context.Background(), grant(), req())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Reused {
		t.Fatal("second identical request should reuse the stored asset")
	}
	if spy.calls != 1 {
		t.Fatalf("provider calls = %d, want 1 (second request must not hit the provider)", spy.calls)
	}
}

func TestGenerateForceBypassesDedupe(t *testing.T) {
	spy := &spyProvider{}
	p, _ := newPipeline(t, spy)

	if _, err := p.Generate(context.Background(), grant(), req()); err != nil {
		t.Fatal(err)
	}
	r := req()
	r.Force = true
	if _, err := p.Generate(context.Background(), grant(), r); err != nil {
		t.Fatal(err)
	}
	if spy.calls != 2 {
		t.Fatalf("provider calls = %d, want 2 for a forced re-render", spy.calls)
	}
}

// A different voice or language is different audio and must not be deduped.
func TestGenerateDoesNotDedupeAcrossIdentity(t *testing.T) {
	spy := &spyProvider{}
	p, _ := newPipeline(t, spy)

	if _, err := p.Generate(context.Background(), grant(), req()); err != nil {
		t.Fatal(err)
	}
	other := req()
	other.Language = "de"
	lic := grant()
	if _, err := p.Generate(context.Background(), lic, other); err != nil {
		t.Fatal(err)
	}
	if spy.calls != 2 {
		t.Fatalf("provider calls = %d, want 2 across differing languages", spy.calls)
	}
}

func TestGenerateSurfacesProviderErrors(t *testing.T) {
	spy := &spyProvider{err: RetryableError("upstream 503")}
	p, audit := newPipeline(t, spy)

	_, err := p.Generate(context.Background(), grant(), req())
	if err == nil {
		t.Fatal("expected provider error")
	}
	if !IsRetryable(err) {
		t.Fatalf("error should be retryable: %v", err)
	}
	if len(audit.entries) != 1 || audit.entries[0].Reason != "provider_error" {
		t.Fatalf("provider failure not audited: %+v", audit.entries)
	}
}

func TestBackoffGrowsAndCaps(t *testing.T) {
	if Backoff(1) != 30*time.Second {
		t.Fatalf("first backoff = %v", Backoff(1))
	}
	if Backoff(2) <= Backoff(1) {
		t.Fatal("backoff should grow")
	}
	if Backoff(50) != 30*time.Minute {
		t.Fatalf("backoff should cap at 30m, got %v", Backoff(50))
	}
}

// ---------------------------------------------------------------------------
// ElevenLabs adapter
// ---------------------------------------------------------------------------

func TestElevenLabsSuccess(t *testing.T) {
	var gotKey, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("xi-api-key")
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Write(make([]byte, 16000))
	}))
	defer srv.Close()

	el := NewElevenLabs("secret-key")
	el.BaseURL = srv.URL

	res, err := el.Synthesize(context.Background(), SynthesisRequest{
		ProviderVoiceID: "el_voice_9", Text: "Peace be still.", Language: "en",
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotKey != "secret-key" {
		t.Fatalf("api key header = %q", gotKey)
	}
	if gotPath != "/v1/text-to-speech/el_voice_9" {
		t.Fatalf("path = %q", gotPath)
	}
	if res.Codec != "mp3" || res.BitrateKbps != 128 || res.SampleRate != 44100 {
		t.Fatalf("unexpected metadata: %+v", res)
	}
	if res.DurationSeconds != 1 {
		t.Fatalf("duration = %d, want 1s for 16kB at 128kbps", res.DurationSeconds)
	}
}

func TestElevenLabsErrorClassification(t *testing.T) {
	cases := []struct {
		status    int
		retryable bool
	}{
		{http.StatusTooManyRequests, true},
		{http.StatusInternalServerError, true},
		{http.StatusBadGateway, true},
		{http.StatusUnauthorized, false},
		{http.StatusForbidden, false},
		{http.StatusBadRequest, false},
	}
	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "upstream says no", tc.status)
		}))
		el := NewElevenLabs("k")
		el.BaseURL = srv.URL

		_, err := el.Synthesize(context.Background(), SynthesisRequest{ProviderVoiceID: "v", Text: "x"})
		srv.Close()
		if err == nil {
			t.Fatalf("status %d: expected an error", tc.status)
		}
		if IsRetryable(err) != tc.retryable {
			t.Fatalf("status %d: retryable = %v, want %v (%v)", tc.status, IsRetryable(err), tc.retryable, err)
		}
	}
}

func TestElevenLabsRejectsBadInputWithoutCallingOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("provider must not be contacted for invalid input")
	}))
	defer srv.Close()

	el := NewElevenLabs("k")
	el.BaseURL = srv.URL

	if _, err := el.Synthesize(context.Background(), SynthesisRequest{ProviderVoiceID: "", Text: "x"}); err == nil {
		t.Fatal("empty voice id accepted")
	}
	if _, err := el.Synthesize(context.Background(), SynthesisRequest{ProviderVoiceID: "v", Text: "  "}); err == nil {
		t.Fatal("empty text accepted")
	}

	noKey := NewElevenLabs("")
	noKey.BaseURL = srv.URL
	if _, err := noKey.Synthesize(context.Background(), SynthesisRequest{ProviderVoiceID: "v", Text: "x"}); err == nil {
		t.Fatal("missing api key accepted")
	}
}

func TestElevenLabsRejectsEmptyAudio(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
	}))
	defer srv.Close()

	el := NewElevenLabs("k")
	el.BaseURL = srv.URL
	_, err := el.Synthesize(context.Background(), SynthesisRequest{ProviderVoiceID: "v", Text: "x"})
	if err == nil {
		t.Fatal("empty audio body accepted")
	}
	if !IsRetryable(err) {
		t.Fatal("an empty body is a transient upstream fault and should be retryable")
	}
}

// TestDurationIsMeasuredNotTakenFromTheProvider covers the PHASE 13 fix. The
// stub reports 30s for a payload that is a 3-second WAV. duration_seconds is
// what the session planner schedules every queue item against, so the
// container's figure must win over the provider's word.
func TestDurationIsMeasuredNotTakenFromTheProvider(t *testing.T) {
	spy := &spyProvider{}
	p, _ := newPipeline(t, spy)

	res, err := p.Generate(context.Background(), grant(), req())
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if res.DurationSeconds != 3 {
		t.Errorf("DurationSeconds = %d, want 3 (measured from the container), not the provider's claim of 30",
			res.DurationSeconds)
	}
	if res.SampleRate != 8000 {
		t.Errorf("SampleRate = %d, want 8000 from the WAV header", res.SampleRate)
	}
}

// TestGenerateRejectsAudioItCannotVerify proves the boundary fails closed: a
// provider that returns something that is not audio must not have it stored,
// billed for, and scheduled into a user's session.
func TestGenerateRejectsAudioItCannotVerify(t *testing.T) {
	spy := &spyProvider{result: &SynthesisResult{
		Audio: []byte("not audio at all, just bytes"), ContentType: "audio/mpeg",
		Codec: "mp3", DurationSeconds: 30,
	}}
	p, audit := newPipeline(t, spy)

	_, err := p.Generate(context.Background(), grant(), req())
	if err == nil {
		t.Fatal("generate accepted a payload that is not audio")
	}
	if !strings.Contains(err.Error(), "verify generated audio") {
		t.Errorf("error should name the verification step, got: %v", err)
	}
	if len(audit.entries) != 1 || audit.entries[0].Reason != "rejected" {
		t.Fatalf("unexpected audit trail: %+v", audit.entries)
	}
	if !audit.entries[0].Allowed {
		t.Error("the rights decision was sound; only the payload was bad")
	}
}
