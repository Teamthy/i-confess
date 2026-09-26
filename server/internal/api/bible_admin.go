package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/Teamthy/i-confess/internal/bible"
	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/google/uuid"
)

var safeBibleID = regexp.MustCompile(`[^a-z0-9-]+`)

func helloAOAppID(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	id = strings.Trim(safeBibleID.ReplaceAllString(id, "-"), "-")
	return "helloao-" + id
}

func languageISO6393(code string) (string, bool) {
	m := map[string]string{"en": "eng", "sw": "swa", "es": "spa", "pt": "por", "fr": "fra", "it": "ita", "tl": "tgl", "ar": "ara", "he": "heb", "yo": "yor", "ig": "ibo", "ha": "hau", "de": "deu", "nl": "nld"}
	v, ok := m[strings.ToLower(code)]
	return v, ok
}

// adminBibleOverview is intentionally aggregate-only; it never includes user
// study records or note content.
func (h *Handler) adminBibleOverview(w http.ResponseWriter, r *http.Request) {
	var out struct {
		Translations    int `json:"translations"`
		Active          int `json:"active"`
		Pending         int `json:"pending_review"`
		FailedImports   int `json:"failed_imports"`
		PublishedAudio  int `json:"published_audio"`
		OfflinePackages int `json:"offline_packages"`
	}
	_ = h.db.QueryRowContext(r.Context(), `SELECT count(*),count(*) FILTER(WHERE status='active'),count(*) FILTER(WHERE status='pending_review') FROM bible_versions WHERE deleted_at IS NULL`).Scan(&out.Translations, &out.Active, &out.Pending)
	_ = h.db.QueryRowContext(r.Context(), `SELECT count(*) FROM bible_import_jobs WHERE status='failed' AND created_at>now()-interval '7 days'`).Scan(&out.FailedImports)
	_ = h.db.QueryRowContext(r.Context(), `SELECT count(*) FROM bible_audio_assets WHERE status='published'`).Scan(&out.PublishedAudio)
	_ = h.db.QueryRowContext(r.Context(), `SELECT count(*) FROM bible_offline_packages WHERE status='ready'`).Scan(&out.OfflinePackages)
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) adminBibleHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	start := time.Now()
	dbOK := h.db.PingContext(ctx) == nil
	provider := h.bibleDiscovery
	if provider == nil {
		provider = h.bible
	}
	remoteOK := false
	providerErr := ""
	if checker, ok := provider.(interface{ HealthCheck(context.Context) error }); ok {
		err := checker.HealthCheck(ctx)
		remoteOK = err == nil
		if err != nil {
			providerErr = "provider_unavailable"
		}
	} else {
		providerErr = "health_check_not_supported"
	}
	var languages, translations, pending int
	_ = h.db.QueryRowContext(ctx, `SELECT count(*) FROM bible_languages WHERE status='active'`).Scan(&languages)
	_ = h.db.QueryRowContext(ctx, `SELECT count(*) FROM bible_versions WHERE status='active' AND api_exposure_allowed=TRUE AND deleted_at IS NULL`).Scan(&translations)
	_ = h.db.QueryRowContext(ctx, `SELECT count(*) FROM bible_versions WHERE status='pending_review' AND deleted_at IS NULL`).Scan(&pending)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"database": map[string]any{"healthy": dbOK}, "provider": map[string]any{"healthy": remoteOK, "error_code": providerErr, "latency_ms": time.Since(start).Milliseconds()}, "catalog": map[string]any{"active_languages": languages, "active_translations": translations, "pending_review": pending}, "checked_at": time.Now().UTC()})
}

func (h *Handler) adminBibleCatalog(w http.ResponseWriter, r *http.Request) {
	provider := h.bibleDiscovery
	if provider == nil {
		provider = h.bible
	}
	language := strings.TrimSpace(r.URL.Query().Get("language"))
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	rows, providerErr := provider.GetTranslations(r.Context(), language, query)
	// This is an authenticated review dashboard, not the public catalog. An
	// upstream outage must not erase locally recorded pending decisions from the
	// admin queue. Approvals still re-verify the live upstream digest and fail
	// closed; the response explicitly marks all records as offline metadata.
	stored, err := h.db.QueryContext(r.Context(), `SELECT id,status,api_exposure_allowed FROM bible_versions
		WHERE provider='helloao' AND deleted_at IS NULL
		AND (?='' OR lower(language)=lower(?) OR lower(language_name)=lower(?))
		AND (?='' OR name ILIKE '%'||?||'%' OR abbrev ILIKE '%'||?||'%' OR language_name ILIKE '%'||?||'%')
		ORDER BY name`, language, language, language, query, query, query, query)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to load translation review status.")
		return
	}
	defer stored.Close()
	items := make([]map[string]any, 0, len(rows))
	byID := map[string]map[string]any{}
	for _, t := range rows {
		id := helloAOAppID(t.ProviderTranslationID)
		item := map[string]any{"translation": t, "registry_id": id, "status": "not_synced"}
		items = append(items, item)
		byID[id] = item
	}
	registry := make([]struct {
		id, status string
		exposed    bool
	}, 0)
	for stored.Next() {
		var id, status string
		var exposed bool
		if err := stored.Scan(&id, &status, &exposed); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "Unable to read translation review status.")
			return
		}
		registry = append(registry, struct {
			id, status string
			exposed    bool
		}{id, status, exposed})
	}
	if err := stored.Err(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to read translation review records.")
		return
	}
	_ = stored.Close() // free the cursor before reading individual records
	local := &bible.LocalBibleProvider{DB: h.db}
	for _, entry := range registry {
		item := byID[entry.id]
		if item == nil {
			record, err := local.GetTranslationRecord(r.Context(), entry.id)
			if err != nil {
				httpx.WriteError(w, http.StatusInternalServerError, "Unable to read reviewed translation metadata.")
				return
			}
			item = map[string]any{"translation": record, "registry_id": entry.id, "remote_unavailable": true}
			items = append(items, item)
		}
		item["status"] = entry.status
		item["api_exposure_allowed"] = entry.exposed
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"provider": "helloao", "provider_available": providerErr == nil,
		"translations": items, "count": len(items),
	})
}

func (h *Handler) adminSyncBibleCatalog(w http.ResponseWriter, r *http.Request) {
	provider := h.bibleDiscovery
	if provider == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "Bible discovery provider is not configured.")
		return
	}
	translations, err := provider.GetTranslations(r.Context(), "", "")
	if err != nil {
		httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"code": "BIBLE_PROVIDER_UNAVAILABLE", "error": "The upstream translation catalog is temporarily unavailable."})
		return
	}
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to start catalog synchronization.")
		return
	}
	defer tx.Rollback()
	inserted, updated := 0, 0
	for _, t := range translations {
		id := helloAOAppID(t.ProviderTranslationID)
		digest := strings.ToLower(strings.TrimSpace(t.ContentHash))
		digestBytes, digestErr := hex.DecodeString(digest)
		if len(id) < 10 || digestErr != nil || len(digestBytes) != sha256.Size {
			// A provider edition with no verifiable source identity cannot
			// become a reviewed/public translation.
			continue
		}
		iso3, knownLanguage := languageISO6393(t.Language)
		if !knownLanguage {
			continue
		}
		locale := t.Locale
		if locale == "" {
			locale = t.Language
		}
		_, err = tx.ExecContext(r.Context(), `INSERT INTO bible_languages(id,iso639_1,iso639_2,iso639_3,bcp47,name,native_name,direction,status) VALUES(?,?,?,?,?,?,?,?, 'pending_review') ON CONFLICT(id) DO NOTHING`, t.Language, t.Language, iso3, iso3, locale, t.LanguageName, t.LanguageName, t.Direction)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "Unable to register a discovered language.")
			return
		}
		var exists bool
		_ = tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM bible_versions WHERE id=?)`, id).Scan(&exists)
		website := strings.TrimSpace(t.SourceURL)
		if website == "" {
			website = "https://bible.helloao.org/" + url.PathEscape(t.ProviderTranslationID)
		}
		attribution := "HelloAO translation " + t.ProviderTranslationID
		_, err = tx.ExecContext(r.Context(), `INSERT INTO bible_versions(id,name,abbrev,language,language_name,format,coverage,licence,licence_url,licence_note,attribution,blob_url,sha256,created_at,updated_at,provider,provider_translation_id,locale,publisher,description,copyright_text,attribution_text,source_url,import_version,book_count,chapter_count,verse_count) VALUES(?,?,?,?,?,'helloao-json','unknown',?,?, 'Rights pending review',?,?, '',?,?,'helloao',?,?,?,?,?,?,?, 'catalog-discovery-v1',?,?,?) ON CONFLICT(id) DO UPDATE SET name=EXCLUDED.name,abbrev=EXCLUDED.abbrev,language=EXCLUDED.language,language_name=EXCLUDED.language_name,licence_url=EXCLUDED.licence_url,provider_translation_id=EXCLUDED.provider_translation_id,locale=EXCLUDED.locale,publisher=EXCLUDED.publisher,description=EXCLUDED.description,source_url=EXCLUDED.source_url,book_count=EXCLUDED.book_count,chapter_count=EXCLUDED.chapter_count,verse_count=EXCLUDED.verse_count,updated_at=EXCLUDED.updated_at WHERE bible_versions.provider='helloao'`, id, t.Name, t.Abbreviation, t.Language, t.LanguageName, t.License, t.LicenseURL, attribution, website, time.Now().UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano), t.ProviderTranslationID, locale, t.Publisher, t.Description, t.Copyright, attribution, website, t.BookCount, t.ChapterCount, t.VerseCount)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "Unable to synchronize discovered translation metadata.")
			return
		}
		// Sync may pin a never-reviewed discovery, but must never silently
		// replace the digest of an already-reviewed remote edition.
		if _, err := tx.ExecContext(r.Context(), `UPDATE bible_versions
			SET sha256=?,source_version=?,content_hash=?
			WHERE id=? AND provider='helloao' AND status='pending_review' AND reviewed_at IS NULL`,
			digest, digest, digest, id); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "Unable to pin discovered translation checksum.")
			return
		}
		if exists {
			updated++
		} else {
			inserted++
		}
	}
	if err := tx.Commit(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to finish catalog synchronization.")
		return
	}
	h.recordAudit(r, "bible_catalog_synced", "bible_provider", "helloao", fmt.Sprintf("inserted=%d updated=%d", inserted, updated), "success")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"provider": "helloao", "discovered": len(translations), "inserted": inserted, "updated": updated, "status": "pending_review_by_default"})
}

type bibleRightsReviewInput struct {
	Decision              string `json:"decision"`
	PublicDomain          *bool  `json:"public_domain"`
	CommercialUse         *bool  `json:"commercial_use"`
	RedistributionAllowed *bool  `json:"redistribution_allowed"`
	ModificationAllowed   *bool  `json:"modification_allowed"`
	AudioAllowed          *bool  `json:"audio_allowed"`
	OfflineAllowed        *bool  `json:"offline_allowed"`
	OfflineMaxDays        int    `json:"offline_max_days"`
	CopyAllowed           *bool  `json:"copy_allowed"`
	ShareAllowed          *bool  `json:"share_allowed"`
	SearchIndexAllowed    *bool  `json:"search_index_allowed"`
	APIExposureAllowed    *bool  `json:"api_exposure_allowed"`
	AttributionRequired   *bool  `json:"attribution_required"`
	AttributionText       string `json:"attribution_text"`
	EvidenceURL           string `json:"evidence_url"`
	EvidenceSHA256        string `json:"evidence_sha256"`
	Rationale             string `json:"rationale"`
}

func (h *Handler) adminReviewBibleRights(w http.ResponseWriter, r *http.Request) {
	var in bibleRightsReviewInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "Invalid rights review.")
		return
	}
	if in.Decision != "approved" && in.Decision != "rejected" && in.Decision != "suspended" {
		httpx.WriteError(w, http.StatusBadRequest, "Choose approved, rejected, or suspended.")
		return
	}
	if in.PublicDomain == nil || in.CommercialUse == nil || in.RedistributionAllowed == nil || in.ModificationAllowed == nil || in.AudioAllowed == nil || in.OfflineAllowed == nil || in.CopyAllowed == nil || in.ShareAllowed == nil || in.SearchIndexAllowed == nil || in.APIExposureAllowed == nil || in.AttributionRequired == nil {
		httpx.WriteError(w, http.StatusBadRequest, "Every rights permission must be submitted explicitly.")
		return
	}
	evidence, err := url.Parse(strings.TrimSpace(in.EvidenceURL))
	hashValid := true
	if in.EvidenceSHA256 != "" {
		decoded, hashErr := hex.DecodeString(in.EvidenceSHA256)
		hashValid = hashErr == nil && len(decoded) == sha256.Size
	}
	if err != nil || evidence.Scheme != "https" || evidence.Host == "" || strings.TrimSpace(in.Rationale) == "" || len(in.Rationale) > 4000 || in.OfflineMaxDays < 0 || in.OfflineMaxDays > 3650 || (*in.OfflineAllowed && in.OfflineMaxDays < 1) || (*in.AttributionRequired && strings.TrimSpace(in.AttributionText) == "") || !hashValid {
		httpx.WriteError(w, http.StatusBadRequest, "Provide HTTPS evidence, an optional valid SHA-256, rationale, required attribution text, and valid offline terms.")
		return
	}
	id := r.PathValue("id")
	var provider, providerID, pinnedDigest string
	if err := h.db.QueryRowContext(r.Context(), `SELECT provider,COALESCE(provider_translation_id,id),COALESCE(content_hash,'')
		FROM bible_versions WHERE id=? AND deleted_at IS NULL`, id).Scan(&provider, &providerID, &pinnedDigest); err == sql.ErrNoRows {
		httpx.WriteError(w, http.StatusNotFound, "Translation not found.")
		return
	} else if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to load translation.")
		return
	}
	if provider == "helloao" && in.Decision == "approved" {
		digestBytes, digestErr := hex.DecodeString(pinnedDigest)
		if digestErr != nil || len(digestBytes) != sha256.Size {
			httpx.WriteError(w, http.StatusConflict, "Sync a provider edition with a pinned source checksum before granting rights.")
			return
		}
		if h.bibleDiscovery == nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "Provider verification is not configured.")
			return
		}
		upstream, err := h.bibleDiscovery.GetTranslation(r.Context(), providerID)
		if err != nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "Provider source cannot be verified right now.")
			return
		}
		if !strings.EqualFold(upstream.ContentHash, pinnedDigest) {
			httpx.WriteError(w, http.StatusConflict, "Provider text changed since discovery; pin a new edition before reviewing rights.")
			return
		}
	}
	status := in.Decision
	if status == "approved" {
		status = "active"
	}
	if status == "active" && !*in.APIExposureAllowed {
		status = "pending_review"
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	actor := h.userID(r)
	grants := map[string]any{"public_domain": *in.PublicDomain, "commercial_use": *in.CommercialUse, "redistribution_allowed": *in.RedistributionAllowed, "modification_allowed": *in.ModificationAllowed, "audio_allowed": *in.AudioAllowed, "offline_allowed": *in.OfflineAllowed, "offline_max_days": in.OfflineMaxDays, "copy_allowed": *in.CopyAllowed, "share_allowed": *in.ShareAllowed, "search_index_allowed": *in.SearchIndexAllowed, "api_exposure_allowed": *in.APIExposureAllowed, "attribution_required": *in.AttributionRequired}
	grantJSON, _ := json.Marshal(grants)
	reviewID := uuid.NewString()
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to save rights decision.")
		return
	}
	defer tx.Rollback()
	approved := in.Decision == "approved"
	offlineDays := 0
	if approved {
		offlineDays = in.OfflineMaxDays
	}
	_, err = tx.ExecContext(r.Context(), `UPDATE bible_versions SET public_domain=?,commercial_use=?,redistribution_allowed=?,modification_allowed=?,audio_allowed=?,offline_allowed=?,offline_max_days=?,copy_allowed=?,share_allowed=?,search_index_allowed=?,api_exposure_allowed=?,attribution_required=?,attribution_text=?,status=?,reviewed_by=?,reviewed_at=?,row_version=row_version+1,updated_at=? WHERE id=?`, approved && *in.PublicDomain, approved && *in.CommercialUse, approved && *in.RedistributionAllowed, approved && *in.ModificationAllowed, approved && *in.AudioAllowed, approved && *in.OfflineAllowed, offlineDays, approved && *in.CopyAllowed, approved && *in.ShareAllowed, approved && *in.SearchIndexAllowed, approved && *in.APIExposureAllowed, !approved || *in.AttributionRequired, strings.TrimSpace(in.AttributionText), status, actor, now, now, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to save rights decision.")
		return
	}
	_, err = tx.ExecContext(r.Context(), `INSERT INTO bible_rights_reviews(id,translation_id,actor_id,decision,grants,evidence_url,evidence_sha256,rationale,created_at) VALUES(?,?,?,?,?::jsonb,?,?,?,?)`, reviewID, id, actor, in.Decision, string(grantJSON), in.EvidenceURL, strings.TrimSpace(in.EvidenceSHA256), strings.TrimSpace(in.Rationale), now)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to save rights audit record.")
		return
	}
	if err = tx.Commit(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to commit rights decision.")
		return
	}
	h.recordAudit(r, "bible_translation_rights_reviewed", "bible_translation", id, "decision="+in.Decision+" provider="+provider, "success")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "status": status, "review_id": reviewID, "grants": grants, "reviewed_at": now})
}

func sha256Hex(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
