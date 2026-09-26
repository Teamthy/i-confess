// Command bible-import fetches, verifies and loads the public-domain Bible
// translations the platform ships.
//
// It exists as a separate binary rather than as a startup task because the
// work is large (twelve translations, roughly a quarter of a million verses)
// and rare (a version changes when a publisher revises the text, which is
// years apart). Doing it at boot would make every deploy depend on a third
// party being reachable, and doing it behind an admin endpoint would make the
// largest write in the system triggerable by an HTTP request.
//
// Nothing here ships Bible text in the repository. The repository ships the
// registry - which translation, under which licence, with which checksum - and
// an operator supplies the sources. That is what lets the text stay out of Git
// while the decisions about it stay reviewable in a pull request, and it is
// why the checksum in the registry is load-bearing: it is the only thing
// standing between "we verified this file" and "this is whatever the URL
// returned today".
//
// Usage:
//
//	bible-import fetch   [-sources DIR] [-only kjv,web]
//	bible-import verify  [-sources DIR] [-only kjv,web] [-json] [-allow-missing]
//	bible-import fixture [-out PATH]            (regenerate the CI fixture)
//	bible-import load    [-sources DIR] [-only kjv,web] [-dsn DSN]
package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Teamthy/i-confess/internal/bible"
	"github.com/Teamthy/i-confess/internal/db"
	"github.com/Teamthy/i-confess/internal/seed"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]

	var err error
	switch cmd {
	case "fetch":
		err = runFetch(args)
	case "verify":
		err = runVerify(args)
	case "fixture":
		err = runFixture(args)
	case "load":
		err = runLoad(args)
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "bible-import: unknown command %q\n\n", cmd)
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "bible-import: %s: %v\n", cmd, err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `bible-import - fetch, verify and load public-domain Bible translations

Commands:
  fetch     Download source files into the sources directory
  verify    Verify checksums, parse each source and validate it against the canon
  fixture   Regenerate the committed CI fixture from the confession corpus
  load      Verify and load translations into PostgreSQL

Common flags:
  -sources DIR   Where source XML files live (default $BIBLE_SOURCES or ./bible-src)
  -only LIST     Comma-separated version IDs to act on (default: all)

verify flags:
  -json            Emit machine-readable reports
  -allow-missing   Skip versions whose source file is absent instead of failing

load flags:
  -dsn DSN   lib/pq connection string (default $DATABASE_URL)
`)
}

// sourcesDir resolves the directory the source files are read from.
func sourcesDir(fs *flag.FlagSet) *string {
	def := strings.TrimSpace(os.Getenv("BIBLE_SOURCES"))
	if def == "" {
		def = "bible-src"
	}
	return fs.String("sources", def, "directory holding source XML files")
}

func selected(list string) ([]bible.Version, error) {
	if strings.TrimSpace(list) == "" {
		return bible.Versions, nil
	}
	out := make([]bible.Version, 0, 4)
	for _, id := range strings.Split(list, ",") {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		v, ok := bible.VersionByID(id)
		if !ok {
			return nil, fmt.Errorf("unknown version %q", id)
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil, errors.New("no versions selected")
	}
	return out, nil
}

// ---------- fetch ----------

func runFetch(args []string) error {
	fs := flag.NewFlagSet("fetch", flag.ExitOnError)
	dir := sourcesDir(fs)
	only := fs.String("only", "", "comma-separated version IDs")
	force := fs.Bool("force", false, "re-download even when the checksum already matches")
	if err := fs.Parse(args); err != nil {
		return err
	}
	versions, err := selected(*only)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(*dir, 0o755); err != nil {
		return err
	}

	client := &http.Client{Timeout: 30 * time.Minute}
	for _, v := range versions {
		path := filepath.Join(*dir, v.SourceFile)
		if !*force {
			if ok, err := fileMatches(path, v); err == nil && ok {
				fmt.Printf("  ok      %-9s %s (checksum matches)\n", v.ID, v.SourceFile)
				continue
			}
		}
		fmt.Printf("  fetch   %-9s %s\n", v.ID, v.SourceFile)
		if err := download(client, v, path); err != nil {
			return fmt.Errorf("%s: %w", v.ID, err)
		}
		if ok, err := fileMatches(path, v); err != nil || !ok {
			// A file that does not match the registry digest is removed rather
			// than left on disk: leaving it invites a later run to import it.
			_ = os.Remove(path)
			return fmt.Errorf("%s: downloaded file does not match the registry checksum (%s)", v.ID, v.SHA256)
		}
		fmt.Printf("  ok      %-9s %s\n", v.ID, v.SourceFile)
	}
	return nil
}

func download(client *http.Client, v bible.Version, path string) error {
	req, err := http.NewRequest(http.MethodGet, v.FetchURL(), nil)
	if err != nil {
		return err
	}
	// The contents API returns metadata unless raw content is requested.
	req.Header.Set("Accept", "application/vnd.github.raw")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("http %d from %s: %s", resp.StatusCode, v.FetchURL(), strings.TrimSpace(string(body)))
	}

	tmp := path + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, resp.Body)
	closeErr := f.Close()
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	// The API answers a missing path with a small JSON error body and a 200 in
	// some proxy configurations; a source file is never this small, so treat a
	// tiny body as a failed fetch rather than a small Bible.
	if n < 64*1024 {
		_ = os.Remove(tmp)
		return fmt.Errorf("response was %d bytes, too small to be a Bible (is the filename right?)", n)
	}
	return os.Rename(tmp, path)
}

// fileMatches reports whether the file on disk has the registry's size and
// digest.
func fileMatches(path string, v bible.Version) (bool, error) {
	st, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	if st.Size() != v.Bytes {
		return false, nil
	}
	sum, err := fileSHA256(path)
	if err != nil {
		return false, err
	}
	return sum == v.SHA256, nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ---------- verify ----------

type verifyResult struct {
	Report *bible.Report `json:"report"`
	Error  string        `json:"error,omitempty"`
	File   string        `json:"file,omitempty"`
}

func runVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	dir := sourcesDir(fs)
	only := fs.String("only", "", "comma-separated version IDs")
	asJSON := fs.Bool("json", false, "emit JSON reports")
	allowMissing := fs.Bool("allow-missing", false, "skip versions with no source file")
	skipChecksum := fs.Bool("skip-checksum", false, "parse without checking the registry digest")
	if err := fs.Parse(args); err != nil {
		return err
	}
	versions, err := selected(*only)
	if err != nil {
		return err
	}

	fmt.Printf("Verifying %d translations against the %d-book canon (%d chapters, KJV reference of %d verses)\n\n",
		len(versions), len(bible.Canon), bible.CanonicalChapterCount(), bible.ReferenceVerseCount())

	results := make([]verifyResult, 0, len(versions))
	failed, missing := 0, 0

	for _, v := range versions {
		path := filepath.Join(*dir, v.SourceFile)
		if _, err := os.Stat(path); err != nil {
			if *allowMissing {
				fmt.Printf("  %-9s SKIPPED - no source file at %s\n", v.ID, path)
				missing++
				continue
			}
			return fmt.Errorf("%s: source file missing at %s (run `bible-import fetch`)", v.ID, path)
		}
		if !*skipChecksum {
			sum, err := fileSHA256(path)
			if err != nil {
				return fmt.Errorf("%s: %w", v.ID, err)
			}
			if sum != v.SHA256 {
				failed++
				res := verifyResult{Error: fmt.Sprintf("checksum mismatch: file is %s, registry expects %s", sum, v.SHA256), File: path}
				results = append(results, res)
				fmt.Printf("  %-9s CHECKSUM MISMATCH (file %s, registry %s)\n", v.ID, sum[:12], v.SHA256[:12])
				continue
			}
		}

		rep, err := verifyFile(path, v)
		if err != nil {
			failed++
			results = append(results, verifyResult{Error: err.Error(), File: path})
			fmt.Printf("  %-9s PARSE ERROR: %v\n", v.ID, err)
			continue
		}
		results = append(results, verifyResult{Report: rep, File: path})
		fmt.Println("  " + rep.Summary())
		if !rep.OK {
			failed++
			fmt.Print(rep.Detail())
		}
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(results); err != nil {
			return err
		}
	}

	fmt.Println()
	switch {
	case failed > 0:
		return fmt.Errorf("%d of %d translations failed verification", failed, len(versions))
	case missing > 0:
		fmt.Printf("%d translations skipped (no source file); %d verified\n", missing, len(versions)-missing)
	default:
		if *skipChecksum {
			fmt.Printf("All %d translations structurally checked; checksums were NOT verified. Do not treat these sources as loadable.\n", len(versions))
		} else {
			fmt.Printf("All %d translations verified: checksums match, structure matches the canon, every verse has text.\n", len(versions))
		}
	}
	return nil
}

// verifyFile parses one source file and validates it.
func verifyFile(path string, v bible.Version) (*bible.Report, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	t, err := bible.Parse(f, v.Format, v.ID)
	if err != nil {
		return nil, err
	}
	rep := bible.Validate(t, v)
	// The coverage label is a claim about the file. If the file turns out to
	// contain the whole canon, saying "New Testament only" would make the
	// reader hide books it actually has.
	if rep.Coverage == bible.CoverageNewTestament && len(rep.MissingBooks) == 0 {
		rep.Findings = append(rep.Findings, bible.Finding{
			Severity: bible.SeverityError, Subject: "coverage",
			Detail: "registry says New Testament only but every canonical book is present",
		})
		rep.OK = false
	}
	return rep, nil
}

// ---------- fixture ----------

// runFixture regenerates the committed CI fixture.
//
// The fixture contains exactly the verses the canonical confession corpus
// cites, in the KJV dialect the parser finds hardest (milestone verses plus a
// non-canonical division to discard). CI can therefore exercise reference
// resolution, the confession-to-verse crosswalk and the reader's text path
// without a 60MB corpus in Git, and it stays small enough to diff in review.
func runFixture(args []string) error {
	fs := flag.NewFlagSet("fixture", flag.ExitOnError)
	dir := sourcesDir(fs)
	out := fs.String("out", filepath.Join("internal", "bible", "testdata", "cited-verses.osis.xml"), "output path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	v := bible.DefaultVersion()
	path := filepath.Join(*dir, v.SourceFile)
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w (the fixture is a slice of the KJV source; fetch it first)", path, err)
	}
	defer f.Close()
	src, err := bible.Parse(f, v.Format, v.ID)
	if err != nil {
		return err
	}

	refs, err := citedReferences()
	if err != nil {
		return err
	}

	// Group the cited verses by book and chapter so the fixture reads like a
	// Bible rather than a list of loose verses.
	type key struct {
		book    string
		chapter int
	}
	byChapter := map[key][]int{}
	bookOrder := []string{}
	seenBook := map[string]bool{}
	for _, ref := range refs {
		k := key{ref.Book, ref.Chapter}
		if len(byChapter[k]) == 0 {
			if !seenBook[ref.Book] {
				seenBook[ref.Book] = true
				bookOrder = append(bookOrder, ref.Book)
			}
		}
		for _, n := range ref.Verses {
			if !containsInt(byChapter[k], n) {
				byChapter[k] = append(byChapter[k], n)
			}
		}
	}
	// Emit books in canonical order, chapters ascending.
	sort.SliceStable(bookOrder, func(i, j int) bool {
		a, _ := bible.BookByID(bookOrder[i])
		b, _ := bible.BookByID(bookOrder[j])
		return a.Order() < b.Order()
	})

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	b.WriteString("<!--\n")
	b.WriteString("  cited-verses.osis.xml - a slice of the King James Version (public domain).\n\n")
	b.WriteString("  Generated by `go run ./cmd/bible-import fixture`. Do not edit by hand.\n")
	b.WriteString("  It contains every verse cited by the canonical confession corpus, with the\n")
	b.WriteString("  text taken verbatim from the imported KJV source, plus a front-matter\n")
	b.WriteString("  division that the parser must discard. It exists so CI can verify that\n")
	b.WriteString("  every Scripture reference in the corpus resolves to a real verse without\n")
	b.WriteString("  carrying the full 10MB translation in Git.\n\n")
	b.WriteString("  internal/bible/fixture_test.go fails when a confession cites a verse this\n")
	b.WriteString("  file does not contain, which keeps it in step with the corpus.\n")
	b.WriteString("-->\n")
	b.WriteString(`<osis xmlns="http://www.bibletechnologies.net/2003/OSIS/namespace">` + "\n")
	b.WriteString(`  <osisText osisIDWork="Bible.en.kjv" xml:lang="en">` + "\n")
	b.WriteString("    <header><revisionDesc><p>Slice of the King James Version for tests.</p></revisionDesc></header>\n")
	// A division that is not canonical: the parser must not import it, and the
	// test asserts it does not.
	b.WriteString(`    <div type="bookGroup"><title>Front Matter</title>` + "\n")
	b.WriteString(`      <div type="book" osisID="FrontMatter"><chapter osisID="FrontMatter.1"/>` + "\n")
	b.WriteString(`        <verse osisID="FrontMatter.1.1" sID="FrontMatter.1.1.seID.1" n="1" />Not Scripture.<verse eID="FrontMatter.1.1.seID.1" />` + "\n")
	b.WriteString("      </div>\n    </div>\n")
	b.WriteString(`    <div type="bookGroup">` + "\n")
	for _, bookID := range bookOrder {
		cb, _ := bible.BookByID(bookID)
		fmt.Fprintf(&b, `      <div type="book" osisID="%s"><title>%s</title>`+"\n", bookID, xmlEscape(cb.Name))
		chapters := []int{}
		for k := range byChapter {
			if k.book == bookID {
				chapters = append(chapters, k.chapter)
			}
		}
		sort.Ints(chapters)
		for _, ch := range chapters {
			verses := byChapter[key{bookID, ch}]
			sort.Ints(verses)
			fmt.Fprintf(&b, `        <chapter osisID="%s.%d" sID="%s.%d.seID.1" />`+"\n", bookID, ch, bookID, ch)
			for _, n := range verses {
				text, ok := src.Text(bookID, ch, n)
				if !ok {
					return fmt.Errorf("KJV source has no %s %d:%d, cited by the confession corpus", cb.Name, ch, n)
				}
				fmt.Fprintf(&b, `        <verse osisID="%s.%d.%d" sID="%s.%d.%d.seID.1" n="%d" />%s<verse eID="%s.%d.%d.seID.1" />`+"\n",
					bookID, ch, n, bookID, ch, n, n, xmlEscape(text), bookID, ch, n)
			}
		}
		b.WriteString("      </div>\n")
	}
	b.WriteString("    </div>\n  </osisText>\n</osis>\n")

	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(*out, []byte(b.String()), 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %s: %d books, %d citations\n", *out, len(bookOrder), len(refs))
	return nil
}

// citedReferences normalises every Scripture reference in the canonical
// confession corpus.
func citedReferences() ([]bible.Reference, error) {
	var out []bible.Reference
	for _, c := range seed.CanonicalConfessions {
		for _, s := range c.Scriptures {
			ref, err := bible.NewReference(s.Book, s.Chapter, s.Verse)
			if err != nil {
				return nil, fmt.Errorf("confession %q cites %s %d:%s: %w", c.Title, s.Book, s.Chapter, s.Verse, err)
			}
			out = append(out, ref)
		}
	}
	return out, nil
}

func xmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}

func containsInt(xs []int, n int) bool {
	for _, x := range xs {
		if x == n {
			return true
		}
	}
	return false
}

// ---------- load ----------

func runLoad(args []string) error {
	fs := flag.NewFlagSet("load", flag.ExitOnError)
	dir := sourcesDir(fs)
	only := fs.String("only", "", "comma-separated version IDs")
	dsn := fs.String("dsn", os.Getenv("DATABASE_URL"), "lib/pq connection string")
	allowMissing := fs.Bool("allow-missing", false, "skip versions with no source file")
	skipChecksum := fs.Bool("skip-checksum", false, "not permitted for load; retained only to reject unsafe invocations")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *skipChecksum {
		return errors.New("loading Bible text without its registry checksum is forbidden")
	}
	versions, err := selected(*only)
	if err != nil {
		return err
	}
	if strings.TrimSpace(*dsn) == "" {
		return errors.New("no connection string: pass -dsn or set DATABASE_URL")
	}

	// Every version is verified before any of them is written. A partial
	// import is the worst outcome available here: a reader would get some
	// translations current and others stale, and nothing in the data says
	// which.
	type loaded struct {
		v    bible.Version
		text *bible.ParsedTranslation
		rep  *bible.Report
	}
	parsed := make([]loaded, 0, len(versions))
	for _, v := range versions {
		path := filepath.Join(*dir, v.SourceFile)
		if _, err := os.Stat(path); err != nil {
			if *allowMissing {
				fmt.Printf("  skip    %-9s no source file\n", v.ID)
				continue
			}
			return fmt.Errorf("%s: source file missing at %s", v.ID, path)
		}
		sum, err := fileSHA256(path)
		if err != nil {
			return err
		}
		if sum != v.SHA256 {
			return fmt.Errorf("%s: checksum mismatch (file %s, registry %s)", v.ID, sum, v.SHA256)
		}
		rep, text, err := parseAndValidate(path, v)
		if err != nil {
			return fmt.Errorf("%s: %w", v.ID, err)
		}
		if !rep.OK {
			return fmt.Errorf("%s failed verification:\n%s", v.ID, rep.Detail())
		}
		fmt.Println("  " + rep.Summary())
		parsed = append(parsed, loaded{v: v, text: text, rep: rep})
	}
	if len(parsed) == 0 {
		return errors.New("nothing to load")
	}

	conn, err := db.Open(*dsn)
	if err != nil {
		return err
	}
	defer conn.Close()

	for _, l := range parsed {
		if err := loadVersion(context.Background(), conn, l.v, l.text, l.rep); err != nil {
			return fmt.Errorf("%s: %w", l.v.ID, err)
		}
		fmt.Printf("  loaded  %-9s %d verses\n", l.v.ID, l.rep.Verses)
	}
	return nil
}

func parseAndValidate(path string, v bible.Version) (*bible.Report, *bible.ParsedTranslation, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	t, err := bible.Parse(f, v.Format, v.ID)
	if err != nil {
		return nil, nil, err
	}
	return bible.Validate(t, v), t, nil
}

// loadVersion imports a verified translation once. A second run with the same
// digest is idempotent; a different source or partial content is refused. A
// revised edition must get a new translation ID, never rewrite cited verses.
func loadVersion(ctx context.Context, conn *db.DB, v bible.Version, t *bible.ParsedTranslation, rep *bible.Report) error {
	if t == nil || rep == nil || !rep.OK || t.ID != v.ID || rep.Verses != t.VerseCount() || v.SHA256 == "" {
		return fmt.Errorf("translation %s has not passed source verification", v.ID)
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	// Rollback after a successful Commit is a no-op, so a deferred rollback is
	// safe and covers every early return below.
	defer func() { _ = tx.Rollback() }()

	// The version row lock serializes concurrent imports of this ID. Checking
	// before the UPSERT matters: otherwise an existing source hash could be
	// overwritten before we notice a different, previously cited edition.
	var provider, digest string
	var existingVerses int
	err = tx.QueryRowContext(ctx, `SELECT provider,COALESCE(content_hash,''),
		(SELECT count(*) FROM bible_verses WHERE version_id=?)
		FROM bible_versions WHERE id=? FOR UPDATE`, v.ID, v.ID).Scan(&provider, &digest, &existingVerses)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("inspect existing translation: %w", err)
	}
	if err == nil && provider != v.Provider {
		return fmt.Errorf("translation %s belongs to provider %s, not %s", v.ID, provider, v.Provider)
	}
	alreadyLoaded := existingVerses > 0
	if alreadyLoaded && (digest != v.SHA256 || existingVerses != rep.Verses) {
		return fmt.Errorf("translation %s already holds %d verses from source %s; revisions require a new translation ID", v.ID, existingVerses, digest)
	}

	{
		now := time.Now().UTC().Format(time.RFC3339)
		defaultFlag := 0
		if v.Default {
			defaultFlag = 1 // bible_versions.is_default is a legacy INTEGER, not a BOOLEAN.
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO bible_versions
			  (id,name,abbrev,language,language_name,format,coverage,year,licence,licence_url,
			   licence_note,attribution,blob_url,sha256,bytes,sort_order,is_default,book_count,
			   chapter_count,verse_count,imported_at,created_at,updated_at,provider,provider_translation_id,
			   locale,country,dialect,publisher,description,copyright_text,public_domain,commercial_use,
			   redistribution_allowed,modification_allowed,audio_allowed,offline_allowed,copy_allowed,
			   share_allowed,search_index_allowed,api_exposure_allowed,attribution_required,
			   attribution_text,source_url,source_version,import_version,content_hash,status)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT (id) DO UPDATE SET
			  name=EXCLUDED.name, abbrev=EXCLUDED.abbrev, language=EXCLUDED.language,
			  language_name=EXCLUDED.language_name, format=EXCLUDED.format,
			  coverage=EXCLUDED.coverage, year=EXCLUDED.year, licence=EXCLUDED.licence,
			  licence_url=EXCLUDED.licence_url, licence_note=EXCLUDED.licence_note,
			  attribution=EXCLUDED.attribution, blob_url=EXCLUDED.blob_url,
			  sha256=EXCLUDED.sha256, bytes=EXCLUDED.bytes,
			  sort_order=EXCLUDED.sort_order, is_default=EXCLUDED.is_default,
			  book_count=EXCLUDED.book_count, chapter_count=EXCLUDED.chapter_count,
			  verse_count=EXCLUDED.verse_count, imported_at=EXCLUDED.imported_at,
			  source_version=EXCLUDED.source_version, import_version=EXCLUDED.import_version,
			  content_hash=EXCLUDED.content_hash, updated_at=EXCLUDED.updated_at`,
			v.ID, v.Name, v.Abbrev, v.Language, v.LanguageName, v.Format, rep.Coverage,
			v.Year, v.Licence, v.LicenceURL, v.LicenceNote, v.Attribution, v.BlobURL(),
			v.SHA256, v.Bytes, v.SortOrder, defaultFlag, rep.Books, rep.Chapters, rep.Verses,
			now, now, now, v.Provider, v.ProviderTranslationID, v.Locale, v.Country, v.Dialect,
			v.Publisher, v.Description, v.Copyright, v.Rights.PublicDomain, v.Rights.CommercialUse,
			v.Rights.RedistributionAllowed, v.Rights.ModificationAllowed, v.Rights.AudioAllowed,
			v.Rights.OfflineAllowed, v.Rights.CopyAllowed, v.Rights.ShareAllowed,
			v.Rights.SearchIndexAllowed, v.Rights.APIExposureAllowed, v.AttributionRequired,
			v.Attribution, v.BlobURL(), v.SourceVersion, "open-bibles-v1", v.SHA256, v.Status,
		); err != nil {
			return fmt.Errorf("upsert version: %w", err)
		}

		if !alreadyLoaded {
			stmt, err := tx.Tx.PrepareContext(ctx, copyVersesSQL)
			if err != nil {
				return fmt.Errorf("prepare COPY: %w", err)
			}
			rows := 0
			for order, b := range bible.Canon {
				book := t.Book(b.ID)
				if book == nil {
					continue
				}
				for _, ch := range book.Chapters {
					for _, vr := range ch.Verses {
						if _, err := stmt.ExecContext(ctx, v.ID, b.ID, b.Name, b.Testament, order+1, ch.Number, vr.Number, vr.Text, now, now); err != nil {
							_ = stmt.Close()
							return fmt.Errorf("stage %s %d:%d: %w", b.ID, ch.Number, vr.Number, err)
						}
						rows++
					}
				}
			}
			if _, err := stmt.ExecContext(ctx); err != nil { // flush
				_ = stmt.Close()
				return fmt.Errorf("flush COPY: %w", err)
			}
			if err := stmt.Close(); err != nil {
				return fmt.Errorf("close COPY: %w", err)
			}
			if rows != rep.Verses {
				return fmt.Errorf("wrote %d verses, verified %d", rows, rep.Verses)
			}
		}
	}

	// A legacy import may predate the book mapping; backfill it without
	// modifying existing entries or the verified source verses.
	for order, canonical := range bible.Canon {
		book := t.Book(canonical.ID)
		if book == nil {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO bible_translation_books
			(version_id,book_id,display_name,testament,canonical_order,chapter_count,has_text)
			VALUES(?,?,?,?,?,?,?) ON CONFLICT(version_id,book_id) DO NOTHING`, v.ID, canonical.ID, canonical.Name, canonical.Testament,
			order+1, len(book.Chapters), len(book.Chapters) > 0); err != nil {
			return fmt.Errorf("map translated book %s: %w", canonical.ID, err)
		}
	}
	return tx.Commit()
}

// copyVersesSQL names every NOT NULL column of bible_verses. COPY does not run
// defaults, so a column left out of this list would abort the import with a
// null violation on the last of three million rows rather than the first.
var copyVersesSQL = `COPY bible_verses (version_id,book_id,book_name,testament,canonical_order,chapter,verse,text,created_at,updated_at) FROM STDIN`
