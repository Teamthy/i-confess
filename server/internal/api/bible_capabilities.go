package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Teamthy/i-confess/internal/bible"
	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/rights"
	"github.com/google/uuid"
)

var bibleDeviceIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{3,128}$`)

func (h *Handler) bibleCompare(w http.ResponseWriter, r *http.Request) {
	ref := strings.TrimSpace(r.URL.Query().Get("reference"))
	if _, err := bible.ParseReference(ref); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "Enter a valid Bible reference.")
		return
	}
	ids := strings.Split(r.URL.Query().Get("translations"), ",")
	if len(ids) < 2 || len(ids) > 4 {
		httpx.WriteError(w, http.StatusBadRequest, "Choose between two and four translations.")
		return
	}
	passages := make([]bible.Passage, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if err := h.readableBibleTranslation(r.Context(), id); err != nil {
			h.bibleError(w, err)
			return
		}
		passage, err := h.bible.GetPassage(r.Context(), id, ref)
		if err != nil {
			h.bibleError(w, err)
			return
		}
		passages = append(passages, passage)
	}
	if len(passages) < 2 {
		httpx.WriteError(w, http.StatusBadRequest, "Choose at least two distinct translations.")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"reference": ref, "passages": passages})
}

func (h *Handler) bibleVerseOfDay(w http.ResponseWriter, r *http.Request) {
	translation := strings.TrimSpace(r.URL.Query().Get("translation"))
	if translation == "" {
		httpx.WriteError(w, http.StatusBadRequest, "Choose a Bible translation.")
		return
	}
	if err := h.readableBibleTranslation(r.Context(), translation); err != nil {
		h.bibleError(w, err)
		return
	}
	// A reviewed seven-day editorial sequence is selected in UTC. The exact
	// canonical verse is resolved from the chosen source translation at request time.
	index := (time.Now().UTC().YearDay()-1)%7 + 1
	var book string
	var chapter, verse int
	var note string
	err := h.db.QueryRowContext(r.Context(), `SELECT book_id,chapter,verse,editor_note FROM bible_verse_of_day WHERE day_index=? AND reviewed_at IS NOT NULL AND deleted_at IS NULL`, index).Scan(&book, &chapter, &verse, &note)
	if err != nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "The verse of the day is not configured.")
		return
	}
	v, err := h.bible.GetVerse(r.Context(), translation, book, chapter, verse)
	if err != nil {
		h.bibleError(w, err)
		return
	}
	t, err := h.bible.GetTranslation(r.Context(), translation)
	if err != nil {
		h.bibleError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"date": time.Now().UTC().Format("2006-01-02"), "editor_note": note, "translation": t, "verse": v, "reference": v.ID})
}

func (h *Handler) biblePlans(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.QueryContext(r.Context(), `SELECT p.id,p.slug,p.title,p.description,p.language,p.duration_days,p.source_note,(SELECT count(*) FROM bible_reading_plan_days d WHERE d.plan_id=p.id AND d.deleted_at IS NULL) FROM bible_reading_plans p WHERE p.status='published' AND p.reviewed_at IS NOT NULL AND p.deleted_at IS NULL ORDER BY p.duration_days,p.title`)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to load reading plans.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, slug, title, description, language, source string
		var days, count int
		if err := rows.Scan(&id, &slug, &title, &description, &language, &days, &source, &count); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "Unable to load reading plans.")
			return
		}
		items = append(items, map[string]any{"id": id, "slug": slug, "title": title, "description": description, "language": language, "duration_days": days, "reading_count": count, "source_note": source})
	}
	if err := rows.Err(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to load reading plans.")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plans": items})
}

func (h *Handler) biblePlan(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	out := map[string]any{}
	var id, title, description, language, source string
	var duration int
	err := h.db.QueryRowContext(r.Context(), `SELECT id,title,description,language,duration_days,source_note FROM bible_reading_plans WHERE slug=? AND status='published' AND reviewed_at IS NOT NULL AND deleted_at IS NULL`, slug).Scan(&id, &title, &description, &language, &duration, &source)
	if err == sql.ErrNoRows {
		httpx.WriteError(w, http.StatusNotFound, "Reading plan not found.")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to load reading plan.")
		return
	}
	rows, err := h.db.QueryContext(r.Context(), `SELECT day_number,title,passage_references FROM bible_reading_plan_days WHERE plan_id=? AND deleted_at IS NULL ORDER BY day_number`, id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to load reading plan days.")
		return
	}
	defer rows.Close()
	days := []map[string]any{}
	for rows.Next() {
		var number int
		var dayTitle string
		var raw []byte
		if err := rows.Scan(&number, &dayTitle, &raw); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "Unable to load reading plan days.")
			return
		}
		refs := []string{}
		if err := json.Unmarshal(raw, &refs); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "Reading plan contains invalid references.")
			return
		}
		for _, ref := range refs {
			if _, err := bible.ParseReference(ref); err != nil {
				httpx.WriteError(w, http.StatusInternalServerError, "Reading plan contains an invalid canonical reference.")
				return
			}
		}
		days = append(days, map[string]any{"day_number": number, "title": dayTitle, "references": refs})
	}
	if err := rows.Err(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to load reading plan days.")
		return
	}
	out["plan"] = map[string]any{"id": id, "slug": slug, "title": title, "description": description, "language": language, "duration_days": duration, "source_note": source, "days": days}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) enrollBiblePlan(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TranslationID string `json:"translation_id"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil || in.TranslationID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "Choose a Bible translation.")
		return
	}
	if err := h.readableBibleTranslation(r.Context(), in.TranslationID); err != nil {
		h.bibleError(w, err)
		return
	}
	planID := r.PathValue("id")
	var status string
	if err := h.db.QueryRowContext(r.Context(), `SELECT status FROM bible_reading_plans WHERE id=? AND reviewed_at IS NOT NULL AND deleted_at IS NULL`, planID).Scan(&status); err == sql.ErrNoRows || status != "published" {
		httpx.WriteError(w, http.StatusNotFound, "Reading plan not found.")
		return
	} else if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to load reading plan.")
		return
	}
	id := uuid.NewString()
	var savedID string
	var currentDay, version int
	err := h.db.QueryRowContext(r.Context(), `INSERT INTO user_bible_plan_enrollments(id,user_id,plan_id,translation_id) VALUES(?,?,?,?) ON CONFLICT(user_id,plan_id) DO UPDATE SET translation_id=EXCLUDED.translation_id,row_version=user_bible_plan_enrollments.row_version+1 RETURNING id,current_day,row_version`, id, h.userID(r), planID, in.TranslationID).Scan(&savedID, &currentDay, &version)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to start this reading plan.")
		return
	}
	h.recordAudit(r, "bible_plan_enrolled", "bible_plan", planID, "", "success")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"id": savedID, "plan_id": planID, "translation_id": in.TranslationID, "current_day": currentDay, "row_version": version})
}

func (h *Handler) myBiblePlans(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.QueryContext(r.Context(), `SELECT e.id,e.plan_id,p.slug,p.title,e.translation_id,e.current_day,e.started_at,e.completed_at,e.row_version,(SELECT count(*) FROM user_bible_plan_day_progress d WHERE d.enrollment_id=e.id) FROM user_bible_plan_enrollments e JOIN bible_reading_plans p ON p.id=e.plan_id WHERE e.user_id=? AND e.deleted_at IS NULL AND p.deleted_at IS NULL ORDER BY e.started_at DESC`, h.userID(r))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to load your reading plans.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, planID, slug, title, translation, started string
		var day, version, completed int
		var finished sql.NullString
		if err := rows.Scan(&id, &planID, &slug, &title, &translation, &day, &started, &finished, &version, &completed); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "Unable to load your reading plans.")
			return
		}
		items = append(items, map[string]any{"id": id, "plan_id": planID, "slug": slug, "title": title, "translation_id": translation, "current_day": day, "started_at": started, "completed_at": finished.String, "completed_days": completed, "row_version": version})
	}
	if err := rows.Err(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to load your reading plans.")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plans": items})
}

func (h *Handler) completeBiblePlanDay(w http.ResponseWriter, r *http.Request) {
	day, err := strconv.Atoi(r.PathValue("day"))
	if err != nil || day < 1 || day > 366 {
		httpx.WriteError(w, http.StatusBadRequest, "Invalid reading-plan day.")
		return
	}
	enrollmentID := r.PathValue("id")
	var planID string
	var duration int
	err = h.db.QueryRowContext(r.Context(), `SELECT e.plan_id,p.duration_days FROM user_bible_plan_enrollments e JOIN bible_reading_plans p ON p.id=e.plan_id WHERE e.id=? AND e.user_id=? AND e.deleted_at IS NULL AND p.status='published' AND p.reviewed_at IS NOT NULL AND p.deleted_at IS NULL`, enrollmentID, h.userID(r)).Scan(&planID, &duration)
	if err == sql.ErrNoRows {
		httpx.WriteError(w, http.StatusNotFound, "Reading plan enrollment not found.")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to update reading plan.")
		return
	}
	var exists bool
	_ = h.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM bible_reading_plan_days WHERE plan_id=? AND day_number=?)`, planID, day).Scan(&exists)
	if !exists {
		httpx.WriteError(w, http.StatusNotFound, "That day is not part of this plan.")
		return
	}
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to update reading plan.")
		return
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = tx.ExecContext(r.Context(), `INSERT INTO user_bible_plan_day_progress(enrollment_id,day_number,completed_at) VALUES(?,?,?) ON CONFLICT(enrollment_id,day_number) DO NOTHING`, enrollmentID, day, now)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to save reading progress.")
		return
	}
	_, err = tx.ExecContext(r.Context(), `UPDATE user_bible_plan_enrollments SET current_day=GREATEST(current_day,?),row_version=row_version+1,completed_at=CASE WHEN ? THEN COALESCE(completed_at,?) ELSE completed_at END WHERE id=?`, day+1, day >= duration, now, enrollmentID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to save reading progress.")
		return
	}
	if err = tx.Commit(); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to save reading progress.")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"enrollment_id": enrollmentID, "day_number": day, "completed_at": now})
}

type bibleQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func readBibleMaps(ctx context.Context, db bibleQueryer, query string, args ...any) ([]map[string]any, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		item := map[string]any{}
		for i, key := range cols {
			v := values[i]
			if b, ok := v.([]byte); ok {
				var decoded any
				if json.Unmarshal(b, &decoded) == nil {
					v = decoded
				} else {
					v = string(b)
				}
			}
			item[key] = v
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (h *Handler) getBiblePreferences(w http.ResponseWriter, r *http.Request) {
	var out map[string]any
	rows, err := readBibleMaps(r.Context(), h.db, `SELECT user_id,default_translation_id,preferred_language,font_size,line_height,theme,show_verse_numbers,updated_at,row_version FROM user_bible_preferences WHERE user_id=?`, h.userID(r))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to load Bible preferences.")
		return
	}
	if len(rows) > 0 {
		out = rows[0]
	} else {
		out = map[string]any{"font_size": 20, "line_height": 1.75, "theme": "light", "show_verse_numbers": true, "row_version": 0}
	}
	delete(out, "user_id")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"preferences": out})
}
func (h *Handler) putBiblePreferences(w http.ResponseWriter, r *http.Request) {
	var in struct {
		DefaultTranslationID *string `json:"default_translation_id"`
		PreferredLanguage    *string `json:"preferred_language"`
		FontSize             int     `json:"font_size"`
		LineHeight           float64 `json:"line_height"`
		Theme                string  `json:"theme"`
		ShowVerseNumbers     bool    `json:"show_verse_numbers"`
		RowVersion           int     `json:"row_version"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil || in.FontSize < 14 || in.FontSize > 40 || in.LineHeight < 1.2 || in.LineHeight > 2.5 || (in.Theme != "light" && in.Theme != "dark") {
		httpx.WriteError(w, http.StatusBadRequest, "Invalid Bible reading preferences.")
		return
	}
	translation := any(nil)
	if in.DefaultTranslationID != nil && strings.TrimSpace(*in.DefaultTranslationID) != "" {
		if err := h.readableBibleTranslation(r.Context(), *in.DefaultTranslationID); err != nil {
			h.bibleError(w, err)
			return
		}
		translation = strings.TrimSpace(*in.DefaultTranslationID)
	}
	language := any(nil)
	if in.PreferredLanguage != nil && strings.TrimSpace(*in.PreferredLanguage) != "" {
		var exists bool
		_ = h.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM bible_languages WHERE id=? AND status='active')`, strings.TrimSpace(*in.PreferredLanguage)).Scan(&exists)
		if !exists {
			httpx.WriteError(w, http.StatusBadRequest, "Choose an available interface language.")
			return
		}
		language = strings.TrimSpace(*in.PreferredLanguage)
	}
	var id string
	var version int
	err := h.db.QueryRowContext(r.Context(), `INSERT INTO user_bible_preferences(user_id,default_translation_id,preferred_language,font_size,line_height,theme,show_verse_numbers,updated_at,row_version) VALUES(?,?,?,?,?,?,?,?,1) ON CONFLICT(user_id) DO UPDATE SET default_translation_id=EXCLUDED.default_translation_id,preferred_language=EXCLUDED.preferred_language,font_size=EXCLUDED.font_size,line_height=EXCLUDED.line_height,theme=EXCLUDED.theme,show_verse_numbers=EXCLUDED.show_verse_numbers,updated_at=EXCLUDED.updated_at,row_version=user_bible_preferences.row_version+1 WHERE (?=0 OR user_bible_preferences.row_version=?) RETURNING user_id,row_version`, h.userID(r), translation, language, in.FontSize, in.LineHeight, in.Theme, in.ShowVerseNumbers, time.Now().UTC().Format(time.RFC3339Nano), in.RowVersion, in.RowVersion).Scan(&id, &version)
	if err == sql.ErrNoRows {
		httpx.WriteJSON(w, http.StatusConflict, map[string]string{"code": "BIBLE_PREFERENCE_CONFLICT", "error": "Preferences changed on another device. Sync and try again."})
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to save Bible preferences.")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"row_version": version})
}

func (h *Handler) getBibleHistory(w http.ResponseWriter, r *http.Request) {
	items, err := readBibleMaps(r.Context(), h.db, `SELECT id,translation_id,book_id,chapter,verse,scroll_offset,device_id,read_at,updated_at,row_version FROM user_bible_history WHERE user_id=? ORDER BY read_at DESC LIMIT 200`, h.userID(r))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to load reading history.")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"history": items})
}
func (h *Handler) recordBibleHistory(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TranslationID string  `json:"translation_id"`
		BookID        string  `json:"book_id"`
		Chapter       int     `json:"chapter"`
		Verse         int     `json:"verse"`
		ScrollOffset  float64 `json:"scroll_offset"`
		DeviceID      string  `json:"device_id"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil || in.Chapter < 1 || in.Chapter > 200 || in.Verse < 1 || in.Verse > 300 || in.ScrollOffset < 0 || in.ScrollOffset > 1 || !bibleDeviceIDPattern.MatchString(in.DeviceID) {
		httpx.WriteError(w, http.StatusBadRequest, "Invalid reading position.")
		return
	}
	if err := h.readableBibleTranslation(r.Context(), in.TranslationID); err != nil {
		h.bibleError(w, err)
		return
	}
	if _, err := h.bible.GetVerse(r.Context(), in.TranslationID, in.BookID, in.Chapter, in.Verse); err != nil {
		h.bibleError(w, err)
		return
	}
	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var saved string
	var version int
	err := h.db.QueryRowContext(r.Context(), `INSERT INTO user_bible_history(id,user_id,translation_id,book_id,chapter,verse,scroll_offset,device_id,read_at,updated_at,row_version) VALUES(?,?,?,?,?,?,?,?,?,?,1) ON CONFLICT(user_id,translation_id,book_id,chapter) WHERE translation_id IS NOT NULL DO UPDATE SET verse=EXCLUDED.verse,scroll_offset=EXCLUDED.scroll_offset,device_id=EXCLUDED.device_id,read_at=EXCLUDED.read_at,updated_at=EXCLUDED.updated_at,row_version=user_bible_history.row_version+1 RETURNING id,row_version`, id, h.userID(r), in.TranslationID, in.BookID, in.Chapter, in.Verse, in.ScrollOffset, in.DeviceID, now, now).Scan(&saved, &version)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to save reading history.")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"id": saved, "row_version": version, "read_at": now})
}

func (h *Handler) getBibleProgress(w http.ResponseWriter, r *http.Request) {
	items, err := readBibleMaps(r.Context(), h.db, `SELECT translation_id,book_id,chapter,completed_at,updated_at,row_version FROM user_bible_progress WHERE user_id=? ORDER BY updated_at DESC LIMIT 1000`, h.userID(r))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to load reading progress.")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"progress": items})
}
func (h *Handler) completeBibleChapter(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TranslationID string `json:"translation_id"`
		BookID        string `json:"book_id"`
		Chapter       int    `json:"chapter"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil || in.Chapter < 1 {
		httpx.WriteError(w, http.StatusBadRequest, "Invalid chapter progress.")
		return
	}
	if err := h.readableBibleTranslation(r.Context(), in.TranslationID); err != nil {
		h.bibleError(w, err)
		return
	}
	if _, err := h.bible.GetChapter(r.Context(), in.TranslationID, in.BookID, in.Chapter); err != nil {
		h.bibleError(w, err)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := h.db.ExecContext(r.Context(), `INSERT INTO user_bible_progress(user_id,translation_id,book_id,chapter,completed_at,updated_at,row_version) VALUES(?,?,?,?,?,?,1) ON CONFLICT(user_id,translation_id,book_id,chapter) DO UPDATE SET completed_at=EXCLUDED.completed_at,updated_at=EXCLUDED.updated_at,row_version=user_bible_progress.row_version+1`, h.userID(r), in.TranslationID, in.BookID, in.Chapter, now, now)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to save chapter progress.")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"translation_id": in.TranslationID, "book_id": in.BookID, "chapter": in.Chapter, "completed_at": now})
}

type bibleSyncMutation struct {
	ID         string         `json:"mutation_id"`
	Entity     string         `json:"entity"`
	Operation  string         `json:"operation"`
	Payload    map[string]any `json:"payload"`
	RowVersion int            `json:"row_version"`
}

func syncString(m map[string]any, key string) string {
	value, _ := m[key].(string)
	return strings.TrimSpace(value)
}
func syncInt(m map[string]any, key string) int {
	switch v := m[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	}
	return 0
}

func (h *Handler) applyBibleSyncMutation(r *http.Request, mutation bibleSyncMutation) (string, string) {
	if !bibleDeviceIDPattern.MatchString(mutation.ID) || len(mutation.Payload) > 20 || len(mutation.Payload) == 0 {
		return "", "invalid_mutation"
	}
	if mutation.Operation != "upsert" && mutation.Operation != "delete" {
		return "", "invalid_operation"
	}
	var prior string
	err := h.db.QueryRowContext(r.Context(), `SELECT COALESCE(entity_id,'') FROM user_bible_sync_mutations WHERE user_id=? AND mutation_id=?`, h.userID(r), mutation.ID).Scan(&prior)
	if err == nil {
		return prior, ""
	}
	if err != sql.ErrNoRows {
		return "", "storage_error"
	}
	userID := h.userID(r)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	entityID := ""
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		return "", "storage_error"
	}
	defer tx.Rollback()
	switch mutation.Entity {
	case "bookmark", "highlight":
		id := syncString(mutation.Payload, "id")
		if mutation.Operation == "delete" {
			if id == "" {
				return "", "invalid_reference"
			}
			table := "verse_bookmarks"
			if mutation.Entity == "highlight" {
				table = "verse_highlights"
			}
			_, err = tx.ExecContext(r.Context(), `UPDATE `+table+` SET deleted_at=?,updated_at=?,row_version=row_version+1 WHERE id=? AND user_id=? AND deleted_at IS NULL`, now, now, id, userID)
			if err != nil {
				return "", "storage_error"
			}
			entityID = id
		} else {
			in := bibleMarkInput{TranslationID: syncString(mutation.Payload, "translation_id"), BookID: syncString(mutation.Payload, "book_id"), Chapter: syncInt(mutation.Payload, "chapter"), Verse: syncInt(mutation.Payload, "verse"), Color: syncString(mutation.Payload, "color"), Label: syncString(mutation.Payload, "label"), Note: syncString(mutation.Payload, "note")}
			verse, valid := h.validateBibleMark(r, in)
			if !valid {
				return "", "invalid_reference"
			}
			if mutation.Entity == "bookmark" {
				if len(in.Label) > 120 || len(in.Note) > 4000 {
					return "", "invalid_payload"
				}
				entityID = uuid.NewString()
				err = tx.QueryRowContext(r.Context(), `INSERT INTO verse_bookmarks(id,user_id,version_id,book_id,chapter,verse,label,note,created_at,updated_at,row_version) VALUES(?,?,?,?,?,?,?,?,?,?,1) ON CONFLICT(user_id,version_id,book_id,chapter,verse) WHERE deleted_at IS NULL DO UPDATE SET label=EXCLUDED.label,note=EXCLUDED.note,updated_at=EXCLUDED.updated_at,row_version=verse_bookmarks.row_version+1 RETURNING id`, entityID, userID, in.TranslationID, verse.BookID, in.Chapter, in.Verse, in.Label, in.Note, now, now).Scan(&entityID)
			} else {
				if !oneOf(in.Color, "yellow", "green", "blue", "pink", "purple") {
					return "", "invalid_payload"
				}
				entityID = uuid.NewString()
				err = tx.QueryRowContext(r.Context(), `INSERT INTO verse_highlights(id,user_id,version_id,book_id,chapter,verse,color,created_at,updated_at,row_version) VALUES(?,?,?,?,?,?,?,?,?,1) ON CONFLICT(user_id,version_id,book_id,chapter,verse) WHERE deleted_at IS NULL DO UPDATE SET color=EXCLUDED.color,updated_at=EXCLUDED.updated_at,row_version=verse_highlights.row_version+1 RETURNING id`, entityID, userID, in.TranslationID, verse.BookID, in.Chapter, in.Verse, in.Color, now, now).Scan(&entityID)
			}
			if err != nil {
				return "", "storage_error"
			}
		}
	case "note":
		id := syncString(mutation.Payload, "id")
		if id == "" {
			id = uuid.NewString()
		}
		entityID = id
		if mutation.Operation == "delete" {
			_, err = tx.ExecContext(r.Context(), `UPDATE user_bible_notes SET deleted_at=?,updated_at=?,row_version=row_version+1 WHERE id=? AND user_id=? AND deleted_at IS NULL`, now, now, id, userID)
			if err != nil {
				return "", "storage_error"
			}
		} else {
			book := syncString(mutation.Payload, "book_id")
			chapter := syncInt(mutation.Payload, "chapter")
			start := syncInt(mutation.Payload, "verse_start")
			end := syncInt(mutation.Payload, "verse_end")
			body := strings.TrimSpace(syncString(mutation.Payload, "body"))
			title := strings.TrimSpace(syncString(mutation.Payload, "title"))
			translation := syncString(mutation.Payload, "translation_id")
			if !validateNoteReference(book, chapter, start, end) || body == "" || len(body) > 20000 || len(title) > 200 {
				return "", "invalid_payload"
			}
			if translation != "" {
				if err := h.readableBibleTranslation(r.Context(), translation); err != nil {
					return "", "invalid_translation"
				}
			}
			var existingVersion int
			existingErr := tx.QueryRowContext(r.Context(), `SELECT row_version FROM user_bible_notes WHERE id=? AND user_id=? AND deleted_at IS NULL`, id, userID).Scan(&existingVersion)
			if existingErr == sql.ErrNoRows {
				newID := uuid.NewString()
				err = tx.QueryRowContext(r.Context(), `INSERT INTO user_bible_notes(id,user_id,book_id,chapter,verse_start,verse_end,translation_id,title,body,device_id,created_at,updated_at,row_version) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,1) RETURNING id`, newID, userID, book, chapter, start, end, nullIfBlank(translation), title, body, syncString(mutation.Payload, "device_id"), now, now).Scan(&entityID)
			} else if existingErr != nil {
				return "", "storage_error"
			} else if mutation.RowVersion > 0 && mutation.RowVersion != existingVersion {
				return "", "version_conflict"
			} else {
				err = tx.QueryRowContext(r.Context(), `UPDATE user_bible_notes SET book_id=?,chapter=?,verse_start=?,verse_end=?,translation_id=?,title=?,body=?,updated_at=?,row_version=row_version+1 WHERE id=? AND user_id=? AND deleted_at IS NULL RETURNING id`, book, chapter, start, end, nullIfBlank(translation), title, body, now, id, userID).Scan(&entityID)
			}
			if err != nil {
				return "", "storage_error"
			}
		}
	case "history":
		if mutation.Operation != "upsert" {
			return "", "invalid_operation"
		}
		in := mutation.Payload
		translation := syncString(in, "translation_id")
		book := syncString(in, "book_id")
		chapter := syncInt(in, "chapter")
		verse := syncInt(in, "verse")
		if err := h.readableBibleTranslation(r.Context(), translation); err != nil {
			return "", "invalid_translation"
		}
		if _, err := h.bible.GetVerse(r.Context(), translation, book, chapter, verse); err != nil {
			return "", "invalid_reference"
		}
		entityID = uuid.NewString()
		err = tx.QueryRowContext(r.Context(), `INSERT INTO user_bible_history(id,user_id,translation_id,book_id,chapter,verse,scroll_offset,device_id,read_at,updated_at,row_version) VALUES(?,?,?,?,?,?,?,?,?,?,1) ON CONFLICT(user_id,translation_id,book_id,chapter) WHERE translation_id IS NOT NULL DO UPDATE SET verse=EXCLUDED.verse,scroll_offset=EXCLUDED.scroll_offset,device_id=EXCLUDED.device_id,read_at=EXCLUDED.read_at,updated_at=EXCLUDED.updated_at,row_version=user_bible_history.row_version+1 RETURNING id`, entityID, userID, translation, book, chapter, verse, 0, syncString(in, "device_id"), now, now).Scan(&entityID)
		if err != nil {
			return "", "storage_error"
		}
	case "progress":
		if mutation.Operation != "upsert" {
			return "", "invalid_operation"
		}
		in := mutation.Payload
		translation := syncString(in, "translation_id")
		book := syncString(in, "book_id")
		chapter := syncInt(in, "chapter")
		if err := h.readableBibleTranslation(r.Context(), translation); err != nil {
			return "", "invalid_translation"
		}
		if _, err := h.bible.GetChapter(r.Context(), translation, book, chapter); err != nil {
			return "", "invalid_reference"
		}
		_, err = tx.ExecContext(r.Context(), `INSERT INTO user_bible_progress(user_id,translation_id,book_id,chapter,completed_at,updated_at,row_version) VALUES(?,?,?,?,?,?,1) ON CONFLICT(user_id,translation_id,book_id,chapter) DO UPDATE SET completed_at=EXCLUDED.completed_at,updated_at=EXCLUDED.updated_at,row_version=user_bible_progress.row_version+1`, userID, translation, book, chapter, now, now)
		if err != nil {
			return "", "storage_error"
		}
		entityID = translation + ":" + book + ":" + strconv.Itoa(chapter)
	default:
		return "", "invalid_entity"
	}
	_, err = tx.ExecContext(r.Context(), `INSERT INTO user_bible_sync_mutations(user_id,mutation_id,entity,entity_id,applied_at) VALUES(?,?,?,?,?)`, userID, mutation.ID, mutation.Entity, entityID, now)
	if err != nil {
		return "", "storage_error"
	}
	if err = tx.Commit(); err != nil {
		return "", "storage_error"
	}
	return entityID, ""
}

func (h *Handler) syncBibleData(w http.ResponseWriter, r *http.Request) {
	var in struct {
		DeviceID  string              `json:"device_id"`
		Since     string              `json:"since"`
		Mutations []bibleSyncMutation `json:"mutations"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil || !bibleDeviceIDPattern.MatchString(in.DeviceID) || len(in.Mutations) > 100 {
		httpx.WriteError(w, http.StatusBadRequest, "Provide a valid device ID and no more than 100 sync mutations.")
		return
	}
	since := time.Unix(0, 0).UTC()
	if in.Since != "" {
		parsed, err := time.Parse(time.RFC3339Nano, in.Since)
		if err != nil || parsed.After(time.Now().Add(time.Minute)) {
			httpx.WriteError(w, http.StatusBadRequest, "Sync cursor must be a valid prior timestamp.")
			return
		}
		since = parsed
	}
	applied := make([]map[string]string, 0, len(in.Mutations))
	conflicts := make([]map[string]string, 0)
	for _, mutation := range in.Mutations {
		entityID, code := h.applyBibleSyncMutation(r, mutation)
		if code != "" {
			conflicts = append(conflicts, map[string]string{"mutation_id": mutation.ID, "error_code": code})
			continue
		}
		applied = append(applied, map[string]string{"mutation_id": mutation.ID, "entity_id": entityID})
	}
	now := time.Now().UTC()
	user := h.userID(r)
	queries := map[string]string{
		"bookmarks":  `SELECT id,version_id AS translation_id,book_id,chapter,verse,label,note,created_at,updated_at,deleted_at,row_version FROM verse_bookmarks WHERE user_id=? AND updated_at>? ORDER BY updated_at`,
		"highlights": `SELECT id,version_id AS translation_id,book_id,chapter,verse,color,created_at,updated_at,deleted_at,row_version FROM verse_highlights WHERE user_id=? AND updated_at>? ORDER BY updated_at`,
		"notes":      `SELECT id,book_id,chapter,verse_start,verse_end,translation_id,title,body,created_at,updated_at,deleted_at,row_version FROM user_bible_notes WHERE user_id=? AND updated_at>? ORDER BY updated_at`,
		"history":    `SELECT id,translation_id,book_id,chapter,verse,scroll_offset,device_id,read_at,updated_at,row_version FROM user_bible_history WHERE user_id=? AND updated_at>? ORDER BY updated_at`,
		"progress":   `SELECT translation_id,book_id,chapter,completed_at,updated_at,row_version FROM user_bible_progress WHERE user_id=? AND updated_at>? ORDER BY updated_at`,
	}
	data := map[string]any{}
	for key, q := range queries {
		rows, err := readBibleMaps(r.Context(), h.db, q, user, since.Format(time.RFC3339Nano))
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "Unable to synchronize Bible study data.")
			return
		}
		data[key] = rows
	}
	_, err := h.db.ExecContext(r.Context(), `INSERT INTO user_bible_sync_devices(user_id,device_id,last_sync_at,updated_at) VALUES(?,?,?,?) ON CONFLICT(user_id,device_id) DO UPDATE SET last_sync_at=EXCLUDED.last_sync_at,updated_at=EXCLUDED.updated_at`, user, in.DeviceID, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to save sync position.")
		return
	}
	prefs, err := readBibleMaps(r.Context(), h.db, `SELECT default_translation_id,preferred_language,font_size,line_height,theme,show_verse_numbers,updated_at,row_version FROM user_bible_preferences WHERE user_id=? AND updated_at>?`, user, since.Format(time.RFC3339Nano))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to synchronize Bible preferences.")
		return
	}
	data["preferences"] = prefs
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"device_id": in.DeviceID, "cursor": now.Format(time.RFC3339Nano), "applied": applied, "conflicts": conflicts, "changes": data})
}
func (h *Handler) requestOfflineBiblePackage(w http.ResponseWriter, r *http.Request) {
	if h.signer == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "Offline storage is unavailable.")
		return
	}
	var in struct {
		TranslationID string `json:"translation_id"`
		BookID        string `json:"book_id"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil || in.TranslationID == "" || in.BookID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "Choose a translation and book.")
		return
	}
	t, err := h.bible.GetTranslation(r.Context(), in.TranslationID)
	if err != nil {
		h.bibleError(w, err)
		return
	}
	var maxDays int
	err = h.db.QueryRowContext(r.Context(), `SELECT offline_max_days FROM bible_versions WHERE id=? AND status='active' AND api_exposure_allowed=TRUE AND offline_allowed=TRUE AND redistribution_allowed=TRUE AND commercial_use=TRUE`, t.ID).Scan(&maxDays)
	if err != nil || !t.OfflineAllowed || !t.RedistributionAllowed || !t.CommercialUse || !t.APIExposureAllowed || maxDays < 1 || maxDays > 3650 {
		httpx.WriteError(w, http.StatusForbidden, "Offline reading is not licensed for this translation.")
		return
	}
	book, err := h.bible.GetBook(r.Context(), t.ID, in.BookID)
	if err != nil {
		h.bibleError(w, err)
		return
	}
	if book.ChapterCount < 1 || book.ChapterCount > 200 {
		httpx.WriteError(w, http.StatusServiceUnavailable, "This book does not have a valid complete chapter map.")
		return
	}
	if t.ContentHash == "" {
		httpx.WriteError(w, http.StatusConflict, "The translation has no pinned source checksum.")
		return
	}
	var packageID, storageKey, hash string
	var size int64
	// Package bytes exclude request-time fields, so all entitled readers share
	// one immutable, source-hash-keyed object rather than rebuilding the book
	// and uploading a different object on every request.
	err = h.db.QueryRowContext(r.Context(), `SELECT id,storage_key,file_size_bytes,content_hash FROM bible_offline_packages
		WHERE translation_id=? AND book_id=? AND source_content_hash=? AND package_schema_version=1
		AND status='ready' AND deleted_at IS NULL ORDER BY created_at DESC LIMIT 1`,
		t.ID, book.ID, t.ContentHash).Scan(&packageID, &storageKey, &size, &hash)
	if err == nil {
		// A database row is not evidence that the R2 object survived. Never
		// hand out a new license for a missing package.
		exists, storageErr := h.signer.Exists(r.Context(), storageKey)
		if storageErr != nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "Unable to check offline package storage.")
			return
		}
		if !exists {
			_, _ = h.db.ExecContext(r.Context(), `UPDATE bible_offline_packages SET status='failed',row_version=row_version+1 WHERE id=? AND status='ready'`, packageID)
			err = sql.ErrNoRows
		}
	}
	if err == sql.ErrNoRows {
		chapters := make([]bible.Chapter, 0, book.ChapterCount)
		for n := 1; n <= book.ChapterCount; n++ {
			ch, chapterErr := h.bible.GetChapter(r.Context(), t.ID, book.ID, n)
			if chapterErr != nil || ch.Translation.ContentHash != t.ContentHash {
				httpx.WriteError(w, http.StatusServiceUnavailable, "This book is not fully available from the reviewed source.")
				return
			}
			chapters = append(chapters, ch)
		}
		payload, marshalErr := json.Marshal(map[string]any{
			"translation": t, "book": book, "chapters": chapters,
			"canonical_reference_format": "USFM.chapter.verse",
			"source_content_hash":        t.ContentHash, "package_schema_version": 1,
		})
		if marshalErr != nil || len(payload) > 32<<20 {
			httpx.WriteError(w, http.StatusRequestEntityTooLarge, "This offline package exceeds the supported size.")
			return
		}
		hash = sha256Hex(payload)
		key := fmt.Sprintf("bible/offline/%s/%s/%s.json", t.ID, hash, book.ID)
		if uploadErr := h.signer.Upload(r.Context(), key, payload, map[string]string{
			"sha256": hash, "content-type": "application/json", "translation": t.ID,
		}); uploadErr != nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "Unable to store an offline package.")
			return
		}
		_, err = h.db.ExecContext(r.Context(), `INSERT INTO bible_offline_packages
			(id,translation_id,book_id,content_hash,source_content_hash,package_schema_version,storage_key,file_size_bytes,status)
			VALUES(?,?,?,?,?,1,?,?,'ready')
			ON CONFLICT(translation_id,book_id,content_hash) DO UPDATE SET status='ready',
			  source_content_hash=EXCLUDED.source_content_hash,package_schema_version=1,
			  file_size_bytes=EXCLUDED.file_size_bytes,row_version=bible_offline_packages.row_version+1`,
			uuid.NewString(), t.ID, book.ID, hash, t.ContentHash, key, len(payload))
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "Unable to register an offline package.")
			return
		}
		err = h.db.QueryRowContext(r.Context(), `SELECT id,storage_key,file_size_bytes,content_hash FROM bible_offline_packages
			WHERE translation_id=? AND book_id=? AND content_hash=? AND status='ready' AND deleted_at IS NULL`,
			t.ID, book.ID, hash).Scan(&packageID, &storageKey, &size, &hash)
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to resolve the offline package.")
		return
	}
	issued := time.Now().UTC()
	expires := issued.Add(time.Duration(maxDays) * 24 * time.Hour)
	licenseID := uuid.NewString()
	_, err = h.db.ExecContext(r.Context(), `INSERT INTO user_bible_offline_licenses(id,user_id,package_id,issued_at,expires_at) VALUES(?,?,?,?,?) ON CONFLICT(user_id,package_id) DO UPDATE SET issued_at=EXCLUDED.issued_at,expires_at=EXCLUDED.expires_at,revoked_at=NULL`, licenseID, h.userID(r), packageID, issued.Format(time.RFC3339Nano), expires.Format(time.RFC3339Nano))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to issue offline license.")
		return
	}
	if err = h.db.QueryRowContext(r.Context(), `SELECT id FROM user_bible_offline_licenses WHERE user_id=? AND package_id=?`, h.userID(r), packageID).Scan(&licenseID); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to read offline license.")
		return
	}
	urlExpires := time.Now().UTC().Add(time.Hour)
	signed, err := h.signer.GenerateSignedURL(r.Context(), storageKey, time.Hour)
	if err != nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "Unable to sign offline package URL.")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"package_id": packageID, "license_id": licenseID, "translation_id": t.ID, "book_id": book.ID, "content_hash": hash, "size_bytes": size, "download_url": signed, "url_expires_at": urlExpires, "offline_expires_at": expires, "attribution_required": t.AttributionRequired, "attribution_text": t.AttributionText})
}
func (h *Handler) listBibleOfflineLicenses(w http.ResponseWriter, r *http.Request) {
	items, err := readBibleMaps(r.Context(), h.db, `SELECT l.id,l.package_id,p.translation_id,p.book_id,p.content_hash,p.file_size_bytes,p.status AS package_status,l.issued_at,l.expires_at,l.revoked_at FROM user_bible_offline_licenses l JOIN bible_offline_packages p ON p.id=l.package_id WHERE l.user_id=? ORDER BY l.issued_at DESC`, h.userID(r))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to load offline licenses.")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"licenses": items})
}
func (h *Handler) revokeBibleOfflineLicense(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := h.db.ExecContext(r.Context(), `UPDATE user_bible_offline_licenses SET revoked_at=? WHERE id=? AND user_id=? AND revoked_at IS NULL`, now, r.PathValue("id"), h.userID(r))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to revoke offline license.")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		httpx.WriteError(w, http.StatusNotFound, "Offline license not found.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) bibleAudio(w http.ResponseWriter, r *http.Request) {
	translationID := strings.TrimSpace(r.URL.Query().Get("translation"))
	reference := strings.TrimSpace(r.URL.Query().Get("reference"))
	ref, err := bible.ParseReference(reference)
	if err != nil || translationID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "Choose an available translation and reference.")
		return
	}
	t, err := h.bible.GetTranslation(r.Context(), translationID)
	if err != nil {
		h.bibleError(w, err)
		return
	}
	if !t.AudioAllowed || !t.RedistributionAllowed || !t.CommercialUse || !t.APIExposureAllowed {
		httpx.WriteError(w, http.StatusNotFound, "Audio is not licensed for this translation.")
		return
	}
	start, end := ref.StartVerse, ref.EndVerse
	if start == 0 {
		start = 1
		book, ok := bible.BookByID(ref.Book)
		if !ok || ref.Chapter > len(book.Verses) {
			httpx.WriteError(w, http.StatusBadRequest, "Invalid chapter reference.")
			return
		}
		end = book.Verses[ref.Chapter-1]
	}
	if end == 0 {
		end = start
	}
	var id, voiceID, key, hash string
	var duration int64
	var raw []byte
	err = h.db.QueryRowContext(r.Context(), `SELECT a.id,a.voice_id,a.storage_key,a.checksum_sha256,a.duration_ms,a.alignment FROM bible_audio_assets a JOIN bible_versions v ON v.id=a.translation_id WHERE a.translation_id=? AND a.book_id=? AND a.chapter=? AND a.verse_start=? AND a.verse_end=? AND a.content_hash=v.content_hash AND a.status='published' AND v.status='active' AND v.audio_allowed=TRUE AND v.redistribution_allowed=TRUE AND v.commercial_use=TRUE AND v.api_exposure_allowed=TRUE`, t.ID, ref.Book, ref.Chapter, start, end).Scan(&id, &voiceID, &key, &hash, &duration, &raw)
	if err == sql.ErrNoRows {
		httpx.WriteError(w, http.StatusNotFound, "No current, approved audio is available for this passage.")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Unable to load Bible audio.")
		return
	}
	lic, _, _ := h.rightsFor(r.Context(), voiceID)
	decision := rights.Evaluate(lic, rights.Request{Use: rights.UsePlayback, At: time.Now().UTC(), Language: t.Locale})
	if !decision.Allowed {
		httpx.WriteError(w, http.StatusForbidden, "Voice playback rights do not permit this audio.")
		return
	}
	if h.signer == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "Audio delivery is unavailable.")
		return
	}
	// A playback grant must not outlive the voice licence that authorized it.
	// Never generate audio on play; sign only a pre-reviewed, published asset.
	ttl := time.Hour
	if lic.Expiry != nil {
		if remaining := time.Until(*lic.Expiry); remaining < ttl {
			ttl = remaining
		}
	}
	if ttl <= time.Second {
		httpx.WriteError(w, http.StatusForbidden, "Voice playback rights have expired.")
		return
	}
	signed, err := h.signer.GenerateSignedURL(r.Context(), key, ttl)
	if err != nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "Audio delivery is temporarily unavailable.")
		return
	}
	var alignment any = []any{}
	_ = json.Unmarshal(raw, &alignment)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "reference": reference, "translation": t, "voice_id": voiceID, "duration_ms": duration, "checksum_sha256": hash, "alignment": alignment, "audio_url": signed, "expires_in_seconds": int(ttl.Seconds())})
}
