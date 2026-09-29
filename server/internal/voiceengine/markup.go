package voiceengine

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// ---------------------------------------------------------------------------
// SpeechMarkup (§39)
//
// Internal authoring syntax, deliberately tiny and not SSML:
//
//	{pause 700}            silence in milliseconds (max 10000)
//	{emph}words{/emph}     emphasis
//	{say Term as re-spell} one-off pronunciation override
//	{speed 0.9}...{/speed} local rate change
//	{pitch -2}...{/pitch}  local pitch change in semitones
//
// Nothing outside this package sees engine markup. Render turns a parsed
// document into Chunks: plain text plus silence, with emphasis and phonemes
// passed only to engines that declare support for them. Engines without
// support get the words, never stray tags read aloud.
// ---------------------------------------------------------------------------

// Segment is one parsed piece of a SpeechMarkup document.
type Segment struct {
	Text     string  `json:"text,omitempty"`
	PauseMS  int     `json:"pause_ms,omitempty"`
	Emphasis bool    `json:"emphasis,omitempty"`
	Say      string  `json:"say,omitempty"` // respelling / phonemes for Text
	Speed    float64 `json:"speed,omitempty"`
	Pitch    float64 `json:"pitch,omitempty"`
}

// Chunk is what a worker synthesizes: text with optional per-chunk tweaks
// followed by inserted silence.
type Chunk struct {
	Text           string  `json:"text"`
	Emphasis       bool    `json:"emphasis,omitempty"`
	Phonemes       string  `json:"phonemes,omitempty"`
	SpeedFactor    float64 `json:"speed_factor,omitempty"`
	PitchSemitones float64 `json:"pitch_semitones,omitempty"`
	SilenceAfterMS int     `json:"silence_after_ms,omitempty"`
}

var tagRe = regexp.MustCompile(`\{(/?)(pause|emph|say|speed|pitch)(?:\s+([^}]*))?\}`)

const maxPauseMS = 10000

// ParseMarkup parses the authoring syntax. Unknown braces are left as text;
// malformed known tags are errors, because silently dropping a pause an
// editor asked for is worse than refusing the script.
func ParseMarkup(src string) ([]Segment, error) {
	var out []Segment
	var emph bool
	var speed, pitch float64
	pos := 0
	emit := func(text string) {
		if text == "" {
			return
		}
		out = append(out, Segment{Text: text, Emphasis: emph, Speed: speed, Pitch: pitch})
	}
	for _, m := range tagRe.FindAllStringSubmatchIndex(src, -1) {
		emit(src[pos:m[0]])
		pos = m[1]
		closing := src[m[2]:m[3]] == "/"
		name := src[m[4]:m[5]]
		arg := ""
		if m[6] >= 0 {
			arg = strings.TrimSpace(src[m[6]:m[7]])
		}
		switch name {
		case "pause":
			if closing {
				return nil, fmt.Errorf("{/pause} is not a tag")
			}
			ms, err := strconv.Atoi(strings.TrimSuffix(arg, "ms"))
			if err != nil || ms < 0 || ms > maxPauseMS {
				return nil, fmt.Errorf("invalid pause %q (0-%d ms)", arg, maxPauseMS)
			}
			out = append(out, Segment{PauseMS: ms})
		case "emph":
			if closing != emph {
				return nil, fmt.Errorf("unbalanced {emph}")
			}
			emph = !closing
		case "say":
			parts := strings.SplitN(arg, " as ", 2)
			if closing || len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
				return nil, fmt.Errorf("invalid {say %s}; expected {say Term as pronunciation}", arg)
			}
			out = append(out, Segment{Text: strings.TrimSpace(parts[0]), Say: strings.TrimSpace(parts[1]), Emphasis: emph, Speed: speed, Pitch: pitch})
		case "speed":
			if closing {
				speed = 0
				continue
			}
			v, err := strconv.ParseFloat(arg, 64)
			if err != nil || v < 0.5 || v > 2 {
				return nil, fmt.Errorf("invalid speed %q (0.5-2.0)", arg)
			}
			speed = v
		case "pitch":
			if closing {
				pitch = 0
				continue
			}
			v, err := strconv.ParseFloat(arg, 64)
			if err != nil || v < -12 || v > 12 {
				return nil, fmt.Errorf("invalid pitch %q (-12..12 semitones)", arg)
			}
			pitch = v
		}
	}
	emit(src[pos:])
	if emph {
		return nil, fmt.Errorf("unclosed {emph}")
	}
	return out, nil
}

// PlainText returns the words only, for hashing display text and ASR checks.
func PlainText(segs []Segment) string {
	var b strings.Builder
	for _, s := range segs {
		b.WriteString(s.Text)
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// Render converts segments into worker chunks for an engine with caps. Pauses
// are scaled by the prosody profile's pause multiplier. Long text is split at
// sentence boundaries to respect MaxChunkChars.
func Render(segs []Segment, caps ProviderCapabilities, prosody ProsodyProfile) []Chunk {
	mult := prosody.PauseMultiplier
	if mult <= 0 {
		mult = 1
	}
	var out []Chunk
	cur := Chunk{}
	flush := func() {
		if strings.TrimSpace(cur.Text) != "" || cur.SilenceAfterMS > 0 {
			cur.Text = strings.Join(strings.Fields(cur.Text), " ")
			out = append(out, cur)
		}
		cur = Chunk{}
	}
	for _, s := range segs {
		if s.PauseMS > 0 {
			cur.SilenceAfterMS += int(float64(s.PauseMS) * mult)
			flush()
			continue
		}
		speed := s.Speed
		if speed == 1 {
			speed = 0
		}
		emph := s.Emphasis && caps.Emphasis
		phon := ""
		text := s.Text
		if s.Say != "" {
			if caps.Phonemes && looksLikeIPA(s.Say) {
				phon = s.Say
			} else {
				// No phoneme support: speak the respelling instead of the
				// original spelling. This is the only portable option.
				text = respellingForSpeech(s.Say)
			}
		}
		// Start a new chunk whenever per-chunk parameters change.
		if cur.Text != "" && (cur.Emphasis != emph || cur.SpeedFactor != speed || cur.PitchSemitones != s.Pitch || phon != "" || cur.Phonemes != "") {
			flush()
		}
		cur.Emphasis, cur.SpeedFactor, cur.PitchSemitones = emph, speed, s.Pitch
		if phon != "" {
			cur.Phonemes = phon
		}
		cur.Text += text
	}
	flush()
	max := caps.MaxChunkChars
	if max <= 0 {
		max = 300
	}
	var split []Chunk
	for _, c := range out {
		split = append(split, splitChunk(c, max)...)
	}
	return split
}

func splitChunk(c Chunk, max int) []Chunk {
	if len(c.Text) <= max || c.Phonemes != "" {
		return []Chunk{c}
	}
	sentences := splitSentences(c.Text)
	var out []Chunk
	cur := c
	cur.Text, cur.SilenceAfterMS = "", 0
	for _, s := range sentences {
		if cur.Text != "" && len(cur.Text)+1+len(s) > max {
			out = append(out, cur)
			cur.Text = ""
		}
		if cur.Text != "" {
			cur.Text += " "
		}
		cur.Text += s
	}
	cur.SilenceAfterMS = c.SilenceAfterMS
	return append(out, cur)
}

func splitSentences(s string) []string {
	var out []string
	start := 0
	rs := []rune(s)
	for i, r := range rs {
		if (r == '.' || r == '!' || r == '?' || r == ';') && (i+1 == len(rs) || unicode.IsSpace(rs[i+1])) {
			out = append(out, strings.TrimSpace(string(rs[start:i+1])))
			start = i + 1
		}
	}
	if rest := strings.TrimSpace(string(rs[start:])); rest != "" {
		out = append(out, rest)
	}
	return out
}

func looksLikeIPA(s string) bool {
	return strings.HasPrefix(s, "/") && strings.HasSuffix(s, "/") && len(s) > 2
}

// respellingForSpeech turns "ha-BAK-kuk" into "ha bak kuk" so engines read
// syllables rather than letters or the word "dash".
func respellingForSpeech(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.ReplaceAll(s, "-", " ")), " "))
}

// ---------------------------------------------------------------------------
// Pronunciation dictionary (§15, §38)
// ---------------------------------------------------------------------------

// PronunciationEntry is one dictionary row.
type PronunciationEntry struct {
	Term string `json:"term"`
	// Locale is a BCP-47 tag ("en-NG"), a bare language ("en"), or "" for all.
	Locale     string   `json:"locale"`
	Respelling string   `json:"respelling,omitempty"`
	IPA        string   `json:"ipa,omitempty"`
	Aliases    []string `json:"aliases,omitempty"`
	// ProviderOverrides lets an engine that mispronounces a respelling use a
	// hand-tuned alternative.
	ProviderOverrides map[Engine]string `json:"provider_overrides,omitempty"`
	Category          string            `json:"category,omitempty"` // biblical_name, nigerian_place, ...
}

// Dictionary resolves terms for a locale.
type Dictionary struct{ entries []PronunciationEntry }

// NewDictionary builds a dictionary.
func NewDictionary(entries []PronunciationEntry) *Dictionary {
	return &Dictionary{entries: entries}
}

// Lookup returns the most specific entry for term in locale: exact locale,
// then language, then universal. en-NG entries never apply to en-NG-PIDGIN
// implicitly and vice versa: those are distinct varieties (§15).
func (d *Dictionary) Lookup(term, locale string) (PronunciationEntry, bool) {
	best, bestRank := PronunciationEntry{}, -1
	for _, e := range d.entries {
		if !matchesTerm(e, term) {
			continue
		}
		rank := -1
		switch {
		case strings.EqualFold(e.Locale, locale):
			rank = 3
		case e.Locale != "" && !strings.Contains(e.Locale, "-") && primary(e.Locale) == primary(locale) && !isPidgin(locale):
			rank = 2
		case e.Locale == "":
			rank = 1
		}
		if rank > bestRank {
			best, bestRank = e, rank
		}
	}
	return best, bestRank >= 0
}

func isPidgin(locale string) bool { return strings.Contains(strings.ToUpper(locale), "PIDGIN") }

func matchesTerm(e PronunciationEntry, term string) bool {
	if strings.EqualFold(e.Term, term) {
		return true
	}
	for _, a := range e.Aliases {
		if strings.EqualFold(a, term) {
			return true
		}
	}
	return false
}

// Apply rewrites plain-text segments so dictionary terms carry a Say value
// suitable for engine. Segments that already carry an explicit {say} win.
func (d *Dictionary) Apply(segs []Segment, locale string, engine Engine, caps ProviderCapabilities) []Segment {
	if d == nil || len(d.entries) == 0 {
		return segs
	}
	terms := d.termsLongestFirst()
	var out []Segment
	for _, s := range segs {
		if s.Text == "" || s.Say != "" {
			out = append(out, s)
			continue
		}
		out = append(out, d.applyOne(s, terms, locale, engine, caps)...)
	}
	return out
}

func (d *Dictionary) termsLongestFirst() []string {
	seen := map[string]bool{}
	var terms []string
	for _, e := range d.entries {
		for _, t := range append([]string{e.Term}, e.Aliases...) {
			k := strings.ToLower(t)
			if !seen[k] {
				seen[k] = true
				terms = append(terms, t)
			}
		}
	}
	sort.Slice(terms, func(i, j int) bool { return len(terms[i]) > len(terms[j]) })
	return terms
}

func (d *Dictionary) applyOne(s Segment, terms []string, locale string, engine Engine, caps ProviderCapabilities) []Segment {
	for _, t := range terms {
		re := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(t) + `\b`)
		loc := re.FindStringIndex(s.Text)
		if loc == nil {
			continue
		}
		e, ok := d.Lookup(t, locale)
		if !ok {
			continue
		}
		say := e.ProviderOverrides[engine]
		if say == "" && caps.Phonemes && e.IPA != "" {
			say = "/" + strings.Trim(e.IPA, "/") + "/"
		}
		if say == "" {
			say = e.Respelling
		}
		if say == "" {
			continue
		}
		before, match, after := s.Text[:loc[0]], s.Text[loc[0]:loc[1]], s.Text[loc[1]:]
		var out []Segment
		if before != "" {
			b := s
			b.Text = before
			out = append(out, d.applyOne(b, terms, locale, engine, caps)...)
		}
		m := s
		m.Text, m.Say = match, say
		out = append(out, m)
		if after != "" {
			a := s
			a.Text = after
			out = append(out, d.applyOne(a, terms, locale, engine, caps)...)
		}
		return out
	}
	return []Segment{s}
}

// ---------------------------------------------------------------------------
// Prosody profiles (§40)
// ---------------------------------------------------------------------------

// ProsodyProfile is engine-neutral delivery. Adapters translate it.
type ProsodyProfile struct {
	Name            string  `json:"name"`
	Speed           float64 `json:"speed"`
	PauseMultiplier float64 `json:"pause_multiplier"`
	Energy          float64 `json:"energy"`
	PitchVariation  float64 `json:"pitch_variation"`
}

// DefaultProsody are starting profiles; admins override them per voice.
var DefaultProsody = map[string]ProsodyProfile{
	"calm":          {Name: "calm", Speed: 0.95, PauseMultiplier: 1.15, Energy: 0.35, PitchVariation: 0.25},
	"reflective":    {Name: "reflective", Speed: 0.92, PauseMultiplier: 1.3, Energy: 0.3, PitchVariation: 0.25},
	"warm":          {Name: "warm", Speed: 0.97, PauseMultiplier: 1.1, Energy: 0.45, PitchVariation: 0.35},
	"gentle":        {Name: "gentle", Speed: 0.93, PauseMultiplier: 1.2, Energy: 0.3, PitchVariation: 0.2},
	"authoritative": {Name: "authoritative", Speed: 1.0, PauseMultiplier: 1.0, Energy: 0.65, PitchVariation: 0.4},
	"energetic":     {Name: "energetic", Speed: 1.05, PauseMultiplier: 0.9, Energy: 0.8, PitchVariation: 0.55},
	"preaching":     {Name: "preaching", Speed: 1.0, PauseMultiplier: 1.1, Energy: 0.75, PitchVariation: 0.6},
	"prayer":        {Name: "prayer", Speed: 0.9, PauseMultiplier: 1.25, Energy: 0.35, PitchVariation: 0.2},
	"teaching":      {Name: "teaching", Speed: 0.98, PauseMultiplier: 1.05, Energy: 0.5, PitchVariation: 0.35},
	"scripture":     {Name: "scripture", Speed: 0.93, PauseMultiplier: 1.2, Energy: 0.45, PitchVariation: 0.3},
}

// styleProsody maps content styles (§17) to a default profile.
var styleProsody = map[string]string{
	"teaching": "teaching", "preaching": "preaching", "prayer": "prayer",
	"reflection": "reflective", "scripture": "scripture", "encouragement": "warm",
	"storytelling": "warm", "counseling": "gentle", "conversational": "calm",
	"neutral": "calm", "confession": "calm",
}

// ProsodyFor returns the profile for a style, applying request overrides.
func ProsodyFor(style string, speedOverride float64) ProsodyProfile {
	p, ok := DefaultProsody[styleProsody[strings.ToLower(style)]]
	if !ok {
		p = DefaultProsody["calm"]
	}
	if speedOverride > 0 {
		p.Speed = speedOverride
	}
	return p
}

// Styles are the content styles recordings are classified into (§17).
var Styles = []string{"neutral", "teaching", "preaching", "prayer", "reflection", "scripture",
	"encouragement", "storytelling", "counseling", "conversational"}

// ValidStyle reports whether s is a known style.
func ValidStyle(s string) bool {
	for _, v := range Styles {
		if v == s {
			return true
		}
	}
	return false
}

// SelectReference picks the best rights-cleared reference for voice+style,
// falling back to "neutral". It never returns another voice's reference.
func SelectReference(refs []Reference, voiceID, style string) (*Reference, error) {
	pick := func(st string) *Reference {
		var best *Reference
		for i := range refs {
			r := &refs[i]
			if r.VoiceID != voiceID || !r.RightsOK || r.Style != st {
				continue
			}
			if best == nil || r.Quality > best.Quality {
				best = r
			}
		}
		return best
	}
	if r := pick(style); r != nil {
		return r, nil
	}
	if r := pick("neutral"); r != nil {
		return r, nil
	}
	return nil, &Error{Class: ClassModel, Msg: fmt.Sprintf("no rights-cleared reference for voice %s style %s or neutral", voiceID, style)}
}
