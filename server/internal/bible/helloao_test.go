package bible

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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
	if err != nil { t.Fatal(err) }
	chapter, err := provider.GetChapter(context.Background(), "bsb", "Gen", 1)
	if err != nil { t.Fatalf("GetChapter: %v", err) }
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

func TestHelloAOTranslationMetadataDoesNotAssumeRights(t *testing.T) {
	translation := normalizeHelloAOTranslation(helloAOTranslationWire{ID:"KJV", Name:"King James", LicenseURL:"https://example.test/license", Language:"eng"})
	if translation.Status != "pending_review" { t.Fatalf("status = %q", translation.Status) }
	if translation.PublicDomain || translation.RedistributionAllowed || translation.CommercialUse || translation.AudioAllowed || translation.OfflineAllowed || translation.APIExposureAllowed {
		t.Fatal("provider metadata incorrectly granted license rights")
	}
	if !strings.Contains(translation.LicenseURL,"license") { t.Fatalf("license URL missing: %#v", translation) }
}
