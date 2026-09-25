package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Teamthy/i-confess/internal/auth"
	"github.com/Teamthy/i-confess/internal/cache"
	"github.com/Teamthy/i-confess/internal/content"
	"github.com/Teamthy/i-confess/internal/db"
	"github.com/Teamthy/i-confess/internal/db/dbtest"
)

// The three content caches are per-process copies of what is in one shared
// database. Before cache_invalidation.go existed, nothing ever dropped an
// entry: an admin could publish a confession and the very instance that
// handled the write kept serving the old list for ttl+swr, which is fifteen
// minutes on the category and voice caches. Across instances it was worse -
// every other replica stayed stale too, with no path to ever notice.
//
// These tests stand two Handlers up over one database, which is the shape of a
// two-replica deployment, and watch a write on one become visible on the other.

// instance is one API node: a Handler, its router, and the cache configuration
// that decides whether it hears about other instances' writes.
type instance struct {
	name string
	h    *Handler
	srv  *httptest.Server
}

func newCacheInstance(t *testing.T, name string, conn *db.DB, bus cache.Bus) *instance {
	t.Helper()
	h := NewHandler(Config{JWTSecret: "test-secret-value", TokenTTL: "24h"}, conn)
	h.BuildEngine()
	if bus != nil {
		if err := h.SetCacheBus(bus); err != nil {
			t.Fatalf("%s: install cache bus: %v", name, err)
		}
	}
	inst := &instance{name: name, h: h, srv: httptest.NewServer(h.Routes())}
	t.Cleanup(func() {
		inst.srv.Close()
		h.CloseCacheBus()
	})
	return inst
}

// adminTokenFor returns a signed-in super admin for the instance.
func adminTokenFor(t *testing.T, inst *instance, email string) string {
	t.Helper()
	// Register and sign in first: the role is read at token issue time, so the
	// admin token below has to be minted after the promotion.
	registerAndSignIn(t, inst.srv, email, "a-strong-enough-passphrase")
	userID := userIDForEmail(t, inst.h, email)
	if err := inst.h.users.SetAdminRole(context.Background(), userID, auth.RoleSuperAdmin); err != nil {
		t.Fatalf("promote %s: %v", email, err)
	}
	return signIn(t, inst.srv, email, "a-strong-enough-passphrase")
}

func fetchJSONBody(t *testing.T, inst *instance, path string) string {
	return fetchJSONBodyAs(t, inst, path, "")
}

// fetchJSONBodyAs fetches with a bearer token. /metrics is admin-only, so the
// observability assertions have to present one.
func fetchJSONBodyAs(t *testing.T, inst *instance, path, token string) string {
	t.Helper()
	status, body := doRequest(t, inst.srv, http.MethodGet, path, token, "")
	if status != http.StatusOK {
		t.Fatalf("%s: GET %s returned %d: %s", inst.name, path, status, truncateBody(body))
	}
	return body
}

// warmCategoryCaches reads the category list on every instance, so each one holds a
// cached copy before the write happens. Without this the test would pass on an
// empty cache and prove nothing.
func warmCategoryCaches(t *testing.T, instances ...*instance) []string {
	t.Helper()
	before := make([]string, len(instances))
	for i, inst := range instances {
		before[i] = fetchJSONBody(t, inst, "/categories")
	}
	return before
}

// TestAPublishedCategoryIsVisibleToOtherInstancesImmediately is the G-10 case:
// an admin write on one replica, read on another, with no waiting.
func TestAPublishedCategoryIsVisibleToOtherInstancesImmediately(t *testing.T) {
	dbConn := dbtest.New(t)
	defer dbConn.Close()

	bus := cache.NewMemoryBus()
	defer bus.Close()

	writer := newCacheInstance(t, "writer", dbConn, bus)
	reader := newCacheInstance(t, "reader", dbConn, bus)
	admin := adminTokenFor(t, writer, "cache-admin@example.com")

	before := warmCategoryCaches(t, reader, writer)
	if containsField(before[0], "cache-probe-category") {
		t.Fatal("slug already present before the write; the assertion below would pass vacuously")
	}

	status, body := doRequest(t, writer.srv, http.MethodPost, "/admin/categories", admin,
		`{"name":"Cache Probe","slug":"cache-probe-category","status":"published"}`)
	if status != http.StatusCreated {
		t.Fatalf("create category: %d %s", status, truncateBody(body))
	}

	after := fetchJSONBody(t, reader, "/categories")
	if !containsField(after, "cache-probe-category") {
		t.Fatalf("the other instance still serves its cached category list after an admin write:\n%s", truncateBody(after))
	}
	// And the writing instance is fresh too. This is the half that never
	// depended on the bus, and it was broken as well.
	if self := fetchJSONBody(t, writer, "/categories"); !containsField(self, "cache-probe-category") {
		t.Fatalf("the writing instance still serves its own cached list:\n%s", truncateBody(self))
	}
}

// TestAnInstanceWithoutTheBusKeepsItsCachedCopy is the control.
//
// The test above asserts that a fresh read contains the new category. That
// could also be true if the read never went through the cache at all, or if the
// cache were flushed for some unrelated reason. A third instance that shares the
// database but not the bus must still be serving its stale copy - and if that
// stops being true, the invalidation test above has stopped measuring the bus.
func TestAnInstanceWithoutTheBusKeepsItsCachedCopy(t *testing.T) {
	dbConn := dbtest.New(t)
	defer dbConn.Close()

	bus := cache.NewMemoryBus()
	defer bus.Close()

	writer := newCacheInstance(t, "writer", dbConn, bus)
	reader := newCacheInstance(t, "reader", dbConn, bus)
	isolated := newCacheInstance(t, "isolated", dbConn, nil)
	admin := adminTokenFor(t, writer, "cache-control-admin@example.com")

	warmCategoryCaches(t, reader, isolated)

	status, body := doRequest(t, writer.srv, http.MethodPost, "/admin/categories", admin,
		`{"name":"Control Probe","slug":"control-probe-category","status":"published"}`)
	if status != http.StatusCreated {
		t.Fatalf("create category: %d %s", status, truncateBody(body))
	}

	if got := fetchJSONBody(t, reader, "/categories"); !containsField(got, "control-probe-category") {
		t.Fatalf("the connected instance did not receive the invalidation:\n%s", truncateBody(got))
	}
	if got := fetchJSONBody(t, isolated, "/categories"); containsField(got, "control-probe-category") {
		t.Fatal("an instance with no bus saw the write, so this test cannot tell invalidation from a cache that was never warm")
	}
}

// TestPublishingAConfessionReachesOtherInstances walks the real editorial
// ladder rather than inserting a published row, because the ladder is the
// write path that has to invalidate.
func TestPublishingAConfessionReachesOtherInstances(t *testing.T) {
	dbConn := dbtest.New(t)
	defer dbConn.Close()

	bus := cache.NewMemoryBus()
	defer bus.Close()

	writer := newCacheInstance(t, "writer", dbConn, bus)
	reader := newCacheInstance(t, "reader", dbConn, bus)
	admin := adminTokenFor(t, writer, "confession-admin@example.com")

	status, body := doRequest(t, writer.srv, http.MethodPost, "/admin/categories", admin,
		`{"name":"Cache Confessions","slug":"cache-confessions","status":"published"}`)
	if status != http.StatusCreated {
		t.Fatalf("create category: %d %s", status, truncateBody(body))
	}
	categoryID := responseStringField(t, body, "id")

	// Warm the reader's per-category cache.
	listPath := "/categories/" + categoryID + "/confessions"
	if got := fetchJSONBody(t, reader, listPath); containsField(got, "cache-confession-probe") {
		t.Fatalf("probe confession present before it was created:\n%s", truncateBody(got))
	}

	status, body = doRequest(t, writer.srv, http.MethodPost, "/admin/confessions", admin,
		`{"category_id":"`+categoryID+`","title":"Cache Confession Probe","short_text":"probe"}`)
	if status != http.StatusCreated {
		t.Fatalf("create confession: %d %s", status, truncateBody(body))
	}
	confessionID := responseStringField(t, body, "id")

	// A draft confession is not served, so the reader's list stays empty and
	// the cache may legitimately hold the empty list.
	if got := fetchJSONBody(t, reader, listPath); containsField(got, "Cache Confession Probe") {
		t.Fatalf("a draft confession was served:\n%s", truncateBody(got))
	}

	// Walk the ladder to published. Each step is a real PATCH through the
	// admin handler.
	for _, to := range []string{
		string(content.StatusContentReview),
		string(content.StatusTheologicalReview),
		string(content.StatusAudioProduction),
		string(content.StatusAudioQA),
		string(content.StatusApproved),
		string(content.StatusPublished),
	} {
		status, body = doRequest(t, writer.srv, http.MethodPatch, "/admin/confessions/"+confessionID, admin,
			`{"status":"`+to+`","reason":"cache invalidation test"}`)
		if status != http.StatusOK {
			t.Fatalf("move to %s: %d %s", to, status, truncateBody(body))
		}
	}

	if got := fetchJSONBody(t, reader, listPath); !containsField(got, "Cache Confession Probe") {
		t.Fatalf("the published confession is not visible on the other instance:\n%s", truncateBody(got))
	}

	// Withdrawal has to travel too: a stale cache here serves withdrawn
	// content, which is the failure that matters most.
	status, body = doRequest(t, writer.srv, http.MethodPatch, "/admin/confessions/"+confessionID, admin,
		`{"status":"`+string(content.StatusArchived)+`","reason":"withdrawn in test"}`)
	if status != http.StatusOK {
		t.Fatalf("archive: %d %s", status, truncateBody(body))
	}
	if got := fetchJSONBody(t, reader, listPath); containsField(got, "Cache Confession Probe") {
		t.Fatalf("an archived confession is still being served from another instance's cache:\n%s", truncateBody(got))
	}
}

// TestCreatingAVoiceReachesOtherInstances covers the third cached surface.
func TestCreatingAVoiceReachesOtherInstances(t *testing.T) {
	dbConn := dbtest.New(t)
	defer dbConn.Close()

	bus := cache.NewMemoryBus()
	defer bus.Close()

	writer := newCacheInstance(t, "writer", dbConn, bus)
	reader := newCacheInstance(t, "reader", dbConn, bus)
	admin := adminTokenFor(t, writer, "voice-admin@example.com")

	if got := fetchJSONBody(t, reader, "/voices"); containsField(got, "Cache Probe Voice") {
		t.Fatalf("probe voice already present:\n%s", truncateBody(got))
	}

	status, body := doRequest(t, writer.srv, http.MethodPost, "/admin/voices", admin,
		`{"name":"Cache Probe Voice","type":"professional","language":"en","status":"active"}`)
	if status != http.StatusCreated {
		t.Fatalf("create voice: %d %s", status, truncateBody(body))
	}

	if got := fetchJSONBody(t, reader, "/voices"); !containsField(got, "Cache Probe Voice") {
		t.Fatalf("the new voice is not visible on the other instance:\n%s", truncateBody(got))
	}
}

// TestCacheInvalidationIsCountedForMetrics covers the observability half: a hit
// rate cannot distinguish a catalogue that never changes from one that changed
// and was not noticed.
func TestCacheInvalidationIsCountedForMetrics(t *testing.T) {
	dbConn := dbtest.New(t)
	defer dbConn.Close()

	bus := cache.NewMemoryBus()
	defer bus.Close()

	writer := newCacheInstance(t, "writer", dbConn, bus)
	reader := newCacheInstance(t, "reader", dbConn, bus)
	admin := adminTokenFor(t, writer, "metrics-admin@example.com")

	// Warm the reader so /metrics has a hit to report. The first read is a
	// miss that populates the cache; the second is the hit.
	warmCategoryCaches(t, reader)
	fetchJSONBody(t, reader, "/categories")

	adminReader := adminTokenFor(t, reader, "metrics-reader-admin@example.com")

	before := cacheStatsOf(t, reader, adminReader)
	status, body := doRequest(t, writer.srv, http.MethodPost, "/admin/categories", admin,
		`{"name":"Metrics Probe","slug":"metrics-probe","status":"published"}`)
	if status != http.StatusCreated {
		t.Fatalf("create category: %d %s", status, truncateBody(body))
	}

	after := cacheStatsOf(t, reader, adminReader)
	if after.Invalidations <= before.Invalidations {
		t.Fatalf("invalidations did not increase on the receiving instance: before=%d after=%d",
			before.Invalidations, after.Invalidations)
	}
	if after.Hits == 0 {
		t.Fatalf("no cache hit was recorded, so hit_rate is not measuring the read path: %+v", after)
	}
	// hit_rate has to be present in the payload, not just computable.
	raw := fetchJSONBodyAs(t, reader, "/metrics", adminReader)
	var payload struct {
		Cache map[string]float64 `json:"cache"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("parse /metrics: %v", err)
	}
	for _, field := range []string{"hit_rate", "invalidations"} {
		if _, ok := payload.Cache[field]; !ok {
			t.Fatalf("/metrics cache payload is missing %q: %s", field, truncateBody(raw))
		}
	}
}

func cacheStatsOf(t *testing.T, inst *instance, token string) CacheStats {
	t.Helper()
	raw := fetchJSONBodyAs(t, inst, "/metrics", token)
	var payload struct {
		Cache struct {
			Hits          int64 `json:"hits"`
			Misses        int64 `json:"misses"`
			Stale         int64 `json:"stale"`
			Invalidations int64 `json:"invalidations"`
		} `json:"cache"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("%s: parse /metrics: %v", inst.name, err)
	}
	return CacheStats{
		Hits:          payload.Cache.Hits,
		Misses:        payload.Cache.Misses,
		Stale:         payload.Cache.Stale,
		Invalidations: payload.Cache.Invalidations,
	}
}

func responseStringField(t *testing.T, body, key string) string {
	t.Helper()
	v, err := jsonField(body, key)
	if err != nil {
		t.Fatalf("field %q in %s: %v", key, truncateBody(body), err)
	}
	return v
}
