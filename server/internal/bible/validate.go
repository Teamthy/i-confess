package bible

import (
	"fmt"
	"sort"
	"strings"
)

// Verification.
//
// "Fully verified" is only meaningful if it is a computation, so this file
// turns it into one. A version passes when every requirement below holds, and
// the report says exactly which one failed when it does not. The importer
// refuses to load a version that fails validation, and the same function runs
// in CI against the committed fixture, so the check that guards production is
// the check that guards a pull request.
//
// The checks fall into two groups, and the distinction matters:
//
//  1. Structural requirements. Every book the version claims to cover is
//     present, every chapter the canon has is present unless the registry
//     declares it missing, verse numbers ascend without duplicates, and no
//     verse is empty. These are absolute; a translation that disagrees is
//     broken, not different.
//  2. Verse-count variance. Translations legitimately differ by a verse here
//     and there (the KJV has 31,102 verses, the BSB 31,085) because of
//     versification choices made centuries ago. Variance is measured against
//     the reference distribution, reported, and allowed within a tolerance -
//     but variance above the tolerance is a parse failure wearing a
//     versification costume, which is why the count is bounded rather than
//     ignored.
//
// Declared omissions cut both ways. A version that is missing a book must
// declare it, and a version that declares one missing must actually be missing
// it: both directions are errors here, so the registry cannot drift away from
// the files it describes and start describing a Bible nobody has.

// Severity values for a finding.
const (
	SeverityError   = "error"
	SeverityWarning = "warning"
)

// Finding is one verification observation.
type Finding struct {
	Severity string `json:"severity"`
	Subject  string `json:"subject"`
	Detail   string `json:"detail"`
}

// Report is the verification result for one version.
type Report struct {
	VersionID     string `json:"version_id"`
	Name          string `json:"name"`
	Language      string `json:"language"`
	LanguageName  string `json:"language_name"`
	Licence       string `json:"licence"`
	Coverage      string `json:"coverage"`
	CoverageLabel string `json:"coverage_label"`
	Books         int    `json:"books"`
	Chapters      int    `json:"chapters"`
	Verses        int    `json:"verses"`
	Milestones    int    `json:"milestones"`
	Containers    int    `json:"containers"`
	// DroppedVerses counts verse elements pruned because they carried no text,
	// with a few examples. These are placeholders the source keeps for verses
	// it does not have, not text the parser lost.
	DroppedVerses  int      `json:"dropped_verses,omitempty"`
	DroppedSamples []string `json:"dropped_samples,omitempty"`
	// MissingBooks are books the version's declared coverage requires but the
	// file does not contain.
	MissingBooks []string `json:"missing_books,omitempty"`
	// UnexpectedBooks are canonical books present that the declared coverage
	// does not include. Not an error - a New Testament that also has Psalms is
	// a better file, not a worse one - but the coverage label has to be right.
	UnexpectedBooks []string `json:"unexpected_books,omitempty"`
	// NonCanonical are divisions in the file that are not in the canon:
	// front matter, apocrypha, glossaries.
	NonCanonical []string `json:"non_canonical_divisions,omitempty"`
	// DeclaredOmissions are the registry's omitted books and chapters that the
	// file was in fact missing. They are reported so the reader can be told.
	DeclaredOmissions []string `json:"declared_omissions,omitempty"`
	// VerseVariance lists chapters whose verse count differs from the
	// reference distribution, largest difference first.
	VerseVariance []VerseVariance `json:"verse_variance,omitempty"`
	// EmptyVerses are verses whose text came out empty. The parser prunes
	// them, so anything listed here is a bug rather than a source quirk.
	EmptyVerses []string `json:"empty_verses,omitempty"`
	// Findings is the full list, in the order discovered.
	Findings []Finding `json:"findings"`

	VarianceTotal int  `json:"variance_total"`
	OK            bool `json:"ok"`
}

// VerseVariance is one chapter whose verse count differs from the reference.
type VerseVariance struct {
	Book     string `json:"book"`
	Name     string `json:"name"`
	Chapter  int    `json:"chapter"`
	Got      int    `json:"got"`
	Expected int    `json:"expected"`
}

func (r *Report) errorf(subject, format string, args ...any) {
	r.Findings = append(r.Findings, Finding{
		Severity: SeverityError, Subject: subject,
		Detail: fmt.Sprintf(format, args...),
	})
}

func (r *Report) warnf(subject, format string, args ...any) {
	r.Findings = append(r.Findings, Finding{
		Severity: SeverityWarning, Subject: subject,
		Detail: fmt.Sprintf(format, args...),
	})
}

// maxVariancePercent bounds how far a version's verse count may differ from
// the reference distribution before the difference is treated as a parse
// failure rather than versification. The shipped versions land at or below
// 0.41%; the sources that were rejected for structural mismatch land above 1%
// (the Clementine Vulgate at 5.5%), so this sits in the gap between "different
// edition" and "broken import".
const maxVariancePercent = 1.0

// Validate checks one parsed translation against its registry entry and the
// canon.
func Validate(t *ParsedTranslation, v Version) *Report {
	r := &Report{
		VersionID: v.ID, Name: v.Name, Language: v.Language, LanguageName: v.LanguageName,
		Licence: v.Licence, Coverage: v.Coverage, CoverageLabel: v.CoverageLabel(),
		NonCanonical:   t.Skipped,
		Milestones:     t.Milestones,
		Containers:     t.Containers,
		DroppedVerses:  t.EmptyVerses,
		DroppedSamples: t.EmptySamples,
	}

	required := requiredBooks(v)
	got := map[string]*BookText{}
	for i := range t.Books {
		got[t.Books[i].ID] = &t.Books[i]
	}

	r.Books = len(t.Books)
	for _, id := range required {
		if _, ok := got[id]; !ok {
			r.MissingBooks = append(r.MissingBooks, id)
		}
	}
	needed := map[string]bool{}
	for _, id := range required {
		needed[id] = true
	}
	for id := range got {
		if !needed[id] {
			r.UnexpectedBooks = append(r.UnexpectedBooks, id)
		}
	}
	sort.Strings(r.UnexpectedBooks)

	if len(r.MissingBooks) > 0 {
		r.errorf("coverage", "%d required books missing: %s",
			len(r.MissingBooks), strings.Join(bookNames(r.MissingBooks), ", "))
	}
	// A declared omission that the file actually contains means the registry
	// is describing a different file than the one on disk.
	for _, id := range v.OmittedBooks {
		if _, present := got[id]; present {
			r.errorf("declaration", "%s is declared omitted but the file contains it", id)
		}
	}
	var declaredChapters []string
	for _, ref := range v.OmittedChapters {
		book, chapter, ok := splitChapterRef(ref)
		if !ok {
			r.errorf("declaration", "unreadable omitted chapter %q", ref)
			continue
		}
		if b := got[book]; b != nil {
			i := sort.Search(len(b.Chapters), func(i int) bool { return b.Chapters[i].Number >= chapter })
			if i < len(b.Chapters) && b.Chapters[i].Number == chapter {
				r.errorf("declaration", "%s is declared omitted but the file contains it", ref)
			}
		}
		declaredChapters = append(declaredChapters, ref)
	}

	// Structural checks, in canonical order so the report reads the way a
	// Bible does.
	for i := range t.Books {
		b := &t.Books[i]
		canonBook, ok := BookByID(b.ID)
		if !ok {
			r.errorf(b.ID, "not a canonical book")
			continue
		}
		if len(b.Chapters) != canonBook.Chapters() {
			missing := missingChapters(b, canonBook)
			if len(missing) > 0 && allDeclared(missing, b.ID, declaredChapters) {
				// Declared and verified: the reader is told the chapter is
				// absent rather than shown a gap with no explanation.
				r.DeclaredOmissions = append(r.DeclaredOmissions, fmt.Sprintf("%s (%d of %d chapters)",
					canonBook.Name, canonBook.Chapters()-len(missing), canonBook.Chapters()))
				for _, n := range missing {
					r.DeclaredOmissions = append(r.DeclaredOmissions, fmt.Sprintf("%s.%d", b.ID, n))
				}
				r.warnf(b.ID, "%s: %d chapter(s) absent from the source: %s",
					canonBook.Name, len(missing), joinInts(missing))
			} else {
				r.errorf(b.ID, "%s has %d chapters, canon has %d (missing %s)",
					canonBook.Name, len(b.Chapters), canonBook.Chapters(), joinInts(missing))
			}
		}
		var lastChapter int
		for j := range b.Chapters {
			ch := &b.Chapters[j]
			r.Chapters++
			if ch.Number <= lastChapter {
				r.errorf(fmt.Sprintf("%s.%d", b.ID, ch.Number),
					"chapter numbers out of order (after %d)", lastChapter)
			}
			lastChapter = ch.Number
			if ch.Number < 1 || ch.Number > canonBook.Chapters() {
				r.errorf(fmt.Sprintf("%s.%d", b.ID, ch.Number),
					"%s has no chapter %d", canonBook.Name, ch.Number)
				continue
			}

			expected := canonBook.Verses[ch.Number-1]
			if len(ch.Verses) != expected {
				r.VerseVariance = append(r.VerseVariance, VerseVariance{
					Book: b.ID, Name: canonBook.Name, Chapter: ch.Number,
					Got: len(ch.Verses), Expected: expected,
				})
				r.VarianceTotal += abs(len(ch.Verses) - expected)
			}

			seen := map[int]bool{}
			var lastVerse int
			for k := range ch.Verses {
				vr := &ch.Verses[k]
				r.Verses++
				switch {
				case vr.Number < 1:
					r.errorf(fmt.Sprintf("%s.%d", b.ID, ch.Number), "verse number %d is not positive", vr.Number)
				case vr.Number > expected:
					// Some editions carry legitimate extra verses, so this is a
					// warning that names the chapter, not a failure.
					r.warnf(fmt.Sprintf("%s.%d.%d", b.ID, ch.Number, vr.Number),
						"beyond the reference distribution (%d verses)", expected)
				}
				if seen[vr.Number] {
					r.errorf(fmt.Sprintf("%s.%d.%d", b.ID, ch.Number, vr.Number), "duplicate verse")
				}
				seen[vr.Number] = true
				if vr.Number <= lastVerse && lastVerse != 0 {
					r.errorf(fmt.Sprintf("%s.%d", b.ID, ch.Number), "verse numbers out of order")
				}
				lastVerse = vr.Number
				if strings.TrimSpace(vr.Text) == "" {
					r.EmptyVerses = append(r.EmptyVerses, fmt.Sprintf("%s.%d.%d", b.ID, ch.Number, vr.Number))
				}
			}
		}
	}

	if n := len(r.EmptyVerses); n > 0 {
		shown := r.EmptyVerses
		if len(shown) > 5 {
			shown = shown[:5]
		}
		r.errorf("text", "%d verses have no text after pruning (e.g. %s)", n, strings.Join(shown, ", "))
	}
	if r.DroppedVerses > 0 {
		r.warnf("text", "%d empty verse placeholders pruned (e.g. %s)",
			r.DroppedVerses, strings.Join(r.DroppedSamples, ", "))
	}

	// Variance against the reference distribution, as a share of the
	// version's own verse count so a New Testament is judged on its own size.
	if r.Verses > 0 {
		pct := float64(r.VarianceTotal) / float64(r.Verses) * 100
		switch {
		case pct > maxVariancePercent:
			r.errorf("variance", "%.2f%% of verses differ from the reference distribution (%d verses), above the %.2f%% limit",
				pct, r.VarianceTotal, maxVariancePercent)
		case r.VarianceTotal > 0:
			r.warnf("variance", "%d verses differ from the reference distribution (%.2f%%)",
				r.VarianceTotal, pct)
		}
	} else {
		r.errorf("text", "no verses parsed")
	}

	sort.Slice(r.VerseVariance, func(i, j int) bool {
		a, b := abs(r.VerseVariance[i].Got-r.VerseVariance[i].Expected), abs(r.VerseVariance[j].Got-r.VerseVariance[j].Expected)
		if a != b {
			return a > b
		}
		return r.VerseVariance[i].Chapter < r.VerseVariance[j].Chapter
	})

	r.OK = true
	for _, f := range r.Findings {
		if f.Severity == SeverityError {
			r.OK = false
			break
		}
	}
	return r
}

// requiredBooks is the book set a version promises: its coverage scope, minus
// everything the registry declares omitted.
func requiredBooks(v Version) []string {
	scope := Canon
	if v.Coverage == CoverageNewTestament {
		scope = nil
		for _, b := range Canon {
			if b.Testament == New {
				scope = append(scope, b)
			}
		}
	}
	omitted := map[string]bool{}
	for _, id := range v.OmittedBooks {
		omitted[id] = true
	}
	out := make([]string, 0, len(scope))
	for _, b := range scope {
		if !omitted[b.ID] {
			out = append(out, b.ID)
		}
	}
	return out
}

// missingChapters names the canonical chapters a parsed book does not have.
func missingChapters(b *BookText, canonBook *Book) []int {
	have := map[int]bool{}
	for _, ch := range b.Chapters {
		have[ch.Number] = true
	}
	var missing []int
	for n := 1; n <= canonBook.Chapters(); n++ {
		if !have[n] {
			missing = append(missing, n)
		}
	}
	return missing
}

// allDeclared reports whether every missing chapter of a book appears in the
// registry's omission list.
func allDeclared(missing []int, book string, declared []string) bool {
	set := map[string]bool{}
	for _, ref := range declared {
		set[ref] = true
	}
	for _, n := range missing {
		if !set[fmt.Sprintf("%s.%d", book, n)] {
			return false
		}
	}
	return true
}

// splitChapterRef parses "Matt.23" into its parts.
func splitChapterRef(ref string) (string, int, bool) {
	i := strings.LastIndex(ref, ".")
	if i <= 0 {
		return "", 0, false
	}
	var n int
	if _, err := fmt.Sscanf(ref[i+1:], "%d", &n); err != nil {
		return "", 0, false
	}
	return ref[:i], n, true
}

func bookNames(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if b, ok := BookByID(id); ok {
			out = append(out, b.Name)
			continue
		}
		out = append(out, id)
	}
	return out
}

func joinInts(ns []int) string {
	parts := make([]string, 0, len(ns))
	for _, n := range ns {
		parts = append(parts, fmt.Sprint(n))
	}
	return strings.Join(parts, ", ")
}

// Summary renders a one-line result for CLI output.
func (r *Report) Summary() string {
	status := "OK"
	if !r.OK {
		status = "FAILED"
	}
	return fmt.Sprintf("%-9s %-10s %-9s %-13s %3d books %5d chapters %6d verses  [%d errors, %d warnings]",
		r.VersionID, r.LanguageName, status, r.Coverage,
		r.Books, r.Chapters, r.Verses, r.errorCount(), len(r.Findings)-r.errorCount())
}

func (r *Report) errorCount() int {
	n := 0
	for _, f := range r.Findings {
		if f.Severity == SeverityError {
			n++
		}
	}
	return n
}

// Detail renders the findings, errors first, for logs and CI output.
func (r *Report) Detail() string {
	if len(r.Findings) == 0 {
		return ""
	}
	ordered := make([]Finding, 0, len(r.Findings))
	for _, f := range r.Findings {
		if f.Severity == SeverityError {
			ordered = append(ordered, f)
		}
	}
	for _, f := range r.Findings {
		if f.Severity != SeverityError {
			ordered = append(ordered, f)
		}
	}
	var b strings.Builder
	for _, f := range ordered {
		fmt.Fprintf(&b, "  [%s] %s: %s\n", strings.ToUpper(f.Severity), f.Subject, f.Detail)
	}
	return b.String()
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
