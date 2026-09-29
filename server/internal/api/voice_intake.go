package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/jobs"
	"github.com/Teamthy/i-confess/internal/ratelimit"
	"github.com/Teamthy/i-confess/internal/store"
	"github.com/Teamthy/i-confess/internal/voicedata"
	"github.com/Teamthy/i-confess/internal/voiceengine"
	"github.com/Teamthy/i-confess/internal/voicegov"
)

// Queue job types for intake and training.
const (
	JobVoiceIngest      = "voice.ingest"
	JobVoiceTrainSubmit = "voice.train.submit"
	JobVoiceTrainPoll   = "voice.train.poll"
)

// MaxRecordingBytes bounds one uploaded source recording.
const MaxRecordingBytes = 512 << 20

// VoiceIntakeConfig tunes intake and training. Zero values take defaults.
type VoiceIntakeConfig struct {
	Thresholds     voicedata.Thresholds
	FineTune       voicedata.FineTunePolicy
	PollInterval   time.Duration
	MaxPolls       int
	TrainingEngine map[string]bool // engines allowed for fine-tuning
}

// SetVoiceWorker enables intake and training against a worker pool.
func (h *Handler) SetVoiceWorker(c *voiceengine.WorkerClient, cfg VoiceIntakeConfig) {
	if cfg.Thresholds == (voicedata.Thresholds{}) {
		cfg.Thresholds = voicedata.DefaultThresholds()
	}
	if cfg.FineTune == (voicedata.FineTunePolicy{}) {
		cfg.FineTune = voicedata.DefaultFineTunePolicy()
	}
	if cfg.MaxPolls == 0 {
		cfg.MaxPolls = 60 * 24 * 3 // three days at one-minute polls
	}
	if cfg.TrainingEngine == nil {
		cfg.TrainingEngine = map[string]bool{"gpt_sovits": true, "cosyvoice": true}
	}
	h.vworker, h.vintake = c, cfg
}

// RegisterVoiceIntakeJobs registers the intake and training job handlers.
func (h *Handler) RegisterVoiceIntakeJobs() {
	h.queue.Register(JobVoiceIngest, h.runVoiceIngest)
	h.queue.Register(JobVoiceTrainSubmit, h.runTrainSubmit)
	h.queue.Register(JobVoiceTrainPoll, h.runTrainPoll)
}

func (h *Handler) requireWorker(w http.ResponseWriter) bool {
	if h.vworker == nil || h.queue == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "voice intake worker is not configured on this server")
		return false
	}
	return true
}

func (h *Handler) authorizeVoice(w http.ResponseWriter, r *http.Request, voiceID string, req voicegov.Request) (*voicegov.Grant, bool) {
	g, err := h.vplat.Grant(r.Context(), voiceID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load rights")
		return nil, false
	}
	if g == nil {
		httpx.WriteError(w, http.StatusNotFound, "voice not found")
		return nil, false
	}
	if d := voicegov.Authorize(g, req); !d.Allowed {
		_, actor := actorOf(r)
		_ = h.vplat.AppendRightsAudit(r.Context(), store.RightsAuditEntry{VoiceID: voiceID, Actor: actor,
			Action: "REFUSED_" + strings.ToUpper(string(req.Action)), GrantVersion: d.GrantVersion, Decision: "denied",
			Reason: string(d.Reason), Detail: d.Detail, RemoteAddr: clientIP(r)})
		httpx.WriteJSON(w, http.StatusForbidden, map[string]any{"error": "rights do not permit this action",
			"reason": d.Reason, "detail": d.Detail, "missing": d.Missing})
		return nil, false
	}
	return g, true
}

// ---------------------------------------------------------------- recordings

// adminUploadRecording accepts a licensed source recording as multipart form
// data (field "file" plus metadata). The server hashes the bytes itself and
// stores them under a private key; the client never chooses the key.
func (h *Handler) adminUploadRecording(w http.ResponseWriter, r *http.Request) {
	if !h.requireWorker(w) {
		return
	}
	if h.signer == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "object storage is not configured")
		return
	}
	voiceID := r.PathValue("id")
	if _, ok := h.authorizeVoice(w, r, voiceID, voicegov.Request{Action: voicegov.ActionIngestRecording}); !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxRecordingBytes+1<<20)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "expected multipart form with a file field")
		return
	}
	defer r.MultipartForm.RemoveAll() //nolint:errcheck
	f, hdr, err := r.FormFile("file")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer f.Close()
	if ext := strings.ToLower(path.Ext(hdr.Filename)); ext != ".wav" && ext != ".flac" {
		httpx.WriteError(w, http.StatusBadRequest, "recordings must be lossless .wav or .flac")
		return
	}
	data, err := io.ReadAll(f)
	if err != nil || len(data) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "could not read file")
		return
	}
	src := voicedata.SourceType(r.FormValue("sourceType"))
	if !voicedata.ValidSource(src) {
		httpx.WriteError(w, http.StatusBadRequest, "sourceType must be upload, authorized_archive or studio_session")
		return
	}
	if strings.TrimSpace(r.FormValue("rightsDocumentId")) == "" || strings.TrimSpace(r.FormValue("title")) == "" {
		httpx.WriteError(w, http.StatusBadRequest, "title and rightsDocumentId are required")
		return
	}
	// Explicit confirmation that the file was supplied by or for the rights
	// holder - not downloaded from a platform whose terms forbid it.
	if r.FormValue("sourceAttestation") != "true" {
		httpx.WriteError(w, http.StatusBadRequest, "sourceAttestation is required: confirm the file was supplied under the rights document, not downloaded from a platform")
		return
	}
	sum := sha256.Sum256(data)
	sha := hex.EncodeToString(sum[:])
	key := fmt.Sprintf("voice-private/voices/%s/recordings/%s%s", voiceID, sha, strings.ToLower(path.Ext(hdr.Filename)))
	_, actor := actorOf(r)
	lang := r.FormValue("language")
	if lang == "" {
		lang = "en"
	}
	rec := &store.Recording{VoiceID: voiceID, SourceType: string(src), SourceURL: r.FormValue("sourceUrl"),
		Title: r.FormValue("title"), RecordingDate: r.FormValue("recordingDate"), Speaker: r.FormValue("speaker"),
		RightsDocumentID: r.FormValue("rightsDocumentId"), ProcessingPermission: r.FormValue("processingPermission") == "true",
		TrainingAllowed: r.FormValue("trainingAllowed") == "true", StorageKey: key, SHA256: sha, Language: lang, UploadedBy: actor}
	if err := h.signer.Upload(r.Context(), key, data, map[string]string{"voice_id": voiceID, "sha256": sha, "private": "true"}); err != nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "failed to store recording")
		return
	}
	if err := h.vplat.CreateRecording(r.Context(), rec); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if rec.ProcessingPermission {
		h.enqueueIngest(r.Context(), rec)
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"recording": rec})
}

func (h *Handler) enqueueIngest(ctx context.Context, rec *store.Recording) {
	jobID, err := h.queue.Enqueue(ctx, jobs.Job{Type: JobVoiceIngest, MaxAttempts: 3,
		IdempotencyKey: "ingest:" + rec.ID + ":" + time.Now().UTC().Format("20060102T150405"),
		Payload:        map[string]any{"recording_id": rec.ID}})
	if err == nil || errors.Is(err, jobs.ErrDuplicateJob) {
		_ = h.vplat.SetRecordingJob(ctx, rec.ID, jobID)
		rec.Status, rec.JobID = "processing", jobID
	}
}

func (h *Handler) adminListRecordings(w http.ResponseWriter, r *http.Request) {
	recs, err := h.vplat.Recordings(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to list recordings")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"recordings": recs})
}

func (h *Handler) adminGetRecording(w http.ResponseWriter, r *http.Request) {
	rec, err := h.vplat.RecordingByID(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "recording not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"recording": rec})
}

func (h *Handler) adminReingestRecording(w http.ResponseWriter, r *http.Request) {
	if !h.requireWorker(w) {
		return
	}
	rec, err := h.vplat.RecordingByID(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "recording not found")
		return
	}
	if !rec.ProcessingPermission {
		httpx.WriteError(w, http.StatusForbidden, "this recording's licence does not permit processing")
		return
	}
	if _, ok := h.authorizeVoice(w, r, rec.VoiceID, voicegov.Request{Action: voicegov.ActionIngestRecording}); !ok {
		return
	}
	h.enqueueIngest(r.Context(), rec)
	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"recording": rec})
}

func (h *Handler) runVoiceIngest(ctx context.Context, p map[string]any) error {
	id, _ := p["recording_id"].(string)
	rec, err := h.vplat.RecordingByID(ctx, id)
	if err != nil {
		return jobs.Permanent(err)
	}
	if rec.Status != "processing" {
		return nil
	}
	if h.vworker == nil {
		return jobs.Permanent(errors.New("intake worker not configured"))
	}
	g, err := h.vplat.Grant(ctx, rec.VoiceID)
	if err != nil {
		return err
	}
	if d := voicegov.Authorize(g, voicegov.Request{Action: voicegov.ActionIngestRecording}); !d.Allowed || !rec.ProcessingPermission {
		_ = h.vplat.FailRecording(ctx, rec.ID, "rejected", "rights no longer permit processing: "+d.Detail)
		return jobs.Permanent(errors.New("rights denied"))
	}
	refKey := ""
	if refs, err := h.vplat.References(ctx, rec.VoiceID); err == nil {
		best := -1.0
		for _, rf := range refs {
			if rf.RightsOK && rf.Quality > best {
				best, refKey = rf.Quality, rf.URI
			}
		}
	}
	res, err := h.vworker.Ingest(ctx, voiceengine.IngestRequest{RecordingID: rec.ID, VoiceID: rec.VoiceID, AudioKey: rec.StorageKey,
		Language: rec.Language, ReferenceKey: refKey, SegmentPrefix: fmt.Sprintf("voice-private/voices/%s/segments/%s", rec.VoiceID, rec.ID)})
	if err != nil {
		class := voiceengine.Classify(err)
		if class.Retryable() {
			return err
		}
		_ = h.vplat.FailRecording(ctx, rec.ID, "failed", err.Error())
		return jobs.Permanent(err)
	}
	report := res.Report
	if len(report) == 0 {
		report = json.RawMessage(`{}`)
	}
	_, _, err = h.vplat.CompleteIngestion(ctx, rec, res.Segments, h.vintake.Thresholds, res.DurationMS, report)
	return err
}

// ---------------------------------------------------------------- review

func (h *Handler) adminListSegments(w http.ResponseWriter, r *http.Request) {
	segs, err := h.vplat.PoolSegments(r.Context(), r.PathValue("id"), r.URL.Query().Get("status"), 200)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to list segments")
		return
	}
	type withURL struct {
		store.IntakeSegment
		AudioURL string `json:"audioUrl,omitempty"`
	}
	out := make([]withURL, len(segs))
	for i, s := range segs {
		out[i].IntakeSegment = s
		if h.signer != nil {
			// Short-lived: reviewers stream the clip; the key never leaves the server.
			out[i].AudioURL, _ = h.signer.GenerateSignedURL(r.Context(), s.AudioKey, 15*time.Minute)
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"segments": out, "thresholds": h.vintake.Thresholds})
}

func (h *Handler) adminReviewSegment(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Approve            bool   `json:"approve"`
		VerifiedTranscript string `json:"verifiedTranscript"`
		Style              string `json:"style"`
		Reason             string `json:"reason"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Style != "" && !voiceengine.ValidStyle(req.Style) {
		httpx.WriteError(w, http.StatusBadRequest, "unknown style")
		return
	}
	_, actor := actorOf(r)
	err := h.vplat.ReviewSegment(r.Context(), r.PathValue("id"), req.Approve, req.VerifiedTranscript, req.Style, req.Reason, actor)
	switch {
	case errors.Is(err, store.ErrIntakeNotFound):
		httpx.WriteError(w, http.StatusNotFound, "segment not found (frozen dataset segments cannot be edited)")
	case err != nil:
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
	default:
		seg, _ := h.vplat.PoolSegmentByID(r.Context(), r.PathValue("id"))
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"segment": seg})
	}
}

// ---------------------------------------------------------------- datasets

func (h *Handler) adminFreezeDataset(w http.ResponseWriter, r *http.Request) {
	voiceID := r.PathValue("id")
	g, ok := h.authorizeVoice(w, r, voiceID, voicegov.Request{Action: voicegov.ActionPrepareDataset})
	if !ok {
		return
	}
	_, actor := actorOf(r)
	ds, manifest, err := h.vplat.FreezeDataset(r.Context(), voiceID, g.Version, actor, func(v string) string {
		return fmt.Sprintf("voice-private/voices/%s/datasets/%s/manifest.json", voiceID, v)
	})
	if err != nil {
		httpx.WriteError(w, http.StatusConflict, err.Error())
		return
	}
	if h.signer != nil {
		_ = h.signer.Upload(r.Context(), ds.ManifestKey, manifest, map[string]string{"sha256": ds.ManifestSHA256, "private": "true"})
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"dataset": ds})
}

func (h *Handler) adminListDatasets(w http.ResponseWriter, r *http.Request) {
	ds, err := h.vplat.Datasets(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to list datasets")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"datasets": ds})
}

// ---------------------------------------------------------------- training

func (h *Handler) adminCreateTrainingRun(w http.ResponseWriter, r *http.Request) {
	if !h.requireWorker(w) {
		return
	}
	var req struct {
		DatasetID       string          `json:"datasetId"`
		Engine          string          `json:"engine"`
		BaseModel       string          `json:"baseModel"`
		Hyperparameters json.RawMessage `json:"hyperparameters"`
		Justification   string          `json:"justification"`
		Override        bool            `json:"override"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || req.DatasetID == "" || req.Engine == "" || req.BaseModel == "" {
		httpx.WriteError(w, http.StatusBadRequest, "datasetId, engine and baseModel are required")
		return
	}
	if !h.vintake.TrainingEngine[req.Engine] {
		httpx.WriteError(w, http.StatusBadRequest, "engine is not enabled for fine-tuning")
		return
	}
	voiceID := r.PathValue("id")
	g, ok := h.authorizeVoice(w, r, voiceID, voicegov.Request{Action: voicegov.ActionFineTune})
	if !ok {
		return
	}
	ds, err := h.vplat.DatasetByID(r.Context(), req.DatasetID)
	if err != nil || ds.VoiceID != voiceID || ds.Status != "frozen" {
		httpx.WriteError(w, http.StatusBadRequest, "dataset must be a frozen dataset of this voice")
		return
	}
	best, err := h.vplat.BestZeroShotScore(r.Context(), voiceID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load baseline")
		return
	}
	st := voicedata.Stats{Segments: ds.TotalSegments, UsableSeconds: ds.UsableSeconds, TestSegments: ds.TestSegments}
	if err := voicedata.CheckFineTune(h.vintake.FineTune, st, best, req.Justification, req.Override); err != nil {
		httpx.WriteJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "policy": h.vintake.FineTune})
		return
	}
	hp := "{}"
	if len(req.Hyperparameters) > 0 {
		if !json.Valid(req.Hyperparameters) {
			httpx.WriteError(w, http.StatusBadRequest, "hyperparameters must be a JSON object")
			return
		}
		hp = string(req.Hyperparameters)
	}
	_, actor := actorOf(r)
	run := &store.TrainingRun{VoiceID: voiceID, DatasetID: ds.ID, DatasetVersion: ds.DatasetVersion, Engine: req.Engine,
		BaseModel: req.BaseModel, Mode: "fine_tune", Hyperparameters: hp, Justification: req.Justification,
		BaselineScore: best, GrantVersion: g.Version, RequestedBy: actor}
	if err := h.vplat.CreateTrainingRun(r.Context(), run); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to create run")
		return
	}
	jobID, err := h.queue.Enqueue(r.Context(), jobs.Job{Type: JobVoiceTrainSubmit, MaxAttempts: 3,
		IdempotencyKey: "train:" + run.ID, Payload: map[string]any{"run_id": run.ID}})
	if err == nil {
		_ = h.vplat.UpdateTrainingRun(r.Context(), run.ID, store.RunUpdate{JobID: jobID}, actor)
		run.JobID = jobID
	}
	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"run": run})
}

func (h *Handler) adminListTrainingRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := h.vplat.TrainingRuns(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to list runs")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"runs": runs})
}

func (h *Handler) adminGetTrainingRun(w http.ResponseWriter, r *http.Request) {
	run, err := h.vplat.TrainingRunByID(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "run not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"run": run})
}

func (h *Handler) adminCancelTrainingRun(w http.ResponseWriter, r *http.Request) {
	run, err := h.vplat.TrainingRunByID(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "run not found")
		return
	}
	_, actor := actorOf(r)
	if err := h.vplat.UpdateTrainingRun(r.Context(), run.ID, store.RunUpdate{Status: voicedata.RunArchived, Error: "cancelled by " + actor}, actor); err != nil {
		httpx.WriteError(w, http.StatusConflict, err.Error())
		return
	}
	if h.vworker != nil {
		_ = h.vworker.CancelTraining(r.Context(), run.ID)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"runId": run.ID, "status": voicedata.RunArchived})
}

// trainRightsOK re-checks training rights for a run; revocation stops runs.
func (h *Handler) trainRightsOK(ctx context.Context, run *store.TrainingRun) (bool, string) {
	g, err := h.vplat.Grant(ctx, run.VoiceID)
	if err != nil {
		return true, "" // transient: let the caller retry rather than kill a run
	}
	d := voicegov.Authorize(g, voicegov.Request{Action: voicegov.ActionFineTune})
	return d.Allowed, d.Detail
}

func (h *Handler) failRun(ctx context.Context, run *store.TrainingRun, msg string) error {
	_ = h.vplat.UpdateTrainingRun(ctx, run.ID, store.RunUpdate{Status: voicedata.RunFailed, Error: msg}, "worker")
	return jobs.Permanent(errors.New(msg))
}

func (h *Handler) runTrainSubmit(ctx context.Context, p map[string]any) error {
	id, _ := p["run_id"].(string)
	run, err := h.vplat.TrainingRunByID(ctx, id)
	if err != nil {
		return jobs.Permanent(err)
	}
	if run.Status != string(voicedata.RunQueued) {
		return nil
	}
	if ok, why := h.trainRightsOK(ctx, run); !ok {
		return h.failRun(ctx, run, "rights no longer permit training: "+why)
	}
	ds, err := h.vplat.DatasetByID(ctx, run.DatasetID)
	if err != nil {
		return err
	}
	entries, err := h.vplat.DatasetManifest(ctx, ds.ID)
	if err != nil {
		return err
	}
	m := voicedata.Manifest{VoiceID: run.VoiceID, DatasetVersion: ds.DatasetVersion, GrantVersion: ds.GrantVersion, Entries: entries}
	_, sum, err := m.Encode()
	if err != nil {
		return err
	}
	if sum != ds.ManifestSHA256 {
		return h.failRun(ctx, run, "dataset integrity check failed: manifest hash mismatch")
	}
	err = h.vworker.StartTraining(ctx, voiceengine.TrainRequest{RunID: run.ID, VoiceID: run.VoiceID, Engine: run.Engine,
		BaseModel: run.BaseModel, DatasetVersion: ds.DatasetVersion, ManifestSHA256: sum, Entries: m.Entries,
		Hyperparameters: json.RawMessage(run.Hyperparameters),
		CheckpointKey:   fmt.Sprintf("voice-private/voices/%s/checkpoints/%s", run.VoiceID, run.ID)})
	if err != nil {
		if voiceengine.Classify(err).Retryable() {
			return err
		}
		return h.failRun(ctx, run, err.Error())
	}
	if err := h.vplat.UpdateTrainingRun(ctx, run.ID, store.RunUpdate{Status: voicedata.RunRunning}, "worker"); err != nil {
		return err
	}
	return h.enqueuePoll(ctx, run.ID, 1)
}

func (h *Handler) enqueuePoll(ctx context.Context, runID string, n int) error {
	_, err := h.queue.Enqueue(ctx, jobs.Job{Type: JobVoiceTrainPoll, MaxAttempts: 5,
		IdempotencyKey: fmt.Sprintf("train-poll:%s:%d", runID, n), AvailableAt: time.Now().Add(h.vintake.PollInterval),
		Payload: map[string]any{"run_id": runID, "n": float64(n)}})
	if errors.Is(err, jobs.ErrDuplicateJob) {
		return nil
	}
	return err
}

func (h *Handler) runTrainPoll(ctx context.Context, p map[string]any) error {
	id, _ := p["run_id"].(string)
	n, _ := p["n"].(float64)
	run, err := h.vplat.TrainingRunByID(ctx, id)
	if err != nil {
		return jobs.Permanent(err)
	}
	if run.Status != string(voicedata.RunRunning) {
		return nil
	}
	if ok, why := h.trainRightsOK(ctx, run); !ok {
		_ = h.vworker.CancelTraining(ctx, run.ID)
		return h.failRun(ctx, run, "training stopped: rights revoked or restricted: "+why)
	}
	st, err := h.vworker.TrainingStatus(ctx, run.ID)
	if err != nil {
		if voiceengine.Classify(err).Retryable() {
			return err
		}
		return h.failRun(ctx, run, err.Error())
	}
	switch st.Status {
	case "COMPLETED":
		if st.CheckpointKey == "" || st.CheckpointSHA256 == "" {
			return h.failRun(ctx, run, "worker reported completion without a checkpoint and hash")
		}
		// Register an immutable candidate. It is NOT promoted: it must pass
		// the golden-set evaluation gate and human approval like any model.
		m := &voiceengine.Model{VoiceID: run.VoiceID, Engine: voiceengine.Engine(run.Engine), EngineVersion: run.BaseModel,
			ModelVersion: "ft-" + run.DatasetVersion + "-" + run.ID[len(run.ID)-8:], Mode: "fine_tuned",
			CheckpointURI: st.CheckpointKey, DatasetVersion: run.DatasetVersion, TrainingRunID: run.ID}
		if err := h.vplat.CreateModel(ctx, m, "fine-tune "+run.DatasetVersion, "en", "worker"); err != nil {
			return err
		}
		return h.vplat.UpdateTrainingRun(ctx, run.ID, store.RunUpdate{Status: voicedata.RunCompleted, Progress: st.Progress,
			FinalLoss: st.Loss, Hardware: st.Hardware, DurationSeconds: st.DurationSeconds, CheckpointKey: st.CheckpointKey,
			CheckpointSHA256: st.CheckpointSHA256, ModelID: m.ID}, "worker")
	case "FAILED":
		return h.failRun(ctx, run, "worker: "+st.Error)
	default:
		_ = h.vplat.UpdateTrainingRun(ctx, run.ID, store.RunUpdate{Progress: st.Progress, FinalLoss: st.Loss}, "worker")
		if int(n) >= h.vintake.MaxPolls {
			_ = h.vworker.CancelTraining(ctx, run.ID)
			return h.failRun(ctx, run, "training exceeded the maximum run time")
		}
		return h.enqueuePoll(ctx, run.ID, int(n)+1)
	}
}

// ---------------------------------------------------------------- streaming

// streamMinisterVoice streams a live render for low-latency preview. It runs
// the same validation and rights checks as generate, plus can_stream. The
// Go process only relays bytes; inference stays on the GPU worker. Streams
// are not cached and not loudness-mastered as a whole (the worker applies a
// fixed gain and peak safety per chunk); the queued render remains the
// canonical, mastered asset.
func (h *Handler) streamMinisterVoice(w http.ResponseWriter, r *http.Request) {
	if h.vorch == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "voice engine is not configured on this server")
		return
	}
	userID, email := actorOf(r)
	if h.limiter != nil {
		if ok, retry := h.limiter.Allow("voicestream:"+userID, VoiceGenerationRule); !ok {
			ratelimit.TooManyRequests(w, retry)
			return
		}
	}
	req, purpose, userText, ok := h.decodeVoiceRequest(w, r, email)
	if !ok {
		return
	}
	ctx := r.Context()
	if _, err := h.vplat.MinisterVoiceByID(ctx, req.VoiceID); err != nil {
		h.writeVoicePlanError(w, r, req.VoiceID, email, err)
		return
	}
	grant, err1 := h.vplat.Grant(ctx, req.VoiceID)
	models, err2 := h.vplat.Models(ctx, req.VoiceID)
	refs, err3 := h.vplat.References(ctx, req.VoiceID)
	orch, err4 := h.orchestratorWithDictionary(ctx)
	if err := errors.Join(err1, err2, err3, err4); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to plan stream")
		return
	}
	plan, st, err := orch.Stream(ctx, grant, models, refs, voiceengine.Request{
		VoiceID: req.VoiceID, Markup: req.Text, Language: req.Language, Locale: req.Locale, Style: req.Style,
		Purpose: purpose, UserText: userText, Territory: req.Territory, Speed: req.Speed, Pitch: req.Pitch,
	})
	if err != nil {
		h.writeVoicePlanError(w, r, req.VoiceID, email, err)
		return
	}
	defer st.Body.Close()
	_ = h.vplat.AppendRightsAudit(ctx, store.RightsAuditEntry{VoiceID: req.VoiceID, Actor: email, Action: "VOICE_STREAMED",
		ModelID: plan.Model.ID, GrantVersion: plan.Decision.GrantVersion, Decision: "allowed", RemoteAddr: clientIP(r)})
	ct := st.ContentType
	if ct == "" {
		ct = "audio/wav"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Synthetic", "true")
	w.Header().Set("X-AI-Disclosure", "AI-generated using an authorized synthetic voice.")
	w.WriteHeader(http.StatusOK)
	fl, _ := w.(http.Flusher)
	buf := make([]byte, 16<<10)
	for {
		n, rerr := st.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return // client went away; closing Body cancels inference
			}
			if fl != nil {
				fl.Flush()
			}
		}
		if rerr != nil {
			return
		}
	}
}

func (h *Handler) registerVoiceIntakeRoutes(mux *http.ServeMux, authed, voiceMgr, audioMgr func(http.Handler) http.Handler) {
	const vm = "voice_manager"
	const am = "audio_producer,voice_manager"
	for _, pfx := range []string{"", "/v1"} {
		h.route(mux, "POST "+pfx+"/voices/stream", "user", "voice", "Stream a live synthetic preview (not cached)", authed, h.streamMinisterVoice)

		h.route(mux, "POST "+pfx+"/admin/voices/{id}/recordings", am, "admin-voice", "Upload a licensed source recording", audioMgr, h.adminUploadRecording)
		h.route(mux, "GET "+pfx+"/admin/voices/{id}/recordings", am, "admin-voice", "List source recordings", audioMgr, h.adminListRecordings)
		h.route(mux, "GET "+pfx+"/admin/recordings/{id}", am, "admin-voice", "Recording with ingestion report", audioMgr, h.adminGetRecording)
		h.route(mux, "POST "+pfx+"/admin/recordings/{id}/ingest", am, "admin-voice", "Re-run ingestion", audioMgr, h.adminReingestRecording)
		h.route(mux, "GET "+pfx+"/admin/voices/{id}/segments", am, "admin-voice", "Intake segments for review", audioMgr, h.adminListSegments)
		h.route(mux, "POST "+pfx+"/admin/segments/{id}/review", am, "admin-voice", "Approve (with verified transcript) or reject a segment", audioMgr, h.adminReviewSegment)
		h.route(mux, "POST "+pfx+"/admin/voices/{id}/datasets", vm, "admin-voice", "Freeze approved segments into an immutable dataset version", voiceMgr, h.adminFreezeDataset)
		h.route(mux, "GET "+pfx+"/admin/voices/{id}/datasets", am, "admin-voice", "List dataset versions", audioMgr, h.adminListDatasets)
		h.route(mux, "POST "+pfx+"/admin/voices/{id}/training-runs", vm, "admin-voice", "Request a fine-tuning run (zero-shot first)", voiceMgr, h.adminCreateTrainingRun)
		h.route(mux, "GET "+pfx+"/admin/voices/{id}/training-runs", am, "admin-voice", "List training runs", audioMgr, h.adminListTrainingRuns)
		h.route(mux, "GET "+pfx+"/admin/training-runs/{id}", am, "admin-voice", "One training run", audioMgr, h.adminGetTrainingRun)
		h.route(mux, "POST "+pfx+"/admin/training-runs/{id}/cancel", vm, "admin-voice", "Cancel a training run", voiceMgr, h.adminCancelTrainingRun)
	}
}
