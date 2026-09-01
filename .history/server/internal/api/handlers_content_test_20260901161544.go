package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Teamthy/i-confess/internal/models"
)

// TestContentWorkflow validates the complete content creation and consumption flow.
func TestContentWorkflow(t *testing.T) {
	dbConn := setupTestDB(t)
	defer dbConn.Close()

	h := NewHandler(Config{
		JWTSecret: "test-secret",
		TokenTTL:  "24h",
	}, dbConn)
	h.BuildEngine()
	router := h.Routes()

	// 1. Register a user
	registerBody := map[string]string{
		"email":        "user@test.com",
		"password":     "password123",
		"display_name": "Test User",
		"timezone":     "UTC",
	}
	registerReq, _ := json.Marshal(registerBody)
	registerResp := httptest.NewRecorder()
	router.ServeHTTP(registerResp, httptest.NewRequest("POST", "/auth/register", bytes.NewReader(registerReq)))
	if registerResp.Code != http.StatusOK {
		t.Fatalf("register failed: status %d, body %s", registerResp.Code, registerResp.Body.String())
	}

	var registerResult map[string]interface{}
	json.Unmarshal(registerResp.Body.Bytes(), &registerResult)
	token := registerResult["token"].(string)

	// 2. List public categories (should be empty initially)
	catResp := httptest.NewRecorder()
	router.ServeHTTP(catResp, httptest.NewRequest("GET", "/categories", nil))
	if catResp.Code != http.StatusOK {
		t.Fatalf("list categories failed: status %d", catResp.Code)
	}
	var cats []models.Category
	json.Unmarshal(catResp.Body.Bytes(), &cats)
	if len(cats) != 0 {
		t.Fatalf("expected 0 categories, got %d", len(cats))
	}

	// 3. Create an admin user and set their role
	adminToken := createAdminUser(t, dbConn, h, router)

	// 4. Create a category as admin
	createCatBody := map[string]interface{}{
		"name":        "Prayer Confessions",
		"slug":        "prayer-confessions",
		"description": "Confessions focused on prayer and spiritual disciplines",
		"icon":        "🙏",
		"premium":     false,
		"status":      "published",
		"sort_order":  1,
	}
	createCatReq, _ := json.Marshal(createCatBody)
	createCatResp := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/admin/categories", bytes.NewReader(createCatReq))
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", adminToken))
	router.ServeHTTP(createCatResp, req)
	if createCatResp.Code != http.StatusCreated {
		t.Fatalf("create category failed: status %d, body %s", createCatResp.Code, createCatResp.Body.String())
	}

	var cat models.Category
	json.Unmarshal(createCatResp.Body.Bytes(), &cat)
	if cat.ID == "" {
		t.Fatal("created category has no ID")
	}

	// 5. Create confessions with variants
	createConfReq := map[string]interface{}{
		"category_id": cat.ID,
		"title":       "Confession of Pride",
		"short_text":  "I have struggled with pride",
		"medium_text": "I have often felt superior to others and have neglected the humble spirit that is required",
		"long_text":   "Lord, I confess that pride has taken root in my heart. I have looked down on others...",
		"language":    "en",
		"status":      "published",
		"author":      "Anonymous",
		"intensity":   2,
		"tags":        []string{"sin", "pride", "repentance"},
		"variants": []map[string]interface{}{
			{
				"label":             "30s",
				"duration_seconds":  30,
				"sort_order":        1,
			},
			{
				"label":             "1m",
				"duration_seconds":  60,
				"sort_order":        2,
			},
			{
				"label":             "3m",
				"duration_seconds":  180,
				"sort_order":        3,
			},
		},
	}
	createConfReqBody, _ := json.Marshal(createConfReq)
	createConfResp := httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/admin/confessions", bytes.NewReader(createConfReqBody))
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", adminToken))
	router.ServeHTTP(createConfResp, req)
	if createConfResp.Code != http.StatusCreated {
		t.Fatalf("create confession failed: status %d, body %s", createConfResp.Code, createConfResp.Body.String())
	}

	var conf models.Confession
	json.Unmarshal(createConfResp.Body.Bytes(), &conf)
	if conf.ID == "" {
		t.Fatal("created confession has no ID")
	}
	if len(conf.Variants) != 3 {
		t.Fatalf("expected 3 variants, got %d", len(conf.Variants))
	}

	// 6. Create a voice
	createVoiceReq := map[string]interface{}{
		"name":        "Sarah - Female Voice",
		"description": "Professional female voice for confessions",
		"type":        "professional",
		"provider":    "google-tts",
		"gender":      "female",
		"language":    "en",
		"premium":     false,
		"status":      "active",
	}
	createVoiceReqBody, _ := json.Marshal(createVoiceReq)
	createVoiceResp := httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/admin/voices", bytes.NewReader(createVoiceReqBody))
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", adminToken))
	router.ServeHTTP(createVoiceResp, req)
	if createVoiceResp.Code != http.StatusCreated {
		t.Fatalf("create voice failed: status %d, body %s", createVoiceResp.Code, createVoiceResp.Body.String())
	}

	var voice models.Voice
	json.Unmarshal(createVoiceResp.Body.Bytes(), &voice)
	if voice.ID == "" {
		t.Fatal("created voice has no ID")
	}

	// 7. Create an audio asset for a confession variant
	createAudioReq := map[string]interface{}{
		"confession_id":    conf.ID,
		"variant_id":       conf.Variants[0].ID, // 30s variant
		"voice_id":         voice.ID,
		"url":              "https://cdn.example.com/audio/confession-1-30s.mp3",
		"duration_seconds": 30,
		"size_bytes":       524288,
		"status":           "ready",
	}
	createAudioReqBody, _ := json.Marshal(createAudioReq)
	createAudioResp := httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/admin/audio", bytes.NewReader(createAudioReqBody))
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", adminToken))
	router.ServeHTTP(createAudioResp, req)
	if createAudioResp.Code != http.StatusCreated {
		t.Fatalf("create audio failed: status %d, body %s", createAudioResp.Code, createAudioResp.Body.String())
	}

	var audio models.AudioAsset
	json.Unmarshal(createAudioResp.Body.Bytes(), &audio)
	if audio.ID == "" {
		t.Fatal("created audio asset has no ID")
	}

	// 8. Verify the confession is now visible in public API
	getConfResp := httptest.NewRecorder()
	router.ServeHTTP(getConfResp, httptest.NewRequest("GET", fmt.Sprintf("/confessions/%s", conf.ID), nil))
	if getConfResp.Code != http.StatusOK {
		t.Fatalf("get confession failed: status %d", getConfResp.Code)
	}

	var fetchedConf models.Confession
	json.Unmarshal(getConfResp.Body.Bytes(), &fetchedConf)
	if fetchedConf.ID != conf.ID {
		t.Fatal("fetched confession doesn't match created confession")
	}
	if len(fetchedConf.Variants) == 0 {
		t.Fatal("fetched confession has no variants")
	}

	// 9. Create a session for the user (authenticated)
	createSessionReq := map[string]interface{}{
		"category_ids":     []string{cat.ID},
		"duration_seconds": 180, // 3 minutes
		"voice_id":         voice.ID,
	}
	createSessionReqBody, _ := json.Marshal(createSessionReq)
	createSessionResp := httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/sessions", bytes.NewReader(createSessionReqBody))
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	router.ServeHTTP(createSessionResp, req)
	if createSessionResp.Code != http.StatusCreated {
		t.Fatalf("create session failed: status %d, body %s", createSessionResp.Code, createSessionResp.Body.String())
	}

	var sess models.Session
	json.Unmarshal(createSessionResp.Body.Bytes(), &sess)
	if sess.ID == "" {
		t.Fatal("created session has no ID")
	}
	if sess.UserID == "" {
		t.Fatal("session has no user ID")
	}
	if len(sess.Items) == 0 {
		t.Fatal("session has no items")
	}

	// 10. Get the created session
	getSessionResp := httptest.NewRecorder()
	req = httptest.NewRequest("GET", fmt.Sprintf("/sessions/%s", sess.ID), nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	router.ServeHTTP(getSessionResp, req)
	if getSessionResp.Code != http.StatusOK {
		t.Fatalf("get session failed: status %d", getSessionResp.Code)
	}

	// 11. Record playback
	recordPlaybackReq := map[string]interface{}{
		"session_id":       sess.ID,
		"confession_id":    conf.ID,
		"duration_seconds": 30,
		"completed":        true,
		"skipped":          false,
	}
	recordPlaybackReqBody, _ := json.Marshal(recordPlaybackReq)
	recordPlaybackResp := httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/me/history", bytes.NewReader(recordPlaybackReqBody))
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	router.ServeHTTP(recordPlaybackResp, req)
	if recordPlaybackResp.Code != http.StatusCreated {
		t.Fatalf("record playback failed: status %d, body %s", recordPlaybackResp.Code, recordPlaybackResp.Body.String())
	}

	// 12. Get user's history
	historyResp := httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/me/history", nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	router.ServeHTTP(historyResp, req)
	if historyResp.Code != http.StatusOK {
		t.Fatalf("get history failed: status %d", historyResp.Code)
	}

	var history []models.PlaybackRecord
	json.Unmarshal(historyResp.Body.Bytes(), &history)
	if len(history) == 0 {
		t.Fatal("history is empty")
	}

	t.Log("✅ Content workflow test passed")
}

// createAdminUser creates a user and sets their role to admin
func createAdminUser(t *testing.T, dbConn interface{}, h *Handler, router http.Handler) string {
	adminRegisterBody := map[string]string{
		"email":        "admin@test.com",
		"password":     "adminpass123",
		"display_name": "Admin User",
		"timezone":     "UTC",
	}
	adminRegisterReq, _ := json.Marshal(adminRegisterBody)
	adminRegisterResp := httptest.NewRecorder()
	router.ServeHTTP(adminRegisterResp, httptest.NewRequest("POST", "/auth/register", bytes.NewReader(adminRegisterReq)))
	if adminRegisterResp.Code != http.StatusOK {
		t.Fatalf("admin register failed: status %d", adminRegisterResp.Code)
	}

	var adminRegisterResult map[string]interface{}
	json.Unmarshal(adminRegisterResp.Body.Bytes(), &adminRegisterResult)
	adminToken := adminRegisterResult["token"].(string)

	// Extract user ID and set admin role
	adminUserData := adminRegisterResult["user"].(map[string]interface{})
	adminUserID := adminUserData["id"].(string)

	setRoleBody := map[string]string{
		"user_id": adminUserID,
		"role":    "super_admin",
	}
	setRoleReq, _ := json.Marshal(setRoleBody)
	setRoleResp := httptest.NewRecorder()

	// Use a temporary token to make the request - we need an admin token
	// For testing, we'll use a super-admin context directly via the store
	// But for now, let's create the admin role in the store directly
	adminUser, _ := h.users.ByID(context.Background(), adminUserID)
	if adminUser != nil {
		h.users.SetAdminRole(context.Background(), adminUserID, "super_admin")
	}

	// Re-login as admin to get a new token with admin role
	adminLoginBody := map[string]string{
		"email":    "admin@test.com",
		"password": "adminpass123",
	}
	adminLoginReq, _ := json.Marshal(adminLoginBody)
	adminLoginResp := httptest.NewRecorder()
	router.ServeHTTP(adminLoginResp, httptest.NewRequest("POST", "/auth/login", bytes.NewReader(adminLoginReq)))
	if adminLoginResp.Code != http.StatusOK {
		t.Fatalf("admin login failed: status %d, body %s", adminLoginResp.Code, adminLoginResp.Body.String())
	}

	var adminLoginResult map[string]interface{}
	json.Unmarshal(adminLoginResp.Body.Bytes(), &adminLoginResult)
	adminToken = adminLoginResult["token"].(string)

	_ = setRoleReq
	_ = setRoleResp

	return adminToken
}

// TestAdminCategoryManagement tests admin category CRUD operations
func TestAdminCategoryManagement(t *testing.T) {
	dbConn := setupTestDB(t)
	defer dbConn.Close()

	h := NewHandler(Config{
		JWTSecret: "test-secret",
		TokenTTL:  "24h",
	}, dbConn)
	h.BuildEngine()
	router := h.Routes()

	// Create admin user
	adminToken := createAdminUser(t, dbConn, h, router)

	// Test: Create category
	createReq := map[string]interface{}{
		"name":   "Forgiveness",
		"slug":   "forgiveness",
		"status": "published",
	}
	createReqBody, _ := json.Marshal(createReq)
	createResp := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/admin/categories", bytes.NewReader(createReqBody))
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", adminToken))
	router.ServeHTTP(createResp, req)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("create failed: status %d", createResp.Code)
	}

	// Test: List categories
	listResp := httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/admin/categories", nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", adminToken))
	router.ServeHTTP(listResp, req)
	if listResp.Code != http.StatusOK {
		t.Fatalf("list failed: status %d", listResp.Code)
	}

	var cats []models.Category
	json.Unmarshal(listResp.Body.Bytes(), &cats)
	if len(cats) == 0 {
		t.Fatal("no categories returned")
	}

	t.Log("✅ Admin category management test passed")
}

// TestPublicContentAccess tests public access to published content
func TestPublicContentAccess(t *testing.T) {
	dbConn := setupTestDB(t)
	defer dbConn.Close()

	h := NewHandler(Config{
		JWTSecret: "test-secret",
		TokenTTL:  "24h",
	}, dbConn)
	h.BuildEngine()
	router := h.Routes()

	adminToken := createAdminUser(t, dbConn, h, router)

	// Create published category
	catReq := map[string]interface{}{
		"name":   "Faith",
		"slug":   "faith",
		"status": "published",
	}
	catReqBody, _ := json.Marshal(catReq)
	catResp := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/admin/categories", bytes.NewReader(catReqBody))
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", adminToken))
	router.ServeHTTP(catResp, req)

	var cat models.Category
	json.Unmarshal(catResp.Body.Bytes(), &cat)

	// Test: Public access to categories
	publicCatResp := httptest.NewRecorder()
	router.ServeHTTP(publicCatResp, httptest.NewRequest("GET", "/categories", nil))
	if publicCatResp.Code != http.StatusOK {
		t.Fatalf("public category list failed: status %d", publicCatResp.Code)
	}

	var cats []models.Category
	json.Unmarshal(publicCatResp.Body.Bytes(), &cats)
	if len(cats) == 0 {
		t.Fatal("no public categories")
	}

	t.Log("✅ Public content access test passed")
}
