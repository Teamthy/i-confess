package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Teamthy/i-confess/internal/bible"
	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/rights"
	"github.com/google/uuid"
)

type bibleAudioManifestInput struct {
	TranslationID           string          `json:"translation_id"`
	BookID                  string          `json:"book_id"`
	Chapter                 int             `json:"chapter"`
	VerseStart              int             `json:"verse_start"`
	VerseEnd                int             `json:"verse_end"`
	VoiceID                 string          `json:"voice_id"`
	AudioSource             string          `json:"audio_source"`
	SourceRightsEvidenceURL string          `json:"source_rights_evidence_url"`
	StorageKey              string          `json:"storage_key"`
	ChecksumSHA256          string          `json:"checksum_sha256"`
	DurationMS              int64           `json:"duration_ms"`
	Alignment               json.RawMessage `json:"alignment"`
	AttributionText         string          `json:"attribution_text"`
}

func (h *Handler) adminBibleAudioList(w http.ResponseWriter, r *http.Request) {
	items, err := readBibleMaps(r.Context(), h.db, `SELECT id,translation_id,book_id,chapter,verse_start,verse_end,content_hash,voice_id,audio_source,checksum_sha256,duration_ms,status,rights_reviewed_by,rights_reviewed_at,created_at FROM bible_audio_assets WHERE ($1='' OR status=$1) ORDER BY created_at DESC LIMIT 500`, strings.TrimSpace(r.URL.Query().Get("status")))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to load Bible audio review queue.")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"assets": items})
}

func (h *Handler) adminRegisterBibleAudio(w http.ResponseWriter, r *http.Request) {
	var in bibleAudioManifestInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "Invalid Bible audio manifest.")
		return
	}
	bookID, ok := bible.ParseBook(in.BookID)
	if !ok || in.Chapter < 1 || in.VerseStart < 1 || in.VerseEnd < in.VerseStart || in.VerseEnd-in.VerseStart > 199 || in.VoiceID == "" || in.StorageKey == "" || in.DurationMS < 1 || in.DurationMS > 24*60*60*1000 || len(in.ChecksumSHA256) != 64 {
		httpx.WriteError(w, http.StatusBadRequest, "Provide a valid passage, voice, storage key, checksum and duration.")
		return
	}
	if in.AudioSource != "human_recording" && in.AudioSource != "licensed_master" && in.AudioSource != "synthetic" {
		httpx.WriteError(w, http.StatusBadRequest, "Choose an explicit audio source classification.")
		return
	}
	evidence, err := url.Parse(in.SourceRightsEvidenceURL)
	if err != nil || evidence.Scheme != "https" || evidence.Host == "" {
		httpx.WriteError(w, http.StatusBadRequest, "A secure source-rights evidence URL is required.")
		return
	}
	t, err := h.bible.GetTranslation(r.Context(), in.TranslationID)
	if err != nil {
		h.bibleError(w, err)
		return
	}
	if !t.AudioAllowed || !t.RedistributionAllowed || !t.CommercialUse || !t.APIExposureAllowed {
		httpx.WriteError(w, http.StatusForbidden, "The translation has not been approved for commercial audio and redistribution.")
		return
	}
	if _, err := h.bible.GetVerse(r.Context(), t.ID, bookID, in.Chapter, in.VerseStart); err != nil {
		h.bibleError(w, err)
		return
	}
	if _, err := h.bible.GetVerse(r.Context(), t.ID, bookID, in.Chapter, in.VerseEnd); err != nil {
		h.bibleError(w, err)
		return
	}
	if !json.Valid(in.Alignment) || len(in.Alignment) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "Alignment must be valid JSON.")
		return
	}
	if h.signer == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "Object storage is unavailable.")
		return
	}
	exists, err := h.signer.Exists(r.Context(), in.StorageKey)
	if err != nil || !exists {
		httpx.WriteError(w, http.StatusBadRequest, "The referenced audio object does not exist.")
		return
	}
	size, err := h.signer.GetSize(r.Context(), in.StorageKey)
	if err != nil || size < 1 || size > 250<<20 {
		httpx.WriteError(w, http.StatusBadRequest, "Audio object size is outside the supported range.")
		return
	}
	data, err := h.signer.Download(r.Context(), in.StorageKey)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "The audio object could not be verified.")
		return
	}
	digest := sha256.Sum256(data)
	if !strings.EqualFold(hex.EncodeToString(digest[:]), in.ChecksumSHA256) {
		httpx.WriteError(w, http.StatusBadRequest, "Audio checksum does not match the uploaded object.")
		return
	}
	id := uuid.NewString()
	contentHash := t.ContentHash
	if contentHash == "" {
		contentHash = t.SourceVersion
	}
	if contentHash == "" {
		httpx.WriteError(w, http.StatusConflict, "The translation has no immutable source-content hash.")
		return
	}
	_, err = h.db.ExecContext(r.Context(), `INSERT INTO bible_audio_assets(id,translation_id,book_id,chapter,verse_start,verse_end,content_hash,voice_id,audio_source,source_rights_evidence_url,storage_key,checksum_sha256,duration_ms,alignment,attribution_text,status) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,'pending_review')`, id, t.ID, bookID, in.Chapter, in.VerseStart, in.VerseEnd, contentHash, in.VoiceID, in.AudioSource, in.SourceRightsEvidenceURL, in.StorageKey, strings.ToLower(in.ChecksumSHA256), in.DurationMS, string(in.Alignment), strings.TrimSpace(in.AttributionText))
	if err != nil {
		httpx.WriteError(w, http.StatusConflict, "This audio manifest already exists or is invalid.")
		return
	}
	h.recordAudit(r, "bible_audio_manifest_registered", "bible_audio", id, "status=pending_review", "success")
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "pending_review", "content_hash": contentHash, "checksum_sha256": strings.ToLower(in.ChecksumSHA256)})
}

func (h *Handler) adminReviewBibleAudio(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Decision  string `json:"decision"`
		Rationale string `json:"rationale"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil || (in.Decision != "approve" && in.Decision != "withdraw") || len(strings.TrimSpace(in.Rationale)) < 8 {
		httpx.WriteError(w, http.StatusBadRequest, "Choose approve or withdraw and provide a review rationale.")
		return
	}
	id := r.PathValue("id")
	var translationID, bookID, voiceID, audioSource, key, hash, contentHash string
	var chapter, start, end int
	err := h.db.QueryRowContext(r.Context(), `SELECT translation_id,book_id,voice_id,audio_source,storage_key,checksum_sha256,content_hash,chapter,verse_start,verse_end FROM bible_audio_assets WHERE id=?`, id).Scan(&translationID, &bookID, &voiceID, &audioSource, &key, &hash, &contentHash, &chapter, &start, &end)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "Bible audio asset not found.")
		return
	}
	if in.Decision == "approve" {
		t, err := h.bible.GetTranslation(r.Context(), translationID)
		if err != nil || !t.AudioAllowed || !t.RedistributionAllowed || !t.CommercialUse || !t.APIExposureAllowed {
			httpx.WriteError(w, http.StatusForbidden, "Translation audio grants are not active.")
			return
		}
		currentHash := t.ContentHash
		if currentHash == "" {
			currentHash = t.SourceVersion
		}
		if currentHash == "" || currentHash != contentHash {
			httpx.WriteError(w, http.StatusConflict, "The source text changed; this audio must be re-reviewed against the current content hash.")
			return
		}
		if h.signer == nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "Object storage is unavailable.")
			return
		}
		exists, err := h.signer.Exists(r.Context(), key)
		if err != nil || !exists {
			httpx.WriteError(w, http.StatusConflict, "The audio object no longer exists.")
			return
		}
		data, err := h.signer.Download(r.Context(), key)
		if err != nil {
			httpx.WriteError(w, http.StatusConflict, "The audio object could not be verified.")
			return
		}
		digest := sha256.Sum256(data)
		if !strings.EqualFold(hex.EncodeToString(digest[:]), hash) {
			httpx.WriteError(w, http.StatusConflict, "Audio checksum verification failed.")
			return
		}
		lic, _, _ := h.rightsFor(r.Context(), voiceID)
		now := time.Now().UTC()
		if decision := rights.Evaluate(lic, rights.Request{Use: rights.UsePlayback, At: now, Language: t.Locale}); !decision.Allowed {
			httpx.WriteError(w, http.StatusForbidden, "Current voice playback rights do not permit publication.")
			return
		}
		if audioSource == "synthetic" {
			if decision := rights.Evaluate(lic, rights.Request{Use: rights.UseSynthesis, At: now, Language: t.Locale}); !decision.Allowed {
				httpx.WriteError(w, http.StatusForbidden, "The voice license does not permit synthesis.")
				return
			}
		}
		for _, n := range []int{start, end} {
			if _, err := h.bible.GetVerse(r.Context(), translationID, bookID, chapter, n); err != nil {
				httpx.WriteError(w, http.StatusConflict, "The referenced canonical source passage is no longer available.")
				return
			}
		}
	}
	status := "published"
	if in.Decision == "withdraw" {
		status = "withdrawn"
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = h.db.ExecContext(r.Context(), `UPDATE bible_audio_assets SET status=?,rights_reviewed_by=?,rights_reviewed_at=? WHERE id=?`, status, h.userID(r), now, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to save audio review.")
		return
	}
	if status == "withdrawn" && h.signer != nil {
		if err := h.signer.Delete(r.Context(), key); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "Audio was withdrawn but object deletion needs operator attention.")
			return
		}
	}
	h.recordAudit(r, "bible_audio_reviewed", "bible_audio", id, "decision="+in.Decision+" rationale="+strings.TrimSpace(in.Rationale), "success")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "status": status, "reviewed_at": now})
}
