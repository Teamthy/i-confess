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

	"github.com/Teamthy/i-confess/internal/storage"
	"github.com/Teamthy/i-confess/internal/voicegov"
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
	// selfHosted says where this provider claims to run. It defaults to false -
	// a hosted API - because that is where the platform's real providers run
	// and it is the case that carries the extra capability.
	selfHosted bool
}

func (s *spyProvider) Name() string     { return "spy" }
func (s *spyProvider) SelfHosted() bool { return s.selfHosted }
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

// grant is a voicegov grant carrying exactly what the happy path needs: the
// generation capabilities, the confession purpose, commercial use, and - since
// the spy stands in for a hosted API - third-party infrastructure.
func grant() *voicegov.Grant {
	return &voicegov.Grant{
		VoiceID: "voice-1", Status: voicegov.StatusApproved, Version: 7,
		Capabilities: map[voicegov.Capability]bool{
			voicegov.CanClone:                true,
			voicegov.CanGenerate:             true,
			voicegov.CanCommercialize:        true,
			voicegov.CanUseInConfessions:     true,
			voicegov.CanUseThirdPartyInfra:   true,
			voicegov.CanUseUserSubmittedText: true,
		},
	}
}

func req() GenerateRequest {
	return GenerateRequest{
		ConfessionID: "conf-1", VariantID: "var-1", VoiceID: "voice-1",
		ProviderVoiceID: "el_pastor_a", Purpose: voicegov.PurposeConfession,
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
func TestGenerateRefusesWithoutGenerationCapability(t *testing.T) {
	spy := &spyProvider{}
	p, audit := newPipeline(t, spy)

	g := grant()
	g.Capabilities[voicegov.CanGenerate] = false

	_, err := p.Generate(context.Background(), g, req())
	if err == nil {
		t.Fatal("generation allowed without the generation capability")
	}
	var denied *ErrRightsDenied
	if !errors.As(err, &denied) {
		t.Fatalf("error type = %T, want *ErrRightsDenied", err)
	}
	if denied.Decision.Reason != voicegov.ReasonMissingCapability {
		t.Fatalf("reason = %q", denied.Decision.Reason)
	}
	// The refusal names the whole gap, so an operator can fix the licence in
	// one pass instead of discovering the missing clauses one request at a time.
	if !containsCap(denied.Decision.Missing, voicegov.CanGenerate) {
		t.Fatalf("missing = %v, want it to name %s", denied.Decision.Missing, voicegov.CanGenerate)
	}
	// The critical assertion: the provider was never contacted.
	if spy.calls != 0 {
		t.Fatalf("provider was called %d times despite refusal", spy.calls)
	}
	// The refusal must still be audited.
	if len(audit.entries) != 1 || audit.entries[0].Allowed {
		t.Fatalf("refusal not audited: %+v", audit.entries)
	}
	if audit.entries[0].Reason != string(voicegov.ReasonMissingCapability) {
		t.Fatalf("audit reason = %q", audit.entries[0].Reason)
	}
}

// TestHostedProviderNeedsThirdPartyCapability is the audit's VE-001 case, at
// the layer the cloud path enforces it: a grant that permits generating in this
// voice does not permit generating it on somebody else's servers.
func TestHostedProviderNeedsThirdPartyCapability(t *testing.T) {
	spy := &spyProvider{}
	p, _ := newPipeline(t, spy)

	g := grant()
	g.Capabilities[voicegov.CanUseThirdPartyInfra] = false

	_, err := p.Generate(context.Background(), g, req())
	var denied *ErrRightsDenied
	if !errors.As(err, &denied) {
		t.Fatalf("a hosted render without the third-party capability was not refused: %v", err)
	}
	if !containsCap(denied.Decision.Missing, voicegov.CanUseThirdPartyInfra) {
		t.Fatalf("missing = %v, want %s", denied.Decision.Missing, voicegov.CanUseThirdPartyInfra)
	}
	if spy.calls != 0 {
		t.Fatalf("provider was called %d times despite the refusal", spy.calls)
	}

	// The same grant, same request, on infrastructure the platform operates:
	// nothing is sent to a third party, so the capability is not needed.
	selfHosted := &spyProvider{selfHosted: true}
	p2, _ := newPipeline(t, selfHosted)
	if _, err := p2.Generate(context.Background(), g, req()); err != nil {
		t.Fatalf("self-hosted render refused for a third-party capability it does not use: %v", err)
	}
	if selfHosted.calls != 1 {
		t.Fatalf("self-hosted provider calls = %d, want 1", selfHosted.calls)
	}
}

func TestGenerateRefusesForEveryRightsFailure(t *testing.T) {
	past := testNow.Add(-time.Hour)
	future := testNow.Add(time.Hour)
	cases := []struct {
		name    string
		mutate  func(*voicegov.Grant)
		mutateR func(*GenerateRequest)
		reason  voicegov.Reason
	}{
		{"no grant at all", nil, nil, voicegov.ReasonNoGrant},
		{"revoked", func(g *voicegov.Grant) { g.Status = voicegov.StatusRevoked }, nil, voicegov.ReasonRevoked},
		{"suspended", func(g *voicegov.Grant) { g.Status = voicegov.StatusSuspended }, nil, voicegov.ReasonSuspended},
		{"pending", func(g *voicegov.Grant) { g.Status = voicegov.StatusPending }, nil, voicegov.ReasonNotApproved},
		{"expired", func(g *voicegov.Grant) { g.ExpiresAt = &past }, nil, voicegov.ReasonExpired},
		{"not yet effective", func(g *voicegov.Grant) { g.EffectiveFrom = &future }, nil, voicegov.ReasonNotYetEffective},
		{"non-commercial", func(g *voicegov.Grant) { g.Capabilities[voicegov.CanCommercialize] = false }, nil, voicegov.ReasonMissingCapability},
		{"no clone", func(g *voicegov.Grant) { g.Capabilities[voicegov.CanClone] = false }, nil, voicegov.ReasonMissingCapability},
		{"wrong purpose", func(g *voicegov.Grant) { g.Capabilities[voicegov.CanUseInConfessions] = false }, nil, voicegov.ReasonMissingCapability},
		{"marketing purpose", func(g *voicegov.Grant) { g.Capabilities[voicegov.CanUseInMarketing] = false },
			func(r *GenerateRequest) { r.Purpose = voicegov.PurposeMarketing }, voicegov.ReasonMissingCapability},
		{"user content without grant", func(g *voicegov.Grant) { g.Capabilities[voicegov.CanUseUserSubmittedText] = false },
			func(r *GenerateRequest) { r.UserSubmittedText = true }, voicegov.ReasonMissingCapability},
		{"restricted purpose", func(g *voicegov.Grant) {
			g.Status = voicegov.StatusRestricted
			g.Restrictions = []voicegov.ContentPurpose{voicegov.PurposeConfession}
		}, nil, voicegov.ReasonRestricted},
		{"territory excluded", func(g *voicegov.Grant) { g.Territories = []string{"NG"} },
			func(r *GenerateRequest) { r.Territory = "US" }, voicegov.ReasonTerritory},
		{"language excluded", func(g *voicegov.Grant) { g.Languages = []string{"de"} }, nil, voicegov.ReasonLanguage},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spy := &spyProvider{}
			p, _ := newPipeline(t, spy)

			var g *voicegov.Grant
			if tc.mutate != nil {
				g = grant()
				tc.mutate(g)
			}
			r := req()
			if tc.mutateR != nil {
				tc.mutateR(&r)
			}

			_, err := p.Generate(context.Background(), g, r)
			if err == nil {
				t.Fatal("expected refusal")
			}
			var denied *ErrRightsDenied
			if !errors.As(err, &denied) {
				t.Fatalf("error type = %T, want *ErrRightsDenied", err)
			}
			if denied.Decision.Reason != tc.reason {
				t.Fatalf("reason = %q, want %q (%s)", denied.Decision.Reason, tc.reason, denied.Decision.Detail)
			}
			if spy.calls != 0 {
				t.Fatalf("provider called %d times despite a rights refusal", spy.calls)
			}
		})
	}
}

// The provider id is configuration, not authority: a render cannot silently
// succeed against the wrong voice because the id was missing, and the refusal
// is permanent rather than a retry that will fail identically.
func TestGenerateRequiresAProviderVoiceID(t *testing.T) {
	spy := &spyProvider{}
	p, _ := newPipeline(t, spy)

	r := req()
	r.ProviderVoiceID = ""
	_, err := p.Generate(context.Background(), grant(), r)
	if err == nil {
		t.Fatal("a render with no provider voice id was allowed")
	}
	if !errors.Is(err, ErrPermanent) {
		t.Fatalf("error = %v, want a permanent one", err)
	}
	if spy.calls != 0 {
		t.Fatalf("provider was called %d times without a voice id", spy.calls)
	}
}

func containsCap(caps []voicegov.Capability, want voicegov.Capability) bool {
	for _, c := range caps {
		if c == want {
			return true
		}
	}
	return false
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
	if _, err := p.Generate(context.Background(), grant(), other); err != nil {
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
