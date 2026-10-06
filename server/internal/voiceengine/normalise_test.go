package voiceengine

import (
	"context"
	"strings"
	"testing"

	"github.com/Teamthy/i-confess/internal/voicegov"
)

// TestNormaliseTable is the oracle the audit found missing (VE-004): the golden
// set carried no `expect` field, so nothing in the repository said what "John
// 3:16" should become. Every case here is an exact expansion, not a
// "contains".
func TestNormaliseTable(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		want     string
		dayFirst bool
	}{
		// --- the audit's cases ---
		{"bible reference", "John 3:16", "John chapter three verse sixteen", true},
		{"bible reference no space", "Romans 8:28", "Romans chapter eight verse twenty-eight", true},
		{"psalm with chapter only", "Read Psalm 23", "Read Psalm twenty-three", true},
		{"grouped number", "about 1,250 people", "about one thousand two hundred and fifty people", true},
		{"currency", "a gift of ₦5,000", "a gift of five thousand naira", true},
		{"day-first date", "on the 14th of March, 2026", "on the fourteenth of March twenty twenty-six", true},
		{"time", "at 7:30 in the evening", "at seven thirty in the evening", true},

		// --- books, aliases, numbered books ---
		{"numbered book", "1 Corinthians 13:4-7", "First Corinthians chapter thirteen verse four to seven", true},
		{"abbreviation", "Gen 1:1", "Genesis chapter one verse one", true},
		{"abbreviation 2", "1 Cor 13:4", "First Corinthians chapter thirteen verse four", true},
		{"song of solomon", "Song of Solomon 2:1", "Song of Solomon chapter two verse one", true},
		{"psalms alias", "Ps 23:1", "Psalm chapter twenty-three verse one", true},
		{"verse range", "Psalm 23 verses 1 to 6", "Psalm twenty-three verses one to six", true},
		{"verse range hyphen", "verses 3-4", "verses three to four", true},
		{"chapter range", "chapters 2 to 4", "chapters two to four", true},

		// --- numbers and money ---
		{"thousands", "10,000 worshippers", "ten thousand worshippers", true},
		{"hundreds of thousands", "120,000 naira", "one hundred and twenty thousand naira", true},
		{"dollar amount", "It cost $1,200.", "It cost one thousand two hundred dollars.", true},
		{"one dollar", "It cost $1.", "It cost one dollar.", true},
		{"pounds", "£250", "two hundred and fifty pounds", true},
		{"ordinal", "the 21st century", "the twenty-first century", true},
		{"ordinal hundreds", "the 100th time", "the one hundredth time", true},
		{"bare digits untouched", "3 people came", "3 people came", true},

		// --- dates and times ---
		{"month-first day-first locale", "March 14, 2026", "the fourteenth of March twenty twenty-six", true},
		{"month-first us locale", "March 14, 2026", "March fourteenth twenty twenty-six", false},
		{"day-first us locale", "the 14th of March, 2026", "March fourteenth twenty twenty-six", false},
		{"date without year", "on 5 June", "on the fifth of June", true},
		{"abbreviated month", "12 Sept 2025", "the twelfth of September twenty twenty-five", true},
		{"year two thousands", "in 2005", "in 2005", true}, // no date context: left alone
		{"year in a date", "1 January 2005", "the first of January two thousand and five", true},
		{"year 2000", "1 January 2000", "the first of January two thousand", true},
		{"year 1905", "1 January 1905", "the first of January nineteen oh five", true},
		{"o'clock", "at 7:00", "at seven o'clock", true},
		{"minutes under ten", "at 7:05", "at seven oh five", true},
		{"twenty-four hour", "at 19:45", "at nineteen forty-five", true},
		{"midnight boundary", "at 0:30", "at twelve thirty", true},
		{"not a time", "at 25:99", "at 25:99", true},

		// --- urls ---
		{"url", "visit https://iconfess.app/help", "visit iconfess dot app slash help", true},
		// The host is left intact: reading "www" aloud is the engine's job,
		// and dropping it would silently change the address.
		{"www url", "visit www.iconfess.app", "visit www dot iconfess dot app", true},

		// --- text that must not be touched ---
		{"plain prose", "Father, thank you for this new morning.", "Father, thank you for this new morning.", true},
		{"no digits", "Let us pray for the peace of Jerusalem.", "Let us pray for the peace of Jerusalem.", true},
		{"not a book", "Mark this day", "Mark this day", true},
		{"empty", "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Normalise(tc.in, tc.dayFirst); got != tc.want {
				t.Errorf("Normalise(%q, dayFirst=%v) =\n  %q\nwant\n  %q", tc.in, tc.dayFirst, got, tc.want)
			}
		})
	}
}

// The golden set's own hard case, end to end. If this sentence is read
// correctly, the feature works for real scripts.
func TestNormaliseGoldenSetReferenceLine(t *testing.T) {
	in := "Read Psalm 23 verses 1 to 6, then John 3:16, and Romans 8:28. " +
		"The meeting is on the 14th of March, 2026, at 7:30 in the evening, " +
		"and about 1,250 people are expected."
	// "one thousand two hundred and fifty" keeps its "and": that is how the
	// number is read in Nigerian and British English, and the exact reading is
	// the point of the test.
	want := "Read Psalm twenty-three verses one to six, then John chapter three verse sixteen, " +
		"and Romans chapter eight verse twenty-eight. " +
		"The meeting is on the fourteenth of March twenty twenty-six, at seven thirty in the evening, " +
		"and about one thousand two hundred and fifty people are expected."

	if got := Normalise(in, true); got != want {
		t.Errorf("golden line =\n  %q\nwant\n  %q", got, want)
	}
}

// Normalisation must be idempotent: plan() resolves, re-renders and streams the
// same segments, and a rule that fired twice would keep rewriting its own
// output.
func TestNormaliseIsIdempotent(t *testing.T) {
	for _, in := range []string{
		"John 3:16 and 1,250 people on the 14th of March, 2026 at 7:30.",
		"Read Psalm 23 verses 1 to 6, then Romans 8:28.",
		"a gift of ₦5,000 to the 21st person",
	} {
		once := Normalise(in, true)
		if twice := Normalise(once, true); twice != once {
			t.Errorf("not idempotent:\n in: %q\none: %q\ntwo: %q", in, once, twice)
		}
	}
}

// Non-English text is never normalised: an English number word inside a
// non-English sentence is worse than a digit.
func TestNormaliseSegmentsSkipsOtherLanguages(t *testing.T) {
	segs := []Segment{{Text: "John 3:16"}}
	got := NormaliseSegments(segs, "yo", "yo-NG")
	if got[0].Text != "John 3:16" {
		t.Fatalf("Yoruba text was normalised: %q", got[0].Text)
	}
	// English, however it is labelled, is normalised.
	for _, pair := range [][2]string{{"en", "en-NG"}, {"", "en-NG-PIDGIN"}, {"en-NG", ""}, {"", ""}} {
		got = NormaliseSegments([]Segment{{Text: "John 3:16"}}, pair[0], pair[1])
		if got[0].Text != "John chapter three verse sixteen" {
			t.Errorf("language=%q locale=%q was not normalised: %q", pair[0], pair[1], got[0].Text)
		}
	}
}

// An explicit {say} is an author's instruction about a specific word. It wins,
// and it is not rewritten.
func TestNormaliseSegmentsLeavesExplicitSayAlone(t *testing.T) {
	segs := []Segment{
		{Text: "Ogbomosho", Say: "Oh-gboh-moh-shoh"},
		{PauseMS: 400},
		{Text: "John 3:16"},
	}
	got := NormaliseSegments(segs, "en", "en-NG")
	if got[0].Say != "Oh-gboh-moh-shoh" || got[0].Text != "Ogbomosho" {
		t.Fatalf("explicit {say} was rewritten: %+v", got[0])
	}
	if got[1].PauseMS != 400 {
		t.Fatalf("pause marker was altered: %+v", got[1])
	}
	if got[2].Text != "John chapter three verse sixteen" {
		t.Fatalf("plain segment was not normalised: %+v", got[2])
	}
}

// The stage is wired into the orchestrator, not merely available: a plan built
// through the real path carries what the engine will actually be asked to say,
// and the cache key changes when that reading changes.
func TestPlanCarriesNormalisedTextAndHashesIt(t *testing.T) {
	o, _, _, models, refs := setup()
	req := req()
	req.Markup = "Read John 3:16 to about 1,250 people."
	plan, err := o.Resolve(context.Background(), approvedGrant(), models, refs, req)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	chunks := plan.Request.Chunks
	var spoken strings.Builder
	for _, c := range chunks {
		spoken.WriteString(c.Text)
	}
	if !strings.Contains(spoken.String(), "John chapter three verse sixteen") {
		t.Fatalf("the plan still says the colon: %q", spoken.String())
	}
	if !strings.Contains(spoken.String(), "one thousand two hundred and fifty") {
		t.Fatalf("the grouped number was not read aloud: %q", spoken.String())
	}

	// The hash covers the normalised text and the rule-set version, so a rule
	// change cannot serve audio recorded under a different reading.
	h := HashInput{VoiceID: "v", Text: "Read John 3:16.", TextNormalised: "Read John chapter three verse sixteen.", NormaliseVersion: NormaliseVersion}
	a := ContentHash(h)
	h.NormaliseVersion = "n2"
	if ContentHash(h) == a {
		t.Fatal("the rule-set version does not change the content hash")
	}
	h.NormaliseVersion = NormaliseVersion
	h.TextNormalised = "Read John chapter three verse xvi."
	if ContentHash(h) == a {
		t.Fatal("the normalised reading does not change the content hash")
	}
}

// A plan resolved twice is the same plan; normalisation must not make the
// content hash depend on anything but the text.
func TestNormalisedPlanIsDeterministic(t *testing.T) {
	o, _, _, models, refs := setup()
	req := req()
	req.Markup = "On the 14th of March, 2026, read 1 Corinthians 13:4-7 to 1,250 people."
	p1, err := o.Resolve(context.Background(), approvedGrant(), models, refs, req)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := o.Resolve(context.Background(), approvedGrant(), models, refs, req)
	if err != nil {
		t.Fatal(err)
	}
	if p1.ContentHash != p2.ContentHash {
		t.Fatal("normalisation made the content hash non-deterministic")
	}
}

// The safety stage and the reading stage must not disagree about what is being
// said. ValidateScript still sees the authored text, but the reference the
// plan produces must not reintroduce a phrase the safety check would refuse.
func TestNormaliseDoesNotIntroduceProhibitedPhrasing(t *testing.T) {
	grant := approvedGrant()
	grant.Capabilities[voicegov.CanUseInConfessions] = true
	segs := NormaliseSegments([]Segment{{Text: "John 3:16"}}, "en", "en-NG")
	if err := ValidateScript(PlainText(segs), true); err != nil {
		t.Fatalf("normalised text failed the safety check: %v", err)
	}
}
