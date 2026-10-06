package voiceengine

import (
	"regexp"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// Text normalisation (audit VE-004)
//
// Until this existed the platform had no stage between a script and an engine,
// so every engine's own text frontend decided how to read a reference, a date,
// a price or a year - differently per engine, and differently again after a
// fallback. The product's core content is Scripture, and the single most likely
// way for it to sound broken is a minister saying "John three colon sixteen".
//
// The rules below are deliberately narrow. Every one of them fixes a reading
// that is *wrong* rather than merely unusual, and none of them rewrites plain
// prose:
//
//	John 3:16            -> John chapter three verse sixteen
//	Psalm 23 verses 1 to 6 -> Psalm twenty-three verses one to six
//	1,250                -> one thousand two hundred and fifty
//	₦5,000               -> five thousand naira
//	14th of March, 2026  -> the fourteenth of March twenty twenty-six
//	7:30                 -> seven thirty
//	21st                 -> twenty-first
//
// What it does not do is as much a decision as what it does:
//
//   - Bare digits are left alone. Engines read "3 people" correctly; spelling
//     every integer out would lengthen the text, change its prosody and buy
//     nothing. Numbers are expanded where the reading is genuinely in doubt:
//     references, ranges, grouped numbers, money, dates, times, ordinals.
//   - Acronyms are not spaced out. "KJV" is a job for the pronunciation
//     dictionary (which carries a respelling an admin can edit without a
//     deploy), not for a rule that would turn an all-caps heading such as
//     "GRACE" into letter soup.
//   - Non-English text is never normalised. An English number word inside a
//     Yoruba sentence is worse than a digit. The stage runs only for English.
//
// NormaliseVersion names the rule set and is part of the content hash, so
// changing a rule re-renders affected text instead of serving cached audio that
// was produced under the old reading.
// ---------------------------------------------------------------------------

// NormaliseVersion identifies this rule set in the content hash.
const NormaliseVersion = "n1"

// NormaliseSegments rewrites every segment's speakable text. Segments carrying
// an explicit {say ...} override are left exactly as authored - the author has
// already said how the words are to be read - and so are pause markers, which
// carry no text at all.
func NormaliseSegments(segs []Segment, language, locale string) []Segment {
	if !isEnglish(language, locale) {
		return segs
	}
	// Nigerian English says "the fourteenth of March"; American English says
	// "March fourteenth". Everything else follows the platform's default, which
	// is day-first, because that is how its audience speaks.
	dayFirst := !strings.EqualFold(strings.TrimSpace(locale), "en-US") &&
		!strings.EqualFold(strings.TrimSpace(locale), "en_US")
	out := make([]Segment, 0, len(segs))
	for _, s := range segs {
		if s.Text == "" || s.Say != "" {
			out = append(out, s)
			continue
		}
		s.Text = Normalise(s.Text, dayFirst)
		out = append(out, s)
	}
	return out
}

// Normalise applies the rule set to one piece of text. It is a pure function so
// it can be table-tested exhaustively; the order of the passes is load-bearing
// (a reference must be read before the time rule sees "3:16").
func Normalise(text string, dayFirst bool) string {
	if text == "" {
		return text
	}
	text = urlRe.ReplaceAllStringFunc(text, spellURL)
	text = bibleRefRe.ReplaceAllStringFunc(text, expandBibleRef)
	text = verseRangeRe.ReplaceAllStringFunc(text, expandVerseRange)
	text = currencyRe.ReplaceAllStringFunc(text, expandCurrency)
	text = groupedNumberRe.ReplaceAllStringFunc(text, expandGroupedNumber)
	text = dateRe.ReplaceAllStringFunc(text, func(m string) string { return expandDate(m, dayFirst) })
	text = timeRe.ReplaceAllStringFunc(text, expandTime)
	text = ordinalRe.ReplaceAllStringFunc(text, expandOrdinal)
	return text
}

// isEnglish reports whether the reading rules apply. A language tag decides it
// when present; otherwise the locale does. An unset pair is English, which is
// the platform's default and the only case that can reach here unlabelled.
func isEnglish(language, locale string) bool {
	for _, tag := range []string{language, locale} {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		primary := strings.ToLower(strings.SplitN(tag, "-", 2)[0])
		primary = strings.SplitN(primary, "_", 2)[0]
		return primary == "en"
	}
	return true
}

// ---------------------------------------------------------------------------
// Scripture references
// ---------------------------------------------------------------------------

// bibleBooks maps every recognised spelling (lowercased) to the book's name as
// it should be read. Aliases are the abbreviations a Nigerian or KJV reader
// would use; the read-aloud form is always the full name, because "Gen" is not
// a word and "Genesis" is.
var bibleBooks = map[string]string{
	"genesis": "Genesis", "gen": "Genesis",
	"exodus": "Exodus", "exod": "Exodus", "ex": "Exodus",
	"leviticus": "Leviticus", "lev": "Leviticus",
	"numbers": "Numbers", "num": "Numbers",
	"deuteronomy": "Deuteronomy", "deut": "Deuteronomy",
	"joshua": "Joshua", "josh": "Joshua",
	"judges": "Judges", "judg": "Judges",
	"ruth":     "Ruth",
	"1 samuel": "First Samuel", "1 sam": "First Samuel", "1samuel": "First Samuel",
	"2 samuel": "Second Samuel", "2 sam": "Second Samuel", "2samuel": "Second Samuel",
	"1 kings": "First Kings", "1 kgs": "First Kings",
	"2 kings": "Second Kings", "2 kgs": "Second Kings",
	"1 chronicles": "First Chronicles", "1 chron": "First Chronicles", "1 chr": "First Chronicles",
	"2 chronicles": "Second Chronicles", "2 chron": "Second Chronicles", "2 chr": "Second Chronicles",
	"ezra": "Ezra", "nehemiah": "Nehemiah", "neh": "Nehemiah",
	"esther": "Esther", "esth": "Esther",
	"job":   "Job",
	"psalm": "Psalm", "psalms": "Psalm", "ps": "Psalm", "psa": "Psalm",
	"proverbs": "Proverbs", "prov": "Proverbs", "pro": "Proverbs",
	"ecclesiastes": "Ecclesiastes", "eccl": "Ecclesiastes", "ecc": "Ecclesiastes",
	"song of solomon": "Song of Solomon", "song of songs": "Song of Solomon", "song": "Song of Solomon",
	"isaiah": "Isaiah", "isa": "Isaiah",
	"jeremiah": "Jeremiah", "jer": "Jeremiah",
	"lamentations": "Lamentations", "lam": "Lamentations",
	"ezekiel": "Ezekiel", "ezek": "Ezekiel",
	"daniel": "Daniel", "dan": "Daniel",
	"hosea": "Hosea", "hos": "Hosea",
	"joel": "Joel", "amos": "Amos",
	"obadiah": "Obadiah", "obad": "Obadiah",
	"jonah": "Jonah", "micah": "Micah", "mic": "Micah",
	"nahum": "Nahum", "habakkuk": "Habakkuk", "hab": "Habakkuk",
	"zephaniah": "Zephaniah", "zeph": "Zephaniah",
	"haggai": "Haggai", "hag": "Haggai",
	"zechariah": "Zechariah", "zech": "Zechariah",
	"malachi": "Malachi", "mal": "Malachi",
	"matthew": "Matthew", "matt": "Matthew", "mt": "Matthew",
	"mark": "Mark", "mk": "Mark",
	"luke": "Luke", "lk": "Luke",
	"john": "John", "jn": "John",
	"acts":   "Acts",
	"romans": "Romans", "rom": "Romans",
	"1 corinthians": "First Corinthians", "1 cor": "First Corinthians",
	"2 corinthians": "Second Corinthians", "2 cor": "Second Corinthians",
	"galatians": "Galatians", "gal": "Galatians",
	"ephesians": "Ephesians", "eph": "Ephesians",
	"philippians": "Philippians", "phil": "Philippians", "php": "Philippians",
	"colossians": "Colossians", "col": "Colossians",
	"1 thessalonians": "First Thessalonians", "1 thess": "First Thessalonians",
	"2 thessalonians": "Second Thessalonians", "2 thess": "Second Thessalonians",
	"1 timothy": "First Timothy", "1 tim": "First Timothy",
	"2 timothy": "Second Timothy", "2 tim": "Second Timothy",
	"titus": "Titus", "philemon": "Philemon", "phlm": "Philemon",
	"hebrews": "Hebrews", "heb": "Hebrews",
	"james": "James", "jas": "James",
	"1 peter": "First Peter", "1 pet": "First Peter",
	"2 peter": "Second Peter", "2 pet": "Second Peter",
	"1 john": "First John", "2 john": "Second John", "3 john": "Third John",
	"jude":       "Jude",
	"revelation": "Revelation", "rev": "Revelation",
}

// bookAlt is the book-name alternation, longest first so "1 Corinthians" wins
// over "Corinthians" and "Song of Solomon" over "Song".
var bookAlt = func() string {
	names := make([]string, 0, len(bibleBooks))
	for name := range bibleBooks {
		names = append(names, regexp.QuoteMeta(name))
	}
	// Longest first. Sorting by length alone is enough here because the
	// alternation is anchored on word boundaries at both ends.
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && len(names[j]) > len(names[j-1]); j-- {
			names[j], names[j-1] = names[j-1], names[j]
		}
	}
	return strings.Join(names, "|")
}()

var (
	// Book chapter:verse, with an optional verse range. "John 3", "John 3:16"
	// and "Romans 8:28-30" all read as references.
	bibleRefRe = regexp.MustCompile(`(?i)\b(` + bookAlt + `)\.?\s+(\d{1,3})(?::(\d{1,3})(?:\s*[-–]\s*(\d{1,3}))?)?`)
	// "verses 1 to 6", "verses 1-6", "chapters 2 to 4".
	verseRangeRe = regexp.MustCompile(`(?i)\b(verses?|chapters?)\s+(\d{1,3})\s*(?:[-–]|\bto\b)\s*(\d{1,3})\b`)
)

func expandBibleRef(m string) string {
	sub := bibleRefRe.FindStringSubmatch(m)
	if sub == nil {
		return m
	}
	book, ok := bibleBooks[strings.ToLower(sub[1])]
	if !ok {
		return m
	}
	var b strings.Builder
	b.WriteString(book)
	if sub[2] != "" {
		n, err := strconv.Atoi(sub[2])
		if err != nil || !canSpell(n) {
			return m
		}
		// "Read Psalm 23" is spoken as "Psalm twenty-three"; the word
		// "chapter" only earns its place when a verse follows and the
		// distinction matters ("John chapter three verse sixteen").
		if sub[3] == "" {
			b.WriteString(" ")
		} else {
			b.WriteString(" chapter ")
		}
		b.WriteString(cardinalWords(n))
	}
	if sub[3] != "" {
		v, err := strconv.Atoi(sub[3])
		if err != nil || !canSpell(v) {
			return m
		}
		b.WriteString(" verse ")
		b.WriteString(cardinalWords(v))
		if sub[4] != "" {
			last, err := strconv.Atoi(sub[4])
			if err != nil || !canSpell(last) {
				return m
			}
			b.WriteString(" to ")
			b.WriteString(cardinalWords(last))
		}
	}
	return b.String()
}

func expandVerseRange(m string) string {
	sub := verseRangeRe.FindStringSubmatch(m)
	if sub == nil {
		return m
	}
	from, err1 := strconv.Atoi(sub[2])
	to, err2 := strconv.Atoi(sub[3])
	if err1 != nil || err2 != nil || !canSpell(from) || !canSpell(to) {
		return m
	}
	return strings.ToLower(sub[1]) + " " + cardinalWords(from) + " to " + cardinalWords(to)
}

// ---------------------------------------------------------------------------
// Numbers, money, ordinals
// ---------------------------------------------------------------------------

var (
	// A number written with thousands separators: 1,250 / 10,000.
	groupedNumberRe = regexp.MustCompile(`\b\d{1,3}(?:,\d{3})+\b`)
	// A currency symbol and its amount, separated or not.
	currencyRe = regexp.MustCompile(`(₦|\$|£|€)\s*(\d{1,3}(?:,\d{3})+|\d+)`)
	// An ordinal: 1st, 2nd, 3rd, 14th.
	ordinalRe = regexp.MustCompile(`\b(\d{1,3})(st|nd|rd|th)\b`)
)

func expandGroupedNumber(m string) string {
	n, err := strconv.Atoi(strings.ReplaceAll(m, ",", ""))
	if err != nil || !canSpell(n) {
		return m
	}
	return cardinalWords(n)
}

func expandCurrency(m string) string {
	sub := currencyRe.FindStringSubmatch(m)
	if sub == nil {
		return m
	}
	n, err := strconv.Atoi(strings.ReplaceAll(sub[2], ",", ""))
	if err != nil || !canSpell(n) {
		return m
	}
	unit := map[string]string{"₦": "naira", "$": "dollar", "£": "pound", "€": "euro"}[sub[1]]
	// Naira is invariant; the others pluralise.
	if n != 1 && unit != "naira" {
		unit += "s"
	}
	return cardinalWords(n) + " " + unit
}

func expandOrdinal(m string) string {
	sub := ordinalRe.FindStringSubmatch(m)
	if sub == nil {
		return m
	}
	n, err := strconv.Atoi(sub[1])
	if err != nil || n < 1 || n > 999 {
		return m
	}
	return ordinalWords(n)
}

// canSpell bounds the number words. Beyond this a figure is left to the engine,
// which is better than a sentence-long approximation nobody would read that
// way ("one billion two hundred and thirty-four million...").
func canSpell(n int) bool { return n >= 0 && n <= 999999999 }

var (
	onesWords = []string{
		"zero", "one", "two", "three", "four", "five", "six", "seven", "eight",
		"nine", "ten", "eleven", "twelve", "thirteen", "fourteen", "fifteen",
		"sixteen", "seventeen", "eighteen", "nineteen",
	}
	tensWords   = []string{"", "", "twenty", "thirty", "forty", "fifty", "sixty", "seventy", "eighty", "ninety"}
	ordinalOnes = []string{
		"", "first", "second", "third", "fourth", "fifth", "sixth", "seventh",
		"eighth", "ninth", "tenth", "eleventh", "twelfth", "thirteenth",
		"fourteenth", "fifteenth", "sixteenth", "seventeenth", "eighteenth",
		"nineteenth",
	}
	ordinalTens = []string{"", "", "twentieth", "thirtieth", "fortieth", "fiftieth", "sixtieth", "seventieth", "eightieth", "ninetieth"}
)

// cardinalWords writes an integer the way a person says it in Nigerian and
// British English: "one thousand two hundred and fifty", not "one thousand two
// hundred fifty", and never with digits.
func cardinalWords(n int) string {
	switch {
	case n < 0 || !canSpell(n):
		return strconv.Itoa(n)
	case n < 20:
		return onesWords[n]
	case n < 100:
		if n%10 == 0 {
			return tensWords[n/10]
		}
		return tensWords[n/10] + "-" + onesWords[n%10]
	case n < 1000:
		if n%100 == 0 {
			return onesWords[n/100] + " hundred"
		}
		return onesWords[n/100] + " hundred and " + cardinalWords(n%100)
	}
	for _, scale := range []struct {
		value int
		name  string
	}{{1000000, "million"}, {1000, "thousand"}} {
		if n >= scale.value {
			head := cardinalWords(n/scale.value) + " " + scale.name
			rest := n % scale.value
			switch {
			case rest == 0:
				return head
			case rest < 100:
				return head + " and " + cardinalWords(rest)
			default:
				return head + " " + cardinalWords(rest)
			}
		}
	}
	return strconv.Itoa(n)
}

func ordinalWords(n int) string {
	switch {
	case n < 20:
		return ordinalOnes[n]
	case n < 100:
		if n%10 == 0 {
			return ordinalTens[n/10]
		}
		return tensWords[n/10] + "-" + ordinalOnes[n%10]
	case n < 1000:
		if n%100 == 0 {
			return onesWords[n/100] + " hundredth"
		}
		return onesWords[n/100] + " hundred and " + ordinalWords(n%100)
	}
	return strconv.Itoa(n)
}

// yearWords reads a four-digit year the way it is spoken: "twenty twenty-six",
// "nineteen ninety-nine", and "two thousand and five" for the decade people
// actually say that way.
func yearWords(y int) string {
	if y < 1000 || y > 9999 {
		return cardinalWords(y)
	}
	if y >= 2000 && y < 2010 {
		if y == 2000 {
			return "two thousand"
		}
		return "two thousand and " + onesWords[y-2000]
	}
	century, rest := y/100, y%100
	if rest == 0 {
		return cardinalWords(century) + " hundred"
	}
	if rest < 10 {
		return cardinalWords(century) + " oh " + onesWords[rest]
	}
	return cardinalWords(century) + " " + cardinalWords(rest)
}

// ---------------------------------------------------------------------------
// Dates and times
// ---------------------------------------------------------------------------

var (
	monthNames = `january|february|march|april|may|june|july|august|september|october|november|december|jan|feb|mar|apr|jun|jul|aug|sep|sept|oct|nov|dec`
	// Either "14th of March 2026" / "14 March" or "March 14, 2026". A leading
	// "the" is part of the match so the day-first reading can supply its own
	// without doubling it: "on the 14th of March" must not become "on the the
	// fourteenth of March".
	dateRe = regexp.MustCompile(`(?i)\b(?:the\s+)?(?:(` + monthNames + `)\.?\s+(\d{1,2})(?:st|nd|rd|th)?(?:,?\s*(\d{4}))?|(\d{1,2})(?:st|nd|rd|th)?\s+(?:of\s+)?(` + monthNames + `)\.?(?:,?\s*(\d{4}))?)\b`)
	// "7:30", "19:45", "7:05".
	timeRe = regexp.MustCompile(`\b(\d{1,2}):(\d{2})\b`)
)

var monthRead = map[string]string{
	"january": "January", "february": "February", "march": "March", "april": "April",
	"may": "May", "june": "June", "july": "July", "august": "August",
	"september": "September", "october": "October", "november": "November", "december": "December",
	"jan": "January", "feb": "February", "mar": "March", "apr": "April",
	"jun": "June", "jul": "July", "aug": "August", "sep": "September",
	"sept": "September", "oct": "October", "nov": "November", "dec": "December",
}

func expandDate(m string, dayFirst bool) string {
	sub := dateRe.FindStringSubmatch(m)
	if sub == nil {
		return m
	}
	month, day, year := sub[1], sub[2], sub[3]
	if month == "" {
		month, day, year = sub[5], sub[4], sub[6]
	}
	name, ok := monthRead[strings.ToLower(month)]
	if !ok {
		return m
	}
	d, err := strconv.Atoi(day)
	if err != nil || d < 1 || d > 31 {
		return m
	}
	phrase := name + " " + ordinalWords(d)
	if dayFirst {
		phrase = "the " + ordinalWords(d) + " of " + name
	}
	if year != "" {
		y, err := strconv.Atoi(year)
		if err != nil {
			return m
		}
		phrase += " " + yearWords(y)
	}
	return phrase
}

func expandTime(m string) string {
	sub := timeRe.FindStringSubmatch(m)
	if sub == nil {
		return m
	}
	hour, err1 := strconv.Atoi(sub[1])
	minute, err2 := strconv.Atoi(sub[2])
	if err1 != nil || err2 != nil || hour > 23 || minute > 59 {
		return m
	}
	prefix := ""
	if hour >= 13 {
		// 19:45 is read as a twenty-four-hour clock: "nineteen forty-five".
		prefix = cardinalWords(hour)
	} else {
		h := hour
		if h == 0 {
			h = 12 // midnight reads "twelve oh five", never "zero"
		}
		prefix = cardinalWords(h)
	}
	switch {
	case minute == 0:
		return prefix + " o'clock"
	case minute < 10:
		return prefix + " oh " + onesWords[minute]
	default:
		return prefix + " " + cardinalWords(minute)
	}
}

// ---------------------------------------------------------------------------
// URLs
// ---------------------------------------------------------------------------

var urlRe = regexp.MustCompile(`(?i)\bhttps?://[^\s<>"']+|\bwww\.[^\s<>"']+`)

// spellURL turns a web address into something an engine says rather than tries
// to pronounce as a word. It is a reading, not a link: the scheme is dropped
// and the separators are named.
func spellURL(m string) string {
	s := strings.TrimSuffix(strings.TrimSuffix(m, "."), ",")
	s = strings.TrimPrefix(strings.TrimPrefix(s, "https://"), "http://")
	s = strings.ReplaceAll(s, "/", " slash ")
	s = strings.ReplaceAll(s, ".", " dot ")
	s = strings.ReplaceAll(s, "-", " dash ")
	s = strings.ReplaceAll(s, "_", " underscore ")
	return strings.Join(strings.Fields(s), " ")
}
