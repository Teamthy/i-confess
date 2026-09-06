package api

import (
	"context"
	"net/http"
	"testing"
)

// TestNewestVersionIsSelected covers the PHASE 15 finding.
//
// AssetsFor ordered by created_at ASC and matchAsset's fallback took assets[0],
// so the engine picked the OLDEST render for a confession. Once content
// versioning exists that means editing a confession and re-voicing it leaves
// every new session playing audio rendered from the superseded text - the
// listener hears words that are no longer what the confession says, and nothing
// in the system surfaces it.
//
// Proven before the fix: adding a v2 render left the engine selecting v1.
func TestNewestVersionIsSelected(t *testing.T) {
	f := newAudioFixture(t)
	ctx := context.Background()

	code, before := f.createSession(t, f.freeTok, f.voiceStd, 60)
	if code != http.StatusCreated {
		t.Fatalf("create session: %d", code)
	}
	if len(before.Items) == 0 {
		t.Fatal("no items, so this test proves nothing")
	}
	original := before.Items[0].AudioAssetID

	var confID, voiceID, key, variantID string
	if err := f.db.QueryRowContext(ctx,
		`SELECT content_id, voice_id, COALESCE(cdn_path, storage_key), COALESCE(variant_id,'')
		 FROM audio_assets WHERE id = $1`, original).Scan(&confID, &voiceID, &key, &variantID); err != nil {
		t.Fatal(err)
	}
	if variantID == "" {
		t.Fatal("the fixture's asset records no variant, so variant matching cannot be exercised")
	}

	// A newer render, made after the text was edited - exactly what generation
	// produces, down to the content version it is attributed to.
	if _, err := f.db.ExecContext(ctx,
		`INSERT INTO content_versions (id, confession_id, version_number, title, short_text,
		   medium_text, long_text, language, status, created_at, updated_at)
		 VALUES ('cv-2', $1, 2, 'edited', 'a', 'b', 'c', 'en', 'approved',
		         '2027-01-01T00:00:00Z', '2027-01-01T00:00:00Z')`, confID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecContext(ctx,
		`INSERT INTO audio_assets (id, content_id, content_version_id, voice_id, variant_id,
		   asset_type, quality_tier, storage_provider, storage_key, format, codec, container,
		   duration_seconds, file_size_bytes, status, created_at, updated_at)
		 SELECT 'render-v2', content_id, 'cv-2', voice_id, variant_id, asset_type, quality_tier,
		        storage_provider, $2, format, codec, container, duration_seconds,
		        file_size_bytes, 'ready', '2027-01-01T00:00:00Z', '2027-01-01T00:00:00Z'
		 FROM audio_assets WHERE id = $1`, original, key); err != nil {
		t.Fatal(err)
	}

	_, after := f.createSession(t, f.freeTok, f.voiceStd, 60)
	if len(after.Items) == 0 {
		t.Fatal("no items after re-voicing")
	}
	for _, it := range after.Items {
		if it.ConfessionID != confID {
			continue
		}
		if it.AudioAssetID != "render-v2" {
			t.Errorf("session selected %q, not the render made from the current text (render-v2)",
				it.AudioAssetID)
		}
	}
}

// TestAssetsAreSelectedByVariant covers the other half of the same finding.
//
// audio_assets had no variant_id column, so every read filled the field with a
// ” literal and matchAsset's variant branch could never match: every selection
// fell through to "any asset for this confession". A confession voiced at
// several lengths therefore had all of its renders treated as interchangeable.
func TestAssetsAreSelectedByVariant(t *testing.T) {
	f := newAudioFixture(t)
	ctx := context.Background()

	var confID, voiceID, key string
	if err := f.db.QueryRowContext(ctx,
		`SELECT content_id, voice_id, COALESCE(cdn_path, storage_key) FROM audio_assets LIMIT 1`,
	).Scan(&confID, &voiceID, &key); err != nil {
		t.Fatal(err)
	}

	// Two more variants on the same confession, each with its own render.
	variants := []string{"var-short", "var-long"}
	for i, v := range variants {
		if _, err := f.db.ExecContext(ctx,
			`INSERT INTO confession_variants (id, confession_id, label, duration_seconds, sort_order)
			 VALUES ($1, $2, $1, $3, $4)`, v, confID, 60*(i+2), i+10); err != nil {
			t.Fatalf("create variant %s: %v", v, err)
		}
		if _, err := f.db.ExecContext(ctx,
			`INSERT INTO audio_assets (id, content_id, content_version_id, voice_id, variant_id,
			   asset_type, quality_tier, storage_provider, storage_key, format, codec, container,
			   duration_seconds, file_size_bytes, status, created_at, updated_at)
			 SELECT $1, content_id, content_version_id, voice_id, $1, asset_type, quality_tier,
			        storage_provider, $2, format, codec, container, duration_seconds,
			        file_size_bytes, 'ready', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'
			 FROM audio_assets WHERE voice_id = $3 LIMIT 1`, v, key+"/"+v+".m4a", voiceID); err != nil {
			t.Fatal(err)
		}
	}

	// Read back through the same path the engine uses.
	assets, err := f.h.audio.AssetsFor(ctx, confID, voiceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) < 3 {
		t.Fatalf("got %d assets, want at least 3 (one per variant)", len(assets))
	}

	seen := map[string]bool{}
	for _, a := range assets {
		if a.VariantID == "" {
			t.Errorf("asset %s has no variant recorded; variant matching cannot work", a.ID)
		}
		seen[a.VariantID] = true
	}
	for _, v := range variants {
		if !seen[v] {
			t.Errorf("variant %q has no asset; the variant column is not being read", v)
		}
	}
}
