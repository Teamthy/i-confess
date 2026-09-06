package store

import (
	"context"
	"errors"
	"testing"

	"github.com/Teamthy/i-confess/internal/audio"
	"github.com/Teamthy/i-confess/internal/db"
	"github.com/Teamthy/i-confess/internal/db/dbtest"
	"github.com/Teamthy/i-confess/internal/models"
)

// qaFixture builds the minimum a generation touches: a category, a confession
// and a voice.
type qaFixture struct {
	confession *models.Confession
	voice      *models.Voice
	asset      models.AudioAsset
	cs         *ContentStore
	as         *AudioStore
}

func newQAFixture(t *testing.T) (*db.DB, *qaFixture) {
	t.Helper()
	conn := dbtest.New(t)
	ctx := context.Background()
	cs := NewContentStore(conn)
	as := NewAudioStore(conn)

	cat := &models.Category{Name: "Peace", Slug: "peace", Status: "published"}
	if err := cs.CreateCategory(ctx, cat); err != nil {
		t.Fatal(err)
	}
	conf := &models.Confession{
		CategoryID: cat.ID, Title: "Be still", Status: "approved", Language: "en",
		ShortText: "I am at peace.", MediumText: "I am at peace and made whole.",
		LongText: "I am at peace and made whole in body and mind.",
		Variants: []models.ConfessionVariant{{Label: "1m", DurationSeconds: 60}},
	}
	if err := cs.CreateConfession(ctx, conf); err != nil {
		t.Fatal(err)
	}
	v := &models.Voice{Name: "Grace", Type: "professional", Language: "en", Status: "active"}
	if err := as.CreateVoice(ctx, v); err != nil {
		t.Fatal(err)
	}

	a := models.AudioAsset{
		ConfessionID: conf.ID, VariantID: "v1", VoiceID: v.ID,
		URL:             "audio/" + conf.ID + "/v1/" + v.ID + "/en/v1.m4a",
		DurationSeconds: 60, Status: string(audio.StatusProcessing),
	}
	if err := as.UpsertAsset(ctx, &a); err != nil {
		t.Fatal(err)
	}
	return conn, &qaFixture{confession: conf, voice: v, asset: a, cs: cs, as: as}
}

// TestGenerationJobIsRecordedAndObservable covers the directive's "generation
// requests" and "generation status". The audio_generation_jobs table had
// existed since the baseline schema with status, attempts, error fields and a
// unique idempotency key, and nothing had ever written to it.
func TestGenerationJobIsRecordedAndObservable(t *testing.T) {
	conn, f := newQAFixture(t)
	defer conn.Close()
	ctx := context.Background()

	version, err := f.cs.EnsureVersion(ctx, f.confession.ID, f.confession.Title,
		f.confession.ShortText, f.confession.MediumText, f.confession.LongText, "en", "tester")
	if err != nil {
		t.Fatalf("EnsureVersion: %v", err)
	}
	if version.VersionNumber != 1 {
		t.Errorf("first version = %d, want 1", version.VersionNumber)
	}

	job, created, err := f.as.CreateJob(ctx, &models.AudioJob{
		ConfessionID: f.confession.ID, VariantID: "v1", VoiceID: f.voice.ID,
		ContentVersionID: version.ID, RequestedBy: "tester",
		IdempotencyKey: "key-1",
	})
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if !created {
		t.Error("first CreateJob should create")
	}
	if job.Status != string(audio.JobQueued) {
		t.Errorf("status = %q, want queued", job.Status)
	}
	if job.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want the schema default of 3", job.MaxAttempts)
	}

	// The whole point: the request is now observable.
	read, err := f.as.JobByID(ctx, job.ID)
	if err != nil {
		t.Fatalf("JobByID: %v", err)
	}
	if read.ContentVersionID != version.ID || read.RequestedBy != "tester" {
		t.Errorf("job lost its request context: %+v", read)
	}

	started, err := f.as.StartJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("StartJob: %v", err)
	}
	if started.Status != string(audio.JobProcessing) || started.AttemptCount != 1 {
		t.Errorf("after start: status=%q attempts=%d, want processing/1", started.Status, started.AttemptCount)
	}
	if started.StartedAt == "" {
		t.Error("started_at was not stamped")
	}

	done, err := f.as.CompleteJob(ctx, job.ID, f.asset.ID)
	if err != nil {
		t.Fatalf("CompleteJob: %v", err)
	}
	if done.Status != string(audio.JobSucceeded) {
		t.Errorf("status = %q, want succeeded", done.Status)
	}
	if done.AudioAssetID != f.asset.ID {
		t.Errorf("the job was not linked to its asset: %q", done.AudioAssetID)
	}
	if done.CompletedAt == "" {
		t.Error("completed_at was not stamped")
	}

	// A succeeded job is terminal: it must not be re-run by a stray transition.
	if _, err := f.as.StartJob(ctx, job.ID); err == nil {
		t.Error("a succeeded job was restarted")
	}
}

// TestGenerationJobIdempotencyPreventsDoubleBilling covers the reason the
// table has a UNIQUE idempotency key: a double-submitted form or a client
// retry must not pay the provider twice for the same render.
func TestGenerationJobIdempotencyPreventsDoubleBilling(t *testing.T) {
	conn, f := newQAFixture(t)
	defer conn.Close()
	ctx := context.Background()

	version, err := f.cs.EnsureVersion(ctx, f.confession.ID, f.confession.Title,
		f.confession.ShortText, f.confession.MediumText, f.confession.LongText, "en", "tester")
	if err != nil {
		t.Fatal(err)
	}
	mk := func() *models.AudioJob {
		return &models.AudioJob{
			ConfessionID: f.confession.ID, VoiceID: f.voice.ID,
			ContentVersionID: version.ID, IdempotencyKey: "same-render",
		}
	}

	first, created, err := f.as.CreateJob(ctx, mk())
	if err != nil || !created {
		t.Fatalf("first create: created=%v err=%v", created, err)
	}
	second, created, err := f.as.CreateJob(ctx, mk())
	if err != nil {
		t.Fatalf("second create: %v", err)
	}
	if created {
		t.Fatal("the same idempotency key created a second job; the provider would be billed twice")
	}
	if second.ID != first.ID {
		t.Errorf("second job id = %q, want the original %q", second.ID, first.ID)
	}

	byKey, err := f.as.JobByIdempotencyKey(ctx, "same-render")
	if err != nil {
		t.Fatalf("JobByIdempotencyKey: %v", err)
	}
	if byKey.ID != first.ID {
		t.Errorf("lookup by key returned %q, want %q", byKey.ID, first.ID)
	}
}

func TestFailedJobRecordsWhyItFailed(t *testing.T) {
	conn, f := newQAFixture(t)
	defer conn.Close()
	ctx := context.Background()

	version, _ := f.cs.EnsureVersion(ctx, f.confession.ID, f.confession.Title,
		f.confession.ShortText, f.confession.MediumText, f.confession.LongText, "en", "tester")
	job, _, err := f.as.CreateJob(ctx, &models.AudioJob{
		ConfessionID: f.confession.ID, VoiceID: f.voice.ID, ContentVersionID: version.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.as.StartJob(ctx, job.ID); err != nil {
		t.Fatal(err)
	}

	failed, err := f.as.FailJob(ctx, job.ID, "provider_rejected", "voice id not licensed for this territory")
	if err != nil {
		t.Fatalf("FailJob: %v", err)
	}
	if failed.ErrorCode != "provider_rejected" || failed.ErrorMessage == "" {
		t.Errorf("failure was not explained: code=%q message=%q", failed.ErrorCode, failed.ErrorMessage)
	}
	// A failed job may be retried while attempts remain - which is what
	// max_attempts is for, and what the old synchronous call could never do.
	if !audio.CanTransitionJob(audio.JobFailed, audio.JobQueued) {
		t.Error("a failed job should be requeueable")
	}
}

func TestJobRequiresAContentVersion(t *testing.T) {
	conn, f := newQAFixture(t)
	defer conn.Close()

	// content_version_id is NOT NULL and a foreign key, so a job cannot be
	// recorded without knowing which text was spoken.
	if _, _, err := f.as.CreateJob(context.Background(), &models.AudioJob{
		ConfessionID: f.confession.ID, VoiceID: f.voice.ID,
	}); err == nil {
		t.Error("a job was recorded with no content version")
	}
}

// TestEnsureVersionIsIdempotentPerText covers audio versioning. Regenerating
// unchanged text must not inflate the history, or "which version is current"
// stops meaning anything.
func TestEnsureVersionIsIdempotentPerText(t *testing.T) {
	conn, f := newQAFixture(t)
	defer conn.Close()
	ctx := context.Background()

	snapshot := func() (models.ContentVersion, error) {
		return f.cs.EnsureVersion(ctx, f.confession.ID, "Be still",
			"I am at peace.", "I am at peace and made whole.", "unchanged long text", "en", "tester")
	}

	first, err := snapshot()
	if err != nil {
		t.Fatal(err)
	}
	again, err := snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != first.ID {
		t.Errorf("unchanged text created a second version: %q then %q", first.ID, again.ID)
	}

	// Edited text is a new version, and the old one still exists so audio
	// already rendered from it stays traceable.
	edited, err := f.cs.EnsureVersion(ctx, f.confession.ID, "Be still",
		"I am at peace.", "I am at peace, whole and kept.", "unchanged long text", "en", "editor")
	if err != nil {
		t.Fatal(err)
	}
	if edited.ID == first.ID {
		t.Fatal("edited text reused the previous version")
	}
	if edited.VersionNumber != 2 {
		t.Errorf("version_number = %d, want 2", edited.VersionNumber)
	}

	all, err := f.cs.VersionsForConfession(ctx, f.confession.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Errorf("%d versions, want 2 (the old render must stay traceable)", len(all))
	}
	if _, err := f.cs.VersionByID(ctx, first.ID); err != nil {
		t.Errorf("the superseded version was lost: %v", err)
	}
}

// TestQAWorkflowMovesAudioThroughReview covers the directive's QA workflow.
// Before this there was no UPDATE audio_assets anywhere in the codebase, so
// generated audio entered 'processing' and could never leave.
func TestQAWorkflowMovesAudioThroughReview(t *testing.T) {
	conn, f := newQAFixture(t)
	defer conn.Close()
	ctx := context.Background()

	// A rejection with a reason, recorded against a person.
	rejected, err := f.as.SetAssetStatus(ctx, f.asset.ID, audio.StatusQARejected, "qa-lead", "sibilance on the third phrase")
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if rejected.Status != string(audio.StatusQARejected) {
		t.Errorf("status = %q, want qa_rejected", rejected.Status)
	}
	if rejected.QAReviewedBy != "qa-lead" || rejected.QANote == "" {
		t.Errorf("the decision was not attributed: by=%q note=%q", rejected.QAReviewedBy, rejected.QANote)
	}

	// A human rejection cannot be silently overridden.
	if _, err := f.as.SetAssetStatus(ctx, f.asset.ID, audio.StatusReady, "someone", ""); err == nil {
		t.Error("a rejected render was approved straight through")
	}

	// Re-render, then approve.
	rerendered, err := f.as.SetAssetStatus(ctx, f.asset.ID, audio.StatusProcessing, "", "")
	if err != nil {
		t.Fatalf("requeue: %v", err)
	}
	if rerendered.Status != string(audio.StatusProcessing) {
		t.Errorf("status = %q, want processing", rerendered.Status)
	}

	approved, err := f.as.SetAssetStatus(ctx, f.asset.ID, audio.StatusReady, "qa-lead", "clean on the second render")
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if approved.Status != string(audio.StatusReady) {
		t.Errorf("status = %q, want ready", approved.Status)
	}
	if !audio.IsServed(approved.Status) {
		t.Error("an approved render is still not servable")
	}

	// Withdrawing must work, because session queues are snapshots: this is how
	// an asset is pulled from sessions that already reference it.
	archived, err := f.as.SetAssetStatus(ctx, f.asset.ID, audio.StatusArchived, "qa-lead", "voice licence lapsed")
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if audio.IsServed(archived.Status) {
		t.Error("an archived render is still servable")
	}

	// Archived is terminal.
	if _, err := f.as.SetAssetStatus(ctx, f.asset.ID, audio.StatusReady, "qa-lead", ""); err == nil {
		t.Error("an archived render was restored")
	}
}

func TestSetAssetStatusRejectsAnUnknownAsset(t *testing.T) {
	conn, f := newQAFixture(t)
	defer conn.Close()

	_, err := f.as.SetAssetStatus(context.Background(), "no-such-asset", audio.StatusReady, "qa", "")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}
