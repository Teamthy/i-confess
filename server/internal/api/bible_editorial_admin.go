package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Teamthy/i-confess/internal/bible"
	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/google/uuid"
)

var biblePlanSlug = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func (h *Handler) adminGetBiblePlan(w http.ResponseWriter, r *http.Request) {
	var plan map[string]any
	plans, err := readBibleMaps(r.Context(), h.db, `SELECT id,slug,title,description,language,duration_days,source_note,status,created_by,reviewed_by,reviewed_at,created_at,updated_at FROM bible_reading_plans WHERE id=?`, r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to load Bible plan.")
		return
	}
	if len(plans) == 0 {
		httpx.WriteError(w, http.StatusNotFound, "Bible plan not found.")
		return
	}
	plan = plans[0]
	rows, err := h.db.QueryContext(r.Context(), `SELECT day_number,title,passage_references FROM bible_reading_plan_days WHERE plan_id=? ORDER BY day_number`, r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to load Bible plan days.")
		return
	}
	defer rows.Close()
	days := make([]map[string]any, 0)
	for rows.Next() {
		var number int
		var title string
		var raw []byte
		if err := rows.Scan(&number, &title, &raw); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "Unable to read Bible plan day.")
			return
		}
		refs := []string{}
		if err := json.Unmarshal(raw, &refs); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "Bible plan contains invalid references.")
			return
		}
		days = append(days, map[string]any{"day_number": number, "title": title, "references": refs})
	}
	if err := rows.Err(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to read Bible plan days.")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan": plan, "days": days})
}

func (h *Handler) adminListBiblePlans(w http.ResponseWriter, r *http.Request) {
	items, err := readBibleMaps(r.Context(), h.db, `SELECT id,slug,title,description,language,duration_days,source_note,status,created_by,reviewed_by,reviewed_at,created_at,updated_at FROM bible_reading_plans ORDER BY updated_at DESC`)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to load Bible plans.")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plans": items})
}

type biblePlanDayInput struct {
	DayNumber  int      `json:"day_number"`
	Title      string   `json:"title"`
	References []string `json:"references"`
}
type biblePlanInput struct {
	Slug         string              `json:"slug"`
	Title        string              `json:"title"`
	Description  string              `json:"description"`
	Language     string              `json:"language"`
	DurationDays int                 `json:"duration_days"`
	SourceNote   string              `json:"source_note"`
	Days         []biblePlanDayInput `json:"days"`
}

func validateBiblePlan(in biblePlanInput) error {
	if !biblePlanSlug.MatchString(in.Slug) || strings.TrimSpace(in.Title) == "" || len(in.Title) > 160 || len(in.Description) > 2000 || len(in.SourceNote) < 12 || len(in.SourceNote) > 1000 || in.DurationDays < 1 || in.DurationDays > 366 || len(in.Days) != in.DurationDays {
		return http.ErrNotSupported
	}
	seen := map[int]bool{}
	for _, day := range in.Days {
		if day.DayNumber < 1 || day.DayNumber > in.DurationDays || seen[day.DayNumber] || len(day.References) < 1 || len(day.References) > 8 {
			return http.ErrNotSupported
		}
		seen[day.DayNumber] = true
		for _, ref := range day.References {
			if _, err := bible.ParseReference(ref); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *Handler) adminCreateBiblePlan(w http.ResponseWriter, r *http.Request) {
	var in biblePlanInput
	if err := httpx.DecodeJSON(r, &in); err != nil || validateBiblePlan(in) != nil {
		httpx.WriteError(w, http.StatusBadRequest, "Plan metadata and every canonical reading day must be complete and valid.")
		return
	}
	var languageActive bool
	_ = h.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM bible_languages WHERE id=? AND status='active')`, in.Language).Scan(&languageActive)
	if !languageActive {
		httpx.WriteError(w, http.StatusBadRequest, "Choose an active language.")
		return
	}
	id := uuid.NewString()
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to create plan.")
		return
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(r.Context(), `INSERT INTO bible_reading_plans(id,slug,title,description,language,duration_days,source_note,status,created_by) VALUES(?,?,?,?,?,?,?,'draft',?)`, id, in.Slug, strings.TrimSpace(in.Title), strings.TrimSpace(in.Description), in.Language, in.DurationDays, strings.TrimSpace(in.SourceNote), h.userID(r))
	if err != nil {
		httpx.WriteError(w, http.StatusConflict, "A plan with that slug already exists or the metadata is invalid.")
		return
	}
	for _, day := range in.Days {
		refs, _ := json.Marshal(day.References)
		_, err = tx.ExecContext(r.Context(), `INSERT INTO bible_reading_plan_days(plan_id,day_number,title,passage_references) VALUES(?,?,?,?::jsonb)`, id, day.DayNumber, strings.TrimSpace(day.Title), string(refs))
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "Unable to save a reading day.")
			return
		}
	}
	if err = tx.Commit(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to commit plan.")
		return
	}
	h.recordAudit(r, "bible_plan_created", "bible_plan", id, "status=draft", "success")
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"id": id, "slug": in.Slug, "status": "draft"})
}

func (h *Handler) adminUpdateBiblePlan(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Status    string `json:"status"`
		Rationale string `json:"rationale"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil || !oneOf(in.Status, "draft", "review", "published", "archived") || len(strings.TrimSpace(in.Rationale)) < 8 {
		httpx.WriteError(w, http.StatusBadRequest, "Provide a supported status and review rationale.")
		return
	}
	id := r.PathValue("id")
	var duration int
	var creator sql.NullString
	err := h.db.QueryRowContext(r.Context(), `SELECT duration_days,created_by FROM bible_reading_plans WHERE id=?`, id).Scan(&duration, &creator)
	if err == sql.ErrNoRows {
		httpx.WriteError(w, http.StatusNotFound, "Reading plan not found.")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to load plan.")
		return
	}
	if in.Status == "published" {
		var count int
		_ = h.db.QueryRowContext(r.Context(), `SELECT count(*) FROM bible_reading_plan_days WHERE plan_id=?`, id).Scan(&count)
		if count != duration {
			httpx.WriteError(w, http.StatusConflict, "A published plan needs exactly one valid reading day per day of its duration.")
			return
		}
		if creator.Valid && creator.String == h.userID(r) {
			httpx.WriteError(w, http.StatusForbidden, "A different admin must review and publish a plan you created.")
			return
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var reviewActor any
	var reviewedAt any
	if in.Status == "published" {
		reviewActor = h.userID(r)
		reviewedAt = now
	}
	_, err = h.db.ExecContext(r.Context(), `UPDATE bible_reading_plans SET status=?,reviewed_by=COALESCE(?,reviewed_by),reviewed_at=COALESCE(?,reviewed_at),updated_at=? WHERE id=?`, in.Status, reviewActor, reviewedAt, now, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to update plan status.")
		return
	}
	h.recordAudit(r, "bible_plan_status_changed", "bible_plan", id, "status="+in.Status+" rationale="+strings.TrimSpace(in.Rationale), "success")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "status": in.Status, "reviewed_at": reviewedAt})
}

func (h *Handler) adminAddBiblePlanDay(w http.ResponseWriter, r *http.Request) {
	var in biblePlanDayInput
	if err := httpx.DecodeJSON(r, &in); err != nil || in.DayNumber < 1 || len(in.References) < 1 || len(in.References) > 8 {
		httpx.WriteError(w, http.StatusBadRequest, "Provide a valid day and canonical references.")
		return
	}
	for _, ref := range in.References {
		if _, err := bible.ParseReference(ref); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "One or more references are invalid.")
			return
		}
	}
	id := r.PathValue("id")
	var status string
	var duration int
	if err := h.db.QueryRowContext(r.Context(), `SELECT status,duration_days FROM bible_reading_plans WHERE id=?`, id).Scan(&status, &duration); err == sql.ErrNoRows {
		httpx.WriteError(w, http.StatusNotFound, "Reading plan not found.")
		return
	} else if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to load plan.")
		return
	}
	if status == "published" || in.DayNumber > duration {
		httpx.WriteError(w, http.StatusConflict, "Unpublish the plan and use an in-range day before editing.")
		return
	}
	refs, _ := json.Marshal(in.References)
	_, err := h.db.ExecContext(r.Context(), `INSERT INTO bible_reading_plan_days(plan_id,day_number,title,passage_references) VALUES(?,?,?,?::jsonb) ON CONFLICT(plan_id,day_number) DO UPDATE SET title=EXCLUDED.title,passage_references=EXCLUDED.passage_references`, id, in.DayNumber, strings.TrimSpace(in.Title), string(refs))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to save reading day.")
		return
	}
	h.recordAudit(r, "bible_plan_day_updated", "bible_plan", id, "day="+strconv.Itoa(in.DayNumber), "success")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan_id": id, "day_number": in.DayNumber, "references": in.References})
}

func (h *Handler) adminListBibleVerseOfDay(w http.ResponseWriter, r *http.Request) {
	items, err := readBibleMaps(r.Context(), h.db, `SELECT day_index,book_id,chapter,verse,editor_note,reviewed_by,reviewed_at,review_evidence_url FROM bible_verse_of_day ORDER BY day_index`)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to load verse-of-day review queue.")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entries": items})
}

func (h *Handler) adminSetBibleVerseOfDay(w http.ResponseWriter, r *http.Request) {
	day, err := strconv.Atoi(r.PathValue("day"))
	var in struct {
		Reference  string `json:"reference"`
		EditorNote string `json:"editor_note"`
		Evidence   string `json:"evidence"`
	}
	if err != nil || day < 1 || day > 7 || httpx.DecodeJSON(r, &in) != nil {
		httpx.WriteError(w, http.StatusBadRequest, "Invalid verse-of-day entry.")
		return
	}
	ref, err := bible.ParseReference(in.Reference)
	evidence, evidenceErr := url.Parse(strings.TrimSpace(in.Evidence))
	if err != nil || ref.StartVerse < 1 || ref.EndVerse != ref.StartVerse || len(in.EditorNote) > 300 || evidenceErr != nil || evidence.Scheme != "https" || evidence.Host == "" || len(in.Evidence) > 2000 {
		httpx.WriteError(w, http.StatusBadRequest, "Provide a single canonical verse, short editor note, and HTTPS review evidence.")
		return
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = h.db.ExecContext(r.Context(), `INSERT INTO bible_verse_of_day(day_index,book_id,chapter,verse,editor_note,reviewed_by,reviewed_at,review_evidence_url) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(day_index) DO UPDATE SET book_id=EXCLUDED.book_id,chapter=EXCLUDED.chapter,verse=EXCLUDED.verse,editor_note=EXCLUDED.editor_note,reviewed_by=EXCLUDED.reviewed_by,reviewed_at=EXCLUDED.reviewed_at,review_evidence_url=EXCLUDED.review_evidence_url,row_version=bible_verse_of_day.row_version+1`, day, ref.Book, ref.Chapter, ref.StartVerse, in.EditorNote, h.userID(r), now, strings.TrimSpace(in.Evidence))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to save verse of the day.")
		return
	}
	h.recordAudit(r, "bible_verse_of_day_reviewed", "bible_verse_of_day", strconv.Itoa(day), strings.TrimSpace(in.Evidence), "success")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"day_index": day, "reference": in.Reference, "reviewed_at": now})
}

// Cross-references are editorial pointers, never replacements for source verse
// text. Seeded suggestions and newly proposed pointers are hidden from public
// readers until a different admin supplies an explicit review and evidence.
func (h *Handler) adminListBibleCrossReferences(w http.ResponseWriter, r *http.Request) {
	items, err := readBibleMaps(r.Context(), h.db, `SELECT id,source_book_id,source_chapter,source_verse_start,source_verse_end,target_reference,
		editor_note,created_by,reviewed_by,reviewed_at,review_evidence_url,review_rationale
		FROM bible_cross_references WHERE deleted_at IS NULL ORDER BY id`)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to load cross-reference review queue.")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"references": items})
}

func (h *Handler) adminCreateBibleCrossReference(w http.ResponseWriter, r *http.Request) {
	var in struct {
		SourceReference string `json:"source_reference"`
		TargetReference string `json:"target_reference"`
		EditorNote      string `json:"editor_note"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "Provide two canonical references and a short editorial note.")
		return
	}
	source, sourceErr := bible.ParseReference(in.SourceReference)
	_, targetErr := bible.ParseReference(in.TargetReference)
	if sourceErr != nil || source.StartVerse < 1 || targetErr != nil || len(in.TargetReference) > 120 || len(in.EditorNote) > 300 {
		httpx.WriteError(w, http.StatusBadRequest, "Provide valid source verse(s), a target passage, and a short editorial note.")
		return
	}
	var id int64
	err := h.db.QueryRowContext(r.Context(), `INSERT INTO bible_cross_references
		(source_book_id,source_chapter,source_verse_start,source_verse_end,target_reference,editor_note,created_by)
		VALUES(?,?,?,?,?,?,?) RETURNING id`, source.Book, source.Chapter, source.StartVerse, source.EndVerse,
		strings.TrimSpace(in.TargetReference), strings.TrimSpace(in.EditorNote), h.userID(r)).Scan(&id)
	if err != nil {
		httpx.WriteError(w, http.StatusConflict, "This cross-reference already exists or cannot be proposed.")
		return
	}
	h.recordAudit(r, "bible_cross_reference_proposed", "bible_cross_reference", strconv.FormatInt(id, 10), "status=review", "success")
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "review"})
}

func (h *Handler) adminReviewBibleCrossReference(w http.ResponseWriter, r *http.Request) {
	id, idErr := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var in struct {
		Decision    string `json:"decision"`
		EvidenceURL string `json:"evidence_url"`
		Rationale   string `json:"rationale"`
	}
	if idErr != nil || id < 1 || httpx.DecodeJSON(r, &in) != nil {
		httpx.WriteError(w, http.StatusBadRequest, "Invalid cross-reference review.")
		return
	}
	evidence, err := url.Parse(strings.TrimSpace(in.EvidenceURL))
	if !oneOf(in.Decision, "approve", "withdraw") || err != nil || evidence.Scheme != "https" || evidence.Host == "" || len(in.EvidenceURL) > 2000 || len(strings.TrimSpace(in.Rationale)) < 8 || len(in.Rationale) > 2000 {
		httpx.WriteError(w, http.StatusBadRequest, "Provide an approval or withdrawal, HTTPS evidence and a review rationale.")
		return
	}
	var result sql.Result
	if in.Decision == "approve" {
		result, err = h.db.ExecContext(r.Context(), `UPDATE bible_cross_references
			SET reviewed_by=?,reviewed_at=?,review_evidence_url=?,review_rationale=?,row_version=row_version+1
			WHERE id=? AND deleted_at IS NULL AND (created_by IS NULL OR created_by<>?)`,
			h.userID(r), time.Now().UTC().Format(time.RFC3339Nano), strings.TrimSpace(in.EvidenceURL), strings.TrimSpace(in.Rationale), id, h.userID(r))
	} else {
		// Emergency withdrawal does not require a different actor. A proposal
		// author may always revoke their own pointer, never approve it.
		result, err = h.db.ExecContext(r.Context(), `UPDATE bible_cross_references
			SET reviewed_by=NULL,reviewed_at=NULL,review_evidence_url=?,review_rationale=?,row_version=row_version+1
			WHERE id=? AND deleted_at IS NULL`, strings.TrimSpace(in.EvidenceURL), strings.TrimSpace(in.Rationale), id)
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to record the cross-reference review.")
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		// Fail closed for both missing references and attempted self-approval.
		httpx.WriteError(w, http.StatusNotFound, "Cross-reference not found or requires another reviewer.")
		return
	}
	h.recordAudit(r, "bible_cross_reference_reviewed", "bible_cross_reference", strconv.FormatInt(id, 10), "decision="+in.Decision, "success")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "decision": in.Decision})
}
