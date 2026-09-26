package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Teamthy/i-confess/internal/bible"
	"github.com/Teamthy/i-confess/internal/db/dbtest"
)

func TestLoadRefusesUnverifiedSources(t *testing.T) {
	err := runLoad([]string{"-skip-checksum", "-only", "kjv", "-dsn", "not-a-connection"})
	if err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("checksum bypass: %v", err)
	}
}

func TestLoadIdempotentAndImmutableAcrossSourceRevisions(t *testing.T) {
	conn := dbtest.New(t)
	ctx := context.Background()
	v, ok := bible.VersionByID("kjv")
	if !ok {
		t.Fatal("KJV registry entry missing")
	}
	// Use the already committed, source-derived citation fixture to exercise
	// the real COPY path without fetching the full translation. This test-only
	// ID can never replace the KJV source or be deployed as a translation.
	v.ID = "test-import-kjv"
	v.ProviderTranslationID = "test-import-kjv"
	fixture := filepath.Join("..", "..", "internal", "bible", "testdata", "cited-verses.osis.xml")
	digest, err := fileSHA256(fixture)
	if err != nil {
		t.Fatal(err)
	}
	v.SHA256 = digest
	file, err := os.Open(fixture)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := bible.Parse(file, v.Format, v.ID)
	_ = file.Close()
	if err != nil {
		t.Fatal(err)
	}
	// The citation fixture is intentionally incomplete; the full-canon
	// validator tests live in internal/bible. Only this loader test supplies
	// a report for its known subset of already source-derived verses.
	rep := &bible.Report{OK: true, VersionID: v.ID, Coverage: "new_testament", Books: len(parsed.Books), Verses: parsed.VerseCount()}
	for _, book := range parsed.Books {
		rep.Chapters += len(book.Chapters)
	}
	if rep.Verses < 1 {
		t.Fatal("citation fixture has no verses")
	}
	if err := loadVersion(ctx, conn, v, parsed, rep); err != nil {
		t.Fatalf("first import: %v", err)
	}
	var count int
	if err := conn.QueryRowContext(ctx, `SELECT count(*) FROM bible_verses WHERE version_id=?`, v.ID).Scan(&count); err != nil || count != rep.Verses {
		t.Fatalf("imported rows=%d, want %d: %v", count, rep.Verses, err)
	}
	if err := loadVersion(ctx, conn, v, parsed, rep); err != nil {
		t.Fatalf("repeat import must leave source text intact: %v", err)
	}
	if err := conn.QueryRowContext(ctx, `SELECT count(*) FROM bible_verses WHERE version_id=?`, v.ID).Scan(&count); err != nil || count != rep.Verses {
		t.Fatalf("repeat changed imported rows=%d: %v", count, err)
	}
	changed := v
	changed.SHA256 = strings.Repeat("a", 64)
	if err := loadVersion(ctx, conn, changed, parsed, rep); err == nil || !strings.Contains(err.Error(), "revisions require a new translation ID") {
		t.Fatalf("revised source replaced immutable verses: %v", err)
	}
	var digestAfter string
	if err := conn.QueryRowContext(ctx, `SELECT sha256 FROM bible_versions WHERE id=?`, v.ID).Scan(&digestAfter); err != nil || digestAfter != digest {
		t.Fatalf("rejected revision changed source digest: %q (%v)", digestAfter, err)
	}
	if _, err := conn.ExecContext(ctx, `UPDATE bible_verses SET text='not the source' WHERE version_id=?`, v.ID); err == nil {
		t.Fatal("canonical verse text was mutable")
	}
	if _, err := conn.ExecContext(ctx, `DELETE FROM bible_verses WHERE version_id=?`, v.ID); err == nil {
		t.Fatal("canonical verses could be removed silently")
	}
	if _, err := conn.ExecContext(ctx, `UPDATE bible_versions SET content_hash='not the source' WHERE id=?`, v.ID); err == nil {
		t.Fatal("source content digest was mutable")
	}
}
