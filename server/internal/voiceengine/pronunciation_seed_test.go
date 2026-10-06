package voiceengine

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The pronunciation dictionary shipped empty (audit VE-005): the engine was
// complete and the content did not exist. The seed lives in a migration, so
// this test reads the migration the deployment will actually apply rather than
// a copy - it fails if the file is truncated, if a respelling is missing, if a
// term is duplicated, or if the golden set's own vocabulary is not covered.

const seedPath = "../db/migrations/0031_pronunciation_seed.sql"

var seedRowRe = regexp.MustCompile(`\('seed-pron-(\d{4})', '([^']*)', '([^']*)', '([^']*)',`)

type seedRow struct {
	term       string
	locale     string
	respelling string
}

func loadSeed(t *testing.T) []seedRow {
	t.Helper()
	b, err := os.ReadFile(seedPath)
	if err != nil {
		t.Fatalf("the pronunciation seed is not where the deployment expects it: %v", err)
	}
	matches := seedRowRe.FindAllStringSubmatch(string(b), -1)
	rows := make([]seedRow, 0, len(matches))
	for _, m := range matches {
		rows = append(rows, seedRow{term: m[2], locale: m[3], respelling: m[4]})
	}
	if len(rows) == 0 {
		t.Fatal("no seed rows parsed; the migration format changed without this test changing")
	}
	return rows
}

func TestPronunciationSeedIsCompleteAndUsable(t *testing.T) {
	rows := loadSeed(t)
	if len(rows) < 140 {
		t.Fatalf("%d entries; the seed is meant to cover biblical names, Nigerian names and places, "+
			"translation acronyms and church terms - a truncated file must not pass", len(rows))
	}

	seen := map[string]bool{}
	for _, r := range rows {
		if r.locale != "en-NG" {
			t.Errorf("%q: locale = %q, want an explicit en-NG row (Pidgin is copied separately)", r.term, r.locale)
		}
		if strings.TrimSpace(r.respelling) == "" {
			t.Errorf("%q has no respelling; an engine without phoneme support would say nothing", r.term)
		}
		for _, ch := range r.respelling {
			if ch >= '0' && ch <= '9' {
				t.Errorf("%q: respelling %q contains a digit", r.term, r.respelling)
			}
		}
		// Render() strips hyphens and lowercases for engines without phoneme
		// support, so the spoken form must contain nothing else to read aloud.
		if spoken := respellingForSpeech(r.respelling); strings.Trim(spoken, "abcdefghijklmnopqrstuvwxyz ") != "" {
			t.Errorf("%q: %q becomes %q once syllables are separated, which an engine would read literally",
				r.term, r.respelling, spoken)
		}
		key := strings.ToLower(r.term) + "\x00" + r.locale
		if seen[key] {
			t.Errorf("duplicate entry for %q under %s; the UNIQUE (term, locale) constraint would reject it",
				r.term, r.locale)
		}
		seen[key] = true
	}
}

// The ten names and places the golden set already exercises, with the reading
// each one carries. These are the exact strings the engine is handed, so they
// are asserted rather than sampled.
func TestPronunciationSeedCoversTheGoldenSetVocabulary(t *testing.T) {
	want := map[string]string{
		"Chukwuemeka": "choo-kwoo-eh-MEH-kah",
		"Onitsha":     "oh-NEE-chah",
		"Ile-Ife":     "ee-leh-EE-feh",
		"Oluwaseun":   "oh-loo-wah-SEH-oon",
		"Ngozi":       "en-GOH-zee",
		"Abeokuta":    "ah-beh-oh-KOO-tah",
		"Adaeze":      "ah-DAY-zeh",
		"Babatunde":   "bah-bah-TOON-deh",
		"Oshogbo":     "oh-SHOG-boh",
		"Ogbomosho":   "og-boh-moh-SHOH",
	}
	dict := NewDictionary(seedEntries(loadSeed(t)))
	for term, respelling := range want {
		e, ok := dict.Lookup(term, "en-NG")
		if !ok {
			t.Errorf("%q does not resolve under en-NG", term)
			continue
		}
		if e.Respelling != respelling {
			t.Errorf("%q resolves to %q, want %q", term, e.Respelling, respelling)
		}
	}
}

// en-NG entries must not be inherited by en-NG-PIDGIN: they are distinct
// varieties, and markup.go's isPidgin rule exists so one cannot silently speak
// for the other. The migration duplicates them explicitly instead - which is
// what makes the duplication a decision rather than an accident.
func TestPronunciationSeedDoesNotInheritAcrossVarieties(t *testing.T) {
	rows := loadSeed(t)
	ngOnly := seedEntries(rows)
	if e, ok := NewDictionary(ngOnly).Lookup("Ogbomosho", "en-NG-PIDGIN"); ok {
		t.Fatalf("an en-NG entry (%q) was inherited by en-NG-PIDGIN", e.Respelling)
	}

	// The migration's second statement copies every en-NG row into
	// en-NG-PIDGIN. Simulate it faithfully: the same entries under both
	// locales. Only then does the variety resolve, so the coverage is an
	// explicit decision rather than an accident of the lookup rule.
	both := append(seedEntries(rows), seedEntries(rows)...)
	for i := range both {
		if i >= len(rows) {
			both[i].Locale = "en-NG-PIDGIN"
		}
	}
	if _, ok := NewDictionary(both).Lookup("Ogbomosho", "en-NG-PIDGIN"); !ok {
		t.Fatal("Ogbomosho does not resolve for Pidgin voices even with an explicit row")
	}
}

// seedEntries converts parsed rows into dictionary entries.
func seedEntries(rows []seedRow) []PronunciationEntry {
	entries := make([]PronunciationEntry, 0, len(rows))
	for _, r := range rows {
		entries = append(entries, PronunciationEntry{Term: r.term, Locale: r.locale, Respelling: r.respelling})
	}
	return entries
}

// A seeded term must actually change what is spoken: the dictionary applies the
// respelling to the segment, and Render turns it into the words an engine
// reads.
func TestPronunciationSeedChangesWhatIsSpoken(t *testing.T) {
	segs := NewDictionary(seedEntries(loadSeed(t))).Apply([]Segment{{Text: "Ogbomosho and Chukwuemeka met in Onitsha."}},
		"en-NG", EngineCosyVoice, ProviderCapabilities{})
	var said strings.Builder
	for _, s := range segs {
		if s.Say != "" {
			said.WriteString(respellingForSpeech(s.Say) + " ")
		} else {
			said.WriteString(s.Text + " ")
		}
	}
	for _, want := range []string{"og boh moh SHOH", "choo kwoo eh MEH kah", "oh NEE chah"} {
		if !strings.Contains(strings.ToLower(said.String()), strings.ToLower(want)) {
			t.Errorf("the dictionary did not respell %q: %q", want, said.String())
		}
	}
}
