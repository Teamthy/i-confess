package bible

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestHelloAOProviderNormalizesChapterWithoutLeakingWireTypes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/available_translations.json":
			_, _ = w.Write([]byte(`{"translations":[{"id":"BSB","name":"Berean Standard Bible","englishName":"Berean Standard Bible","shortName":"BSB","language":"eng","languageEnglishName":"English","textDirection":"ltr","licenseUrl":"https://example.test/terms","website":"https://example.test","numberOfBooks":66,"totalNumberOfChapters":1189,"totalNumberOfVerses":31086}]}`))
		case "/api/BSB/books.json":
			_, _ = w.Write([]byte(`{"books":[{"id":"GEN","name":"Genesis","commonName":"Genesis","order":1,"numberOfChapters":50}]}`))
		case "/api/BSB/GEN/1.simple.json":
			_, _ = w.Write([]byte(`{"translation":{"id":"BSB"},"book":{"id":"GEN"},"chapter":{"number":1,"content":[{"type":"heading","text":"Creation"},{"type":"verse","number":1,"text":"In the beginning."}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider, err := NewHelloAOBibleProvider(server.URL+"/api", &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	chapter, err := provider.GetChapter(context.Background(), "bsb", "Gen", 1)
	if err != nil {
		t.Fatalf("GetChapter: %v", err)
	}
	if chapter.Translation.Provider != "helloao" || chapter.Translation.Status != "pending_review" {
		t.Fatalf("unexpected provider/review status: %#v", chapter.Translation)
	}
	if chapter.Translation.APIExposureAllowed || chapter.Translation.AudioAllowed || chapter.Translation.OfflineAllowed {
		t.Fatal("unreviewed translation acquired rights by default")
	}
	if chapter.Book.ID != "Gen" || len(chapter.Verses) != 1 || chapter.Verses[0].ID != "GEN.1.1" || chapter.Verses[0].Text != "In the beginning." {
		t.Fatalf("chapter was not normalized: %#v", chapter)
	}
}

func TestHelloAOProviderRejectsRemoteHTTP(t *testing.T) {
	if _, err := NewHelloAOBibleProvider("http://bible.example.test/api", nil); err == nil {
		t.Fatal("remote HTTP should be rejected")
	}
}

func TestHelloAOProviderAllowsOnlyItsPinnedHostAndRejectsRedirects(t *testing.T) {
	for _, base := range []string{
		"https://unreviewed.example.test/api",
		"http://127.0.0.1.evil.example/api",
		"https://bible.helloao.org:8443/api",
		"https://user:pass@bible.helloao.org/api",
	} {
		if _, err := NewHelloAOBibleProvider(base, nil); err == nil {
			t.Errorf("accepted provider origin %q", base)
		}
	}
	if _, err := NewHelloAOBibleProvider("https://bible.helloao.org/api", nil); err != nil {
		t.Fatalf("rejected official provider: %v", err)
	}
	var redirected atomic.Int64
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redirected.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer other.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/internal", http.StatusFound)
	}))
	defer upstream.Close()
	provider, err := NewHelloAOBibleProvider(upstream.URL+"/api", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.GetTranslations(context.Background(), "", ""); err != ErrUnavailable {
		t.Fatalf("redirected catalog should be unavailable: %v", err)
	}
	if got := redirected.Load(); got != 0 {
		t.Fatalf("provider followed %d cross-host redirects", got)
	}
}

func TestHelloAOTranslationMetadataDoesNotAssumeRights(t *testing.T) {
	translation := normalizeHelloAOTranslation(helloAOTranslationWire{ID: "KJV", Name: "King James", LicenseURL: "https://example.test/license", Language: "eng"})
	if translation.Status != "pending_review" {
		t.Fatalf("status = %q", translation.Status)
	}
	if translation.PublicDomain || translation.RedistributionAllowed || translation.CommercialUse || translation.AudioAllowed || translation.OfflineAllowed || translation.APIExposureAllowed {
		t.Fatal("provider metadata incorrectly granted license rights")
	}
	if !strings.Contains(translation.LicenseURL, "license") {
		t.Fatalf("license URL missing: %#v", translation)
	}
}

func TestHelloAOProviderUsesCaseSensitiveCatalogID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/available_translations.json":
			_, _ = w.Write([]byte(`{"translations":[{"id":"aai_wbt","name":"Fixture version","language":"eng","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]}`))
		case "/api/aai_wbt/books.json":
			_, _ = w.Write([]byte(`{"translation":{"id":"aai_wbt","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"books":[{"id":"GEN","commonName":"Genesis","order":1,"numberOfChapters":1}]}`))
		case "/api/aai_wbt/GEN/1.simple.json":
			_, _ = w.Write([]byte(`{"translation":{"id":"aai_wbt","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"book":{"id":"GEN"},"chapter":{"number":1,"content":[{"type":"verse","number":1,"text":"Provider test fixture, not Scripture."}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	provider, err := NewHelloAOBibleProvider(server.URL+"/api", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	chapter, err := provider.GetChapter(context.Background(), "AAI_WBT", "Gen", 1)
	if err != nil || len(chapter.Verses) != 1 || chapter.Translation.ContentHash == "" {
		t.Fatalf("provider ID casing or source hash was lost: %#v (%v)", chapter, err)
	}
}
