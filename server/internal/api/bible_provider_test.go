package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Teamthy/i-confess/internal/bible"
	"github.com/Teamthy/i-confess/internal/db/dbtest"
)

func TestHelloAOCatalogPinsProviderEditionAndFailsClosedOnRevision(t *testing.T) {
	const firstHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const nextHash = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	var currentHash atomic.Value
	currentHash.Store(firstHash)
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		hash := currentHash.Load().(string)
		switch r.URL.Path {
		case "/api/available_translations.json":
			fmt.Fprintf(w, `{"translations":[{"id":"Testabc","name":"Test-only source fixture","language":"eng","languageEnglishName":"English","shortName":"TEST","textDirection":"ltr","sha256":"%s","website":"https://example.test/source","licenseUrl":"https://example.test/license","numberOfBooks":1,"totalNumberOfChapters":2,"totalNumberOfVerses":2}]}`, hash)
		case "/api/Testabc/books.json":
			fmt.Fprintf(w, `{"translation":{"id":"Testabc","sha256":"%s"},"books":[{"id":"GEN","commonName":"Genesis","order":1,"numberOfChapters":2}]}`, hash)
		case "/api/Testabc/GEN/1.simple.json", "/api/Testabc/GEN/2.simple.json":
			chapter := 1
			if strings.Contains(r.URL.Path, "/2.simple.json") {
				chapter = 2
			}
			fmt.Fprintf(w, `{"translation":{"id":"Testabc","sha256":"%s"},"book":{"id":"GEN"},"chapter":{"number":%d,"content":[{"type":"verse","number":1,"text":"Provider test fixture, not Scripture."}]}}`, hash, chapter)
		default:
			http.NotFound(w, r)
		}
	}))
	defer remote.Close()

	conn := dbtest.New(t)
	inst := newCacheInstance(t, "Bible provider", conn, nil)
	admin := adminTokenFor(t, inst, "provider-rights-admin@example.com")
	provider, err := bible.NewHelloAOBibleProvider(remote.URL+"/api", remote.Client())
	if err != nil {
		t.Fatal(err)
	}
	inst.h.SetBibleDiscoveryProvider(provider)
	inst.h.SetBibleProvider(&bible.ReviewedProvider{Local: &bible.LocalBibleProvider{DB: conn}, Remote: provider})

	call := func(method, path, token, body string, want int) string {
		t.Helper()
		status, response := doRequest(t, inst.srv, method, path, token, body)
		if status != want {
			t.Fatalf("%s %s: got %d, want %d: %s", method, path, status, want, response)
		}
		return response
	}
	if response := call("GET", "/bible/translations", "", "", http.StatusOK); strings.Contains(response, "helloao-testabc") {
		t.Fatalf("unreviewed remote translation exposed: %s", response)
	}
	call("POST", "/admin/bible/catalog/sync", admin, `{}`, http.StatusOK)
	var pinned string
	if err := conn.QueryRow(`SELECT content_hash FROM bible_versions WHERE id='helloao-testabc'`).Scan(&pinned); err != nil || pinned != firstHash {
		t.Fatalf("catalog did not pin the provider digest: %q (%v)", pinned, err)
	}
	grants := `{"decision":"approved","public_domain":false,"commercial_use":false,"redistribution_allowed":false,"modification_allowed":false,"audio_allowed":false,"offline_allowed":false,"offline_max_days":0,"copy_allowed":false,"share_allowed":false,"search_index_allowed":false,"api_exposure_allowed":true,"attribution_required":true,"attribution_text":"Fixture source, test only","evidence_url":"https://example.test/license","rationale":"Test fixture rights reviewed for this isolated database."}`
	call("POST", "/admin/bible/translations/helloao-testabc/rights", admin, grants, http.StatusOK)
	if response := call("GET", "/bible/translations", "", "", http.StatusOK); strings.Count(response, `"id":"helloao-testabc"`) != 1 {
		t.Fatalf("remote translation missing or listed twice: %s", response)
	}
	if response := call("GET", "/bible/helloao-testabc/Gen/1", "", "", http.StatusOK); !strings.Contains(response, "Provider test fixture") {
		t.Fatalf("reviewed remote chapter unavailable: %s", response)
	}

	// Upstream changes the edition behind the same ID. An already-cached old
	// chapter can remain immutable, but an uncached new chapter must not leak.
	currentHash.Store(nextHash)
	call("GET", "/bible/helloao-testabc/Gen/2", "", "", http.StatusServiceUnavailable)
	refreshed, err := bible.NewHelloAOBibleProvider(remote.URL+"/api", remote.Client())
	if err != nil {
		t.Fatal(err)
	}
	inst.h.SetBibleDiscoveryProvider(refreshed)
	inst.h.SetBibleProvider(&bible.ReviewedProvider{Local: &bible.LocalBibleProvider{DB: conn}, Remote: refreshed})
	if response := call("GET", "/bible/translations", "", "", http.StatusOK); strings.Contains(response, "helloao-testabc") {
		t.Fatalf("revised remote edition still listed as approved: %s", response)
	}
	call("POST", "/admin/bible/translations/helloao-testabc/rights", admin, grants, http.StatusConflict)
	call("POST", "/admin/bible/catalog/sync", admin, `{}`, http.StatusOK)
	if err := conn.QueryRow(`SELECT content_hash FROM bible_versions WHERE id='helloao-testabc'`).Scan(&pinned); err != nil || pinned != firstHash {
		t.Fatalf("catalog sync silently replaced reviewed edition: %q (%v)", pinned, err)
	}

	// Even without a reachable provider, admin review records must remain
	// inspectable. Only approvals/content reads require upstream verification.
	remote.Close()
	disconnected, err := bible.NewHelloAOBibleProvider(remote.URL+"/api", remote.Client())
	if err != nil {
		t.Fatal(err)
	}
	inst.h.SetBibleDiscoveryProvider(disconnected)
	if response := call("GET", "/admin/bible/catalog", admin, "", http.StatusOK); !strings.Contains(response, `"provider_available":false`) || !strings.Contains(response, `"registry_id":"helloao-testabc"`) || !strings.Contains(response, `"remote_unavailable":true`) {
		t.Fatalf("admin review queue disappeared during an upstream outage: %s", response)
	}
	if response := call("GET", "/admin/bible/catalog?language=fr", admin, "", http.StatusOK); strings.Contains(response, `"registry_id":"helloao-testabc"`) {
		t.Fatalf("offline admin catalog ignored language filter: %s", response)
	}
}
