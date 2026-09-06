package voice

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/Teamthy/i-confess/internal/audio"
	"github.com/Teamthy/i-confess/internal/rights"
	"github.com/Teamthy/i-confess/internal/storage"
)

// ErrRightsDenied is returned when a generation is refused on rights grounds.
// It wraps the machine-readable decision so callers can report the exact reason
// rather than a generic failure.
type ErrRightsDenied struct {
	Decision rights.Decision
}

func (e *ErrRightsDenied) Error() string {
	return fmt.Sprintf("voice rights denied (%s): %s", e.Decision.Reason, e.Decision.Detail)
}

// AuditSink records generation decisions (§36, §61). Attempts are audited
// whether they succeed or are refused, so a rights dispute can be reconstructed
// after the fact.
type AuditSink interface {
	RecordGeneration(ctx context.Context, entry AuditEntry)
}

// AuditEntry is one generation decision.
type AuditEntry struct {
	At           time.Time
	VoiceID      string
	ConfessionID string
	VariantID    string
	Language     string
	Provider     string
	Allowed      bool
	Reason       string
	Detail       string
	AssetKey     string
	RequestedBy  string
}

// LogAudit is a default AuditSink writing structured lines. Production swaps in
// a database-backed sink.
type LogAudit struct{}

// RecordGeneration implements AuditSink.
func (LogAudit) RecordGeneration(_ context.Context, e AuditEntry) {
	log.Printf("audit generation voice=%s confession=%s variant=%s lang=%s provider=%s allowed=%t reason=%s key=%s by=%s",
		e.VoiceID, e.ConfessionID, e.VariantID, e.Language, e.Provider, e.Allowed, e.Reason, e.AssetKey, e.RequestedBy)
}

// Pipeline turns approved text into a stored, addressable audio asset.
//
// It is the only sanctioned route to a Provider. The order of operations is
// deliberate and must not be rearranged:
//
//	rights gate → dedupe check → synthesize → store → return metadata
//
// The rights gate runs first so an unauthorized voice never reaches the
// network, and the dedupe check runs before synthesis so the platform never
// pays twice for identical audio (§60).
type Pipeline struct {
	Provider Provider
	Store    storage.ObjectStorage
	Audit    AuditSink
	Now      func() time.Time
}

// NewPipeline wires a pipeline with sane defaults.
func NewPipeline(p Provider, s storage.ObjectStorage) *Pipeline {
	return &Pipeline{Provider: p, Store: s, Audit: LogAudit{}, Now: time.Now}
}

func (p *Pipeline) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

// GenerateRequest describes one confession rendering.
type GenerateRequest struct {
	ConfessionID string
	VariantID    string
	VoiceID      string
	Language     string
	Text         string
	Version      int
	// Use distinguishes editorial content from user-submitted text; the latter
	// requires its own rights grant because it cannot be reviewed in advance.
	Use rights.Use
	// Territory optionally narrows the rights check to a distribution region.
	Territory   string
	RequestedBy string
	// Force regenerates even when an asset exists, for QA re-renders.
	Force bool
}

// GenerateResult describes the produced (or reused) asset.
type GenerateResult struct {
	Key             string
	SizeBytes       int64
	Checksum        string
	Codec           string
	BitrateKbps     int
	SampleRate      int
	DurationSeconds int
	// Reused is true when an existing asset satisfied the request and no
	// provider call was made.
	Reused bool
}

// Generate renders text to stored audio, refusing anything the licence does not
// explicitly permit.
func (p *Pipeline) Generate(ctx context.Context, lic *rights.License, req GenerateRequest) (*GenerateResult, error) {
	if req.Language == "" {
		req.Language = "en"
	}
	if req.Version < 1 {
		req.Version = 1
	}
	use := req.Use
	if use == "" {
		use = rights.UseSynthesis
	}

	providerName := "none"
	if p.Provider != nil {
		providerName = p.Provider.Name()
	}
	audit := AuditEntry{
		At: p.now(), VoiceID: req.VoiceID, ConfessionID: req.ConfessionID,
		VariantID: req.VariantID, Language: req.Language, Provider: providerName,
		RequestedBy: req.RequestedBy,
	}

	// ---- 1. Rights gate. Nothing reaches the provider before this passes. ----
	decision := rights.Evaluate(lic, rights.Request{
		Use: use, Territory: req.Territory, Language: req.Language, At: p.now(),
	})
	if !decision.Allowed {
		audit.Allowed = false
		audit.Reason = string(decision.Reason)
		audit.Detail = decision.Detail
		p.record(ctx, audit)
		return nil, &ErrRightsDenied{Decision: decision}
	}

	key := storage.AudioKeyFor(req.ConfessionID, req.VariantID, req.VoiceID, req.Language, req.Version)
	audit.AssetKey = key

	if p.Store == nil {
		return nil, errors.New("no object storage configured")
	}

	// ---- 2. Dedupe. Never pay the provider for audio we already hold. ----
	if !req.Force {
		if exists, _ := p.Store.Exists(ctx, key); exists {
			size, _ := p.Store.GetSize(ctx, key)
			audit.Allowed = true
			audit.Reason = "reused_existing_asset"
			p.record(ctx, audit)
			return &GenerateResult{Key: key, SizeBytes: size, Codec: "mp3", Reused: true}, nil
		}
	}

	if p.Provider == nil {
		return nil, errors.New("no synthesis provider configured")
	}

	// ---- 3. Synthesize. ----
	out, err := p.Provider.Synthesize(ctx, SynthesisRequest{
		ProviderVoiceID: lic.ProviderVoiceID,
		Text:            req.Text,
		Language:        req.Language,
	})
	if err != nil {
		audit.Allowed = true
		audit.Reason = "provider_error"
		audit.Detail = err.Error()
		p.record(ctx, audit)
		return nil, err
	}

	// ---- 4. Verify the bytes before anything trusts them. ----
	//
	// Duration is measured here, never taken from the provider's word. The
	// session planner schedules every queue item against duration_seconds, so a
	// provider that under-reports lets a session overrun its plan and one that
	// over-reports leaves trailing silence. SynthesisResult documents its
	// DurationSeconds as "may be zero ... the pipeline then derives it during
	// post-processing" - this is that derivation, and it is also the point
	// where a truncated or corrupt render is caught, before it is stored and
	// billed for.
	report, err := audio.Inspect(out.Audio, "")
	if err != nil {
		audit.Allowed = true
		audit.Reason = "rejected"
		audit.Detail = "provider returned audio this system cannot verify: " + err.Error()
		p.record(ctx, audit)
		return nil, fmt.Errorf("verify generated audio: %w", err)
	}

	// A provider whose self-report disagrees with the container is not failing
	// yet, but it is worth recording: the audio team needs to know before the
	// drift shows up as sessions that run long.
	detail := ""
	if out.DurationSeconds > 0 && absInt(out.DurationSeconds-report.DurationSeconds) > 1 {
		detail = fmt.Sprintf("provider reported %ds, container measures %ds",
			out.DurationSeconds, report.DurationSeconds)
	}

	// ---- 5. Store. ----
	sum := sha256.Sum256(out.Audio)
	checksum := hex.EncodeToString(sum[:])
	meta := map[string]string{
		"confession_id": req.ConfessionID,
		"voice_id":      req.VoiceID,
		"language":      req.Language,
		"codec":         out.Codec,
		"checksum":      checksum,
		// Measured, not claimed. Anything reading this back gets the number the
		// planner was built on.
		"duration_seconds": strconv.Itoa(report.DurationSeconds),
		"sample_rate":      strconv.Itoa(report.SampleRate),
		"format":           report.Format,
	}
	if err := p.Store.Upload(ctx, key, out.Audio, meta); err != nil {
		audit.Allowed = true
		audit.Reason = "storage_error"
		audit.Detail = err.Error()
		p.record(ctx, audit)
		return nil, fmt.Errorf("store audio: %w", err)
	}

	audit.Allowed = true
	audit.Reason = "generated"
	audit.Detail = detail
	p.record(ctx, audit)

	return &GenerateResult{
		Key: key, SizeBytes: int64(len(out.Audio)), Checksum: checksum,
		Codec: out.Codec, BitrateKbps: out.BitrateKbps,
		// From the container, not from the provider.
		SampleRate:      report.SampleRate,
		DurationSeconds: report.DurationSeconds,
	}, nil
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func (p *Pipeline) record(ctx context.Context, e AuditEntry) {
	if p.Audit != nil {
		p.Audit.RecordGeneration(ctx, e)
	}
}
