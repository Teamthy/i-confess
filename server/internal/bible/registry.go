package bible

import "fmt"

// registry.go — what the platform is allowed to ship, and why.
//
// Every translation here is public domain. That is not a preference, it is the
// only basis on which the text can be redistributed at all: the platform
// stores, serves and reproduces Scripture, and a modern copyrighted
// translation cannot be redistributed that way without a licence from its
// publisher. The large crawled corpora (thiagobodruk, scrollmapper, getbible)
// were surveyed and rejected for exactly this reason - their own READMEs state
// that the translations are the property of their respective owners. Nothing
// from them is imported here.
//
// All twelve files come from one upstream repository, seven1m/open-bibles,
// which is the surveyed source that publishes a per-file licence column
// stating each translation's terms. The licence string and its URL travel with
// the version into the API and the apps, because a user reading a verse is
// entitled to know the terms it arrives under.
//
// The SHA-256 is of the upstream file as fetched. The importer refuses to load
// a source whose digest does not match, which is what makes "verified" a
// property of the shipped data rather than a claim about it: if a source file
// changes upstream, the import fails loudly instead of silently loading
// different Scripture under the same name.
//
// Two more languages were searched for and not found. Yoruba, Igbo and Hausa
// have no public-domain text on any reachable source - the modern editions are
// Bible Society copyright and the rest are unlicensed - so they are a data
// drop to be added behind this same interface when licensed text is available,
// not a gap in the feature. Twelve versions across seven languages is what
// verifies today: six English editions, plus Swahili, Spanish, Portuguese,
// French, Italian and Tagalog, all of them public domain.

// Coverage describes how much of the canon a version's scope promises.
const (
	// CoverageFull is a complete 66-book Bible.
	CoverageFull = "full"
	// CoverageNewTestament is a New Testament. The reader shows the books a
	// version actually has and the API reports the coverage, so a client can
	// say "New Testament" rather than render an empty Old Testament and look
	// broken. A New Testament that also carries some Old Testament books is
	// still labelled new_testament; the extra books are reported, not hidden.
	CoverageNewTestament = "new_testament"
)

// Version is one translation the importer knows how to load.
type Version struct {
	// ID is the stable identifier used in API paths, database columns and
	// highlight/bookmark rows. It never changes once shipped.
	ID string
	// Name is the full name shown to a reader.
	Name string
	// Abbrev is the short form used in reference lists and badges.
	Abbrev string
	// Language is the ISO 639-1 code ("en", "sw").
	Language string
	// LanguageName is the display name ("English", "Swahili").
	LanguageName string
	// Format is the source dialect: FormatOSIS, FormatUSFX or FormatZefania.
	Format string
	// Coverage is CoverageFull or CoverageNewTestament.
	Coverage string
	// Year is the publication year where the version's own name states it.
	Year string
	// Licence is the redistribution licence, e.g. "Public Domain".
	Licence string
	// LicenceURL links to the licence text, where one applies.
	LicenceURL string
	// LicenceNote records a caveat about the licence that a reader should see.
	// It is empty for the versions whose terms are unambiguous.
	LicenceNote string
	// OmittedBooks are canonical books the source file does not contain. Every
	// one must be declared here or verification fails: an import that silently
	// drops a book is indistinguishable from a file that never had it.
	OmittedBooks []string
	// OmittedChapters are chapters the source file does not contain, written
	// as "Matt.23".
	OmittedChapters []string
	// Attribution is the provenance line: repository plus the upstream
	// filename, so a reader can find the exact file this text came from.
	Attribution string
	// SourceFile is the upstream filename in the open-bibles repository. The
	// blob URL for a reader and the API URL the fetcher uses are both derived
	// from it, so the three cannot disagree.
	SourceFile string
	// SHA256 is the expected digest of that file.
	SHA256 string
	// Bytes is the expected size of that file, used to catch a truncated
	// download before the part that costs minutes (parsing) happens.
	Bytes int64
	// SortOrder controls the order versions appear in the picker. The KJV is
	// first because it is the translation the confession corpus cites, so it
	// is the one that resolves every deep link in the library.
	SortOrder int
	// Default marks the version a reader lands on when they have no stored
	// preference.
	Default bool
}

// OpenBiblesBase is the upstream repository every version here is fetched
// from.
const OpenBiblesBase = "https://github.com/seven1m/open-bibles/blob/master/"

// PublicDomainURL is the licence link carried by every version in this
// registry.
const PublicDomainURL = "https://creativecommons.org/publicdomain/mark/1.0/"

// Versions is the shipped registry: twelve public-domain translations across
// seven languages.
var Versions = []Version{
	{
		ID: "kjv", Name: "King James Version", Abbrev: "KJV",
		Language: "en", LanguageName: "English", Format: FormatOSIS,
		Coverage: CoverageFull, Year: "1611",
		Licence: "Public Domain", LicenceURL: PublicDomainURL,
		Attribution: "seven1m/open-bibles (eng-kjv.osis.xml)",
		SourceFile:  "eng-kjv.osis.xml",
		SHA256:      "eeeae647fc28360ce47f9c0d5cc3b397b7fdd9913fe53dc9f44eb6deee50e253",
		Bytes:       10096487, SortOrder: 1, Default: true,
	},
	{
		ID: "web", Name: "World English Bible", Abbrev: "WEB",
		Language: "en", LanguageName: "English", Format: FormatUSFX,
		Coverage: CoverageFull,
		Licence:  "Public Domain", LicenceURL: PublicDomainURL,
		Attribution: "seven1m/open-bibles (eng-web.usfx.xml)",
		SourceFile:  "eng-web.usfx.xml",
		SHA256:      "5ffa2626f170a109a4a96afc90775c06f0821cb4ba81ed34e63663e085708d68",
		Bytes:       6210746, SortOrder: 2,
	},
	{
		ID: "asv", Name: "American Standard Version", Abbrev: "ASV",
		Language: "en", LanguageName: "English", Format: FormatZefania,
		Coverage: CoverageFull, Year: "1901",
		Licence: "Public Domain", LicenceURL: PublicDomainURL,
		Attribution: "seven1m/open-bibles (eng-asv.zefania.xml)",
		SourceFile:  "eng-asv.zefania.xml",
		SHA256:      "ef20ebf6b56bf1e3dfab949af55e83ff5887565ed24aeecd51da7a45210914bf",
		Bytes:       5239191, SortOrder: 3,
	},
	{
		ID: "webbe", Name: "World English Bible, British Edition", Abbrev: "WEBBE",
		Language: "en", LanguageName: "English", Format: FormatUSFX,
		Coverage: CoverageFull,
		Licence:  "Public Domain", LicenceURL: PublicDomainURL,
		Attribution: "seven1m/open-bibles (eng-gb-webbe.usfx.xml)",
		SourceFile:  "eng-gb-webbe.usfx.xml",
		SHA256:      "f12734398614d77d8a124332c23250c1bee89ba79c1425add523b0e17675457e",
		Bytes:       18723593, SortOrder: 4,
	},
	{
		ID: "bsb", Name: "Berean Standard Bible", Abbrev: "BSB",
		Language: "en", LanguageName: "English", Format: FormatUSFX,
		Coverage: CoverageFull, Year: "2020",
		Licence: "Public Domain", LicenceURL: PublicDomainURL,
		Attribution: "seven1m/open-bibles (eng-bsb.usfx.xml)",
		SourceFile:  "eng-bsb.usfx.xml",
		SHA256:      "7bf75004ecbc8901f179bd36078cf48fa4eb34cb2894791342ac840bcbf586fb",
		Bytes:       17599832, SortOrder: 5,
	},
	{
		ID: "ylt", Name: "Young's Literal Translation", Abbrev: "YLT",
		Language: "en", LanguageName: "English", Format: FormatZefania,
		Coverage: CoverageNewTestament, Year: "1862",
		Licence: "Public Domain", LicenceURL: PublicDomainURL,
		Attribution: "seven1m/open-bibles (eng-ylt.zefania.xml)",
		SourceFile:  "eng-ylt.zefania.xml",
		SHA256:      "e6fd78a6c26d758ec5cf06fdd0d702bb770628e227d932156ba835e5e46b4665",
		Bytes:       1416994, SortOrder: 6,
	},
	{
		ID: "swahili", Name: "Swahili Bible", Abbrev: "SWA",
		Language: "sw", LanguageName: "Swahili", Format: FormatOSIS,
		Coverage: CoverageNewTestament,
		Licence:  "Public Domain", LicenceURL: PublicDomainURL,
		LicenceNote: "The upstream file's own rights line reads \"i think public domain\" rather than citing a formal dedication; the source repository publishes it as Public Domain. Treat the terms as public domain per the source, and re-check before any commercial redistribution outside this platform.",
		// Both omissions are properties of the source file, verified against
		// the raw XML: it has no Philippians at all, and Matthew jumps from 22
		// to 24. Declaring them is what keeps the reader honest - it shows a
		// New Testament that lacks those passages instead of appearing whole.
		OmittedBooks:    []string{"Phil"},
		OmittedChapters: []string{"Matt.23"},
		Attribution:     "seven1m/open-bibles (swa-swahili.osis.xml)",
		SourceFile:      "swa-swahili.osis.xml",
		SHA256:          "1ed6bdb2291d711d838b574ff6c2647cc897d9b766a510ba72d869038e5316ab",
		Bytes:           1279743, SortOrder: 7,
	},
	{
		ID: "rv1909", Name: "Reina Valera 1909", Abbrev: "RV1909",
		Language: "es", LanguageName: "Spanish", Format: FormatUSFX,
		Coverage: CoverageFull, Year: "1909",
		Licence: "Public Domain", LicenceURL: PublicDomainURL,
		Attribution: "seven1m/open-bibles (spa-rv1909.usfx.xml)",
		SourceFile:  "spa-rv1909.usfx.xml",
		SHA256:      "233b4d6a87f833ac809d0e68dcf9ba83c709d612e914a357a4f5473782b703a3",
		Bytes:       11293936, SortOrder: 8,
	},
	{
		ID: "almeida", Name: "João Ferreira de Almeida", Abbrev: "ALM",
		Language: "pt", LanguageName: "Portuguese", Format: FormatUSFX,
		Coverage: CoverageFull,
		Licence:  "Public Domain", LicenceURL: PublicDomainURL,
		Attribution: "seven1m/open-bibles (por-almeida.usfx.xml)",
		SourceFile:  "por-almeida.usfx.xml",
		SHA256:      "4990d0d1909e2bd57aa6ecc2a2ce9a565b154aac8575b0242dc943ad796e643e",
		Bytes:       4745976, SortOrder: 9,
	},
	{
		ID: "ostervald", Name: "Ostervald Bible", Abbrev: "OST",
		Language: "fr", LanguageName: "French", Format: FormatOSIS,
		Coverage: CoverageFull,
		Licence:  "Public Domain", LicenceURL: PublicDomainURL,
		Attribution: "seven1m/open-bibles (fra-ostervald.osis.xml)",
		SourceFile:  "fra-ostervald.osis.xml",
		SHA256:      "5a69ce3e1c4ab6b69dd6d21b74c6891afb1882c531d0fec416438d4aee674d52",
		Bytes:       5663717, SortOrder: 10,
	},
	{
		ID: "riveduta", Name: "Italian Riveduta Bible", Abbrev: "RIV",
		Language: "it", LanguageName: "Italian", Format: FormatOSIS,
		Coverage: CoverageFull, Year: "1927",
		Licence: "Public Domain", LicenceURL: PublicDomainURL,
		Attribution: "seven1m/open-bibles (ita-riveduta.osis.xml)",
		SourceFile:  "ita-riveduta.osis.xml",
		SHA256:      "e0b5c35ad8c2edfc76cbc4d988f67e7b702c9214e538426b5c5f01b33da1df20",
		Bytes:       5500177, SortOrder: 11,
	},
	{
		ID: "tagalog", Name: "Tagalog Bible (Ang Dating Biblia)", Abbrev: "TLAB",
		Language: "tl", LanguageName: "Tagalog", Format: FormatOSIS,
		Coverage: CoverageFull,
		Licence:  "Public Domain", LicenceURL: PublicDomainURL,
		Attribution: "seven1m/open-bibles (tgl-tagalog.osis.xml)",
		SourceFile:  "tgl-tagalog.osis.xml",
		SHA256:      "c7567b9c234392d3538a889775412335e0ff5b04c18ebd237722eb6ffe37709d",
		Bytes:       6258591, SortOrder: 12,
	},
}

// ExcludedSource records a candidate that was audited and left out, with the
// measurement that ruled it out. It is here rather than in a document so the
// decision sits next to the registry it constrains: the next person to
// consider adding one of these files can see what happened last time.
type ExcludedSource struct {
	File   string
	Reason string
}

// ExcludedSources are the audited candidates that are not shipped.
var ExcludedSources = []ExcludedSource{
	{
		File:   "eng-bbe.usfx.xml",
		Reason: "The upstream file is truncated: it ends mid-document after Revelation 22:21 with no closing tags, and re-downloading returns the same bytes, so the defect is upstream rather than local. The content is complete through the last verse, but importing a file whose digest cannot be relied on to be a well-formed document is not worth the repair logic.",
	},
	{
		File:   "eng-dra.zefania.xml",
		Reason: "Chapter numbering does not follow the canon: the file labels the Hebrew letter heading \"Aleph\" as Psalm 119:1 and shifts the psalm's 176 verses into Psalm 120, gives Psalms 152 chapters and Daniel 14, and so misaligns 760 verses. Every deep link into the Psalms would point at the wrong text.",
	},
	{
		File:   "eng-gb-oeb.osis.xml, eng-us-oeb.osis.xml",
		Reason: "Partial coverage: 42 books (the New Testament plus 15 Old Testament books). Shippable behind the same coverage reporting as the Swahili file, but the twelve slots are better used on complete Bibles and a second language.",
	},
	{
		File:   "deu-luther1912.osis.xml",
		Reason: "Uses the Luther versification, which disagrees with the canon structurally rather than in a few verses: Joel has 4 chapters against 3 and Malachi 3 against 4, and chapters differ by up to 16 verses. Importing it needs a per-version chapter mapping, which is not built.",
	},
	{
		File:   "lat-clementine.usfx.xml",
		Reason: "Vulgate Psalm numbering: 1,197 chapters against the canon's 1,189, because Psalms 9/10 and 113/114/115 are divided differently. 1,735 verses (5.5%) sit in a chapter the canon numbers differently.",
	},
}

// VersionByID returns the registry entry for a version identifier.
func VersionByID(id string) (Version, bool) {
	for _, v := range Versions {
		if v.ID == id {
			return v, true
		}
	}
	return Version{}, false
}

// BlobURL is the human-readable upstream location of this version's source
// file, suitable for the provenance line in the apps.
func (v Version) BlobURL() string { return OpenBiblesBase + v.SourceFile }

// FetchURL is the GitHub API URL the importer downloads the file from.
// raw.githubusercontent.com is not reachable from every environment this runs
// in, while the contents API served with an Accept header returns the same
// bytes.
func (v Version) FetchURL() string {
	return "https://api.github.com/repos/seven1m/open-bibles/contents/" + v.SourceFile
}

// DefaultVersion is the version a reader lands on with no stored preference.
func DefaultVersion() Version {
	for _, v := range Versions {
		if v.Default {
			return v
		}
	}
	return Versions[0]
}

// Languages lists the shipped languages in picker order, each once.
func Languages() []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range Versions {
		if !seen[v.Language] {
			seen[v.Language] = true
			out = append(out, v.Language)
		}
	}
	return out
}

// CoverageLabel is the user-facing description of how much of the Bible a
// version contains, including any books or chapters the source is missing.
func (v Version) CoverageLabel() string {
	var b string
	switch v.Coverage {
	case CoverageNewTestament:
		b = "New Testament"
	default:
		b = "Old and New Testaments"
	}
	if n := len(v.OmittedBooks); n == 1 {
		if book, ok := BookByID(v.OmittedBooks[0]); ok {
			b += ", without " + book.Name
		} else {
			b += ", one book missing"
		}
	} else if n > 1 {
		b += fmt.Sprintf(", %d books missing", n)
	}
	if n := len(v.OmittedChapters); n > 0 {
		b += fmt.Sprintf(", %d chapter(s) missing", n)
	}
	return b
}
