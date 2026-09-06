package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestAdminAudioDurationIsNotAcceptedOnTrust covers the PHASE 13 fix.
//
// POST /admin/audio records audio that already exists elsewhere, and it used to
// write duration_seconds straight from the request body. That number is what
// the session planner schedules every queue item against, so a typo or a
// compromised admin token could put an absurd value into every session built
// from that asset. When the bytes are reachable through our own storage the
// duration is measured instead; when they are not, the figure is range-checked.
func TestAdminAudioDurationIsNotAcceptedOnTrust(t *testing.T) {
	dbConn := setupTestDB(t)
	defer dbConn.Close()

	h := NewHandler(Config{JWTSecret: "test-secret", TokenTTL: "24h"}, dbConn)
	h.BuildEngine()
	router := h.Routes()
	adminToken := createAdminUser(t, dbConn, h, router)

	post := func(duration int) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]interface{}{
			"confession_id":    "11111111-1111-1111-1111-111111111111",
			"voice_id":         "22222222-2222-2222-2222-222222222222",
			"url":              "https://cdn.example.com/a.mp3",
			"duration_seconds": duration,
			"status":           "ready",
		})
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/admin/audio", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		router.ServeHTTP(rr, req)
		return rr
	}

	for _, dur := range []int{999999, 0, -30} {
		rr := post(dur)
		if rr.Code != http.StatusUnprocessableEntity {
			t.Errorf("duration_seconds=%d → %d, want 422: %s", dur, rr.Code, rr.Body.String())
			continue
		}
		if !strings.Contains(rr.Body.String(), "duration_seconds") {
			t.Errorf("422 body should name the offending field, got: %s", rr.Body.String())
		}
	}

	// The guard must not refuse everything. A plausible figure has to get past
	// the duration check — which it proves by failing later, on the voice
	// lookup, with 400 rather than 422.
	if rr := post(30); rr.Code != http.StatusBadRequest {
		t.Errorf("duration_seconds=30 → %d, want 400 (past the duration check, failing on the missing voice): %s",
			rr.Code, rr.Body.String())
	}
}
