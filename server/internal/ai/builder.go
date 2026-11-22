package ai

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Teamthy/i-confess/internal/engine"
)

// Builder §43, §106 — AI orchestrates, never invents theology.
// NLU extracts {categories[], duration} from a user utterance, then the
// trusted Session Engine selects real Scripture-rooted content. A parser may
// only choose among the categories it is handed; it never writes confession
// text, never quotes Scripture, and never chooses a voice.

// Duration bounds enforced on every parse, whichever parser produced it.
const (
	MinDurationSeconds     = 60
	MaxDurationSeconds     = 3 * 3600
	DefaultDurationSeconds = 600
	// MaxCategories caps how many categories one utterance maps to, so a
	// rambling request does not dilute the session across the whole catalogue.
	MaxCategories = 3
)

type Request struct {
	Utterance  string   `json:"utterance"` // free text: "I need peace before surgery"
	Categories []string `json:"categories,omitempty"`
	Duration   int      `json:"duration_seconds,omitempty"`
	VoiceID    string   `json:"voice_id,omitempty"`
}

// Category is one category a parser may choose. The catalogue is read from the
// database per request (categories are backend-configurable), so a parser
// never relies on a hard-coded list.
type Category struct {
	Slug        string
	Name        string
	Description string
}

type Parsed struct {
	Categories []string `json:"categories"` // category slugs from the supplied catalogue
	Duration   int      `json:"duration_seconds"`
	VoiceID    string   `json:"voice_id"`
	Strategy   string   `json:"strategy"` // BALANCED default
	Confidence float64  `json:"confidence"`
	Reason     string   `json:"reason"`
	// Parser names what produced this result: "keyword", "claude", or
	// "keyword-fallback" when the configured model could not answer.
	Parser string `json:"parser"`
}

// Parser is the NLU interface. Implementations must return only slugs present
// in catalog.
type Parser interface {
	Parse(ctx context.Context, req Request, catalog []Category) (Parsed, error)
}

// ErrNoInput is returned when the request has neither text nor categories.
var ErrNoInput = errors.New("utterance or categories required")

// KeywordParser is the deterministic parser: no network call, no model. It is
// the default and the fallback when a model is unavailable.
type KeywordParser struct{}

// keywordToCategory maps words in an utterance to canonical category slugs
// (internal/seed/categories_canonical.go). Every target must be a canonical
// slug; TestKeywordTargetsAreCanonical enforces it.
var keywordToCategory = map[string]string{
	"heal": "healing", "sick": "healing", "pain": "healing", "surgery": "healing",
	"peace": "peace", "anxious": "peace", "anxiety": "peace", "calm": "peace", "worry": "peace",
	"faith": "faith", "trust": "faith",
	"purpose": "purpose", "direction": "direction", "decision": "direction",
	"joy": "joy", "gratitude": "gratitude", "thank": "gratitude",
	"confidence": "confidence", "courage": "confidence",
	"fear": "overcoming-fear", "afraid": "overcoming-fear", "scared": "overcoming-fear",
	"wisdom": "wisdom", "sleep": "rest", "rest": "rest", "tired": "rest",
	"future": "hope", "hope": "hope", "love": "love",
	"money": "finance", "debt": "finance", "job": "career", "work": "career",
	"marriage": "marriage", "husband": "marriage", "wife": "marriage",
	"family": "family", "children": "children", "kids": "children",
	"forgive": "forgiveness", "pray": "prayer",
}

// defaultCategory is used when nothing in the utterance matches.
const defaultCategory = "peace"

func (KeywordParser) Parse(_ context.Context, req Request, catalog []Category) (Parsed, error) {
	if strings.TrimSpace(req.Utterance) == "" && len(req.Categories) == 0 {
		return Parsed{}, ErrNoInput
	}
	// Caller-supplied categories are passed through; the handler resolves and
	// rejects unknown ones (editor flow).
	if len(req.Categories) > 0 {
		return Parsed{
			Categories: req.Categories, Duration: ClampDuration(req.Duration, 1800), VoiceID: req.VoiceID,
			Strategy: engine.StrategyBalanced, Confidence: 1, Reason: "explicit categories", Parser: "keyword",
		}, nil
	}

	allowed := make(map[string]bool, len(catalog))
	for _, c := range catalog {
		allowed[c.Slug] = true
	}

	lower := strings.ToLower(req.Utterance)
	// Order matches by where they appear in the utterance (ties by keyword) so
	// the result does not depend on Go's randomised map iteration.
	type match struct {
		pos int
		kw  string
	}
	var matches []match
	for kw := range keywordToCategory {
		if pos := strings.Index(lower, kw); pos >= 0 {
			matches = append(matches, match{pos, kw})
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].pos != matches[j].pos {
			return matches[i].pos < matches[j].pos
		}
		return matches[i].kw < matches[j].kw
	})
	var cats []string
	seen := map[string]bool{}
	for _, m := range matches {
		cat := keywordToCategory[m.kw]
		if allowed[cat] && !seen[cat] {
			cats = append(cats, cat)
			seen[cat] = true
		}
	}
	if len(cats) > MaxCategories {
		cats = cats[:MaxCategories]
	}
	confidence := 0.7
	if len(cats) == 0 {
		confidence = 0.3
		if allowed[defaultCategory] {
			cats = []string{defaultCategory}
		} else if len(catalog) > 0 {
			cats = []string{catalog[0].Slug}
		} else {
			return Parsed{}, errors.New("no categories are available")
		}
	}

	dur := req.Duration
	if dur == 0 {
		dur = DefaultDurationSeconds
		if secs, ok := spokenDuration(lower); ok {
			dur = secs
		} else if strings.Contains(lower, "long") {
			dur = 1800
		}
	}
	return Parsed{
		Categories: cats, Duration: ClampDuration(dur, DefaultDurationSeconds), VoiceID: req.VoiceID,
		Strategy: engine.StrategyBalanced, Confidence: confidence, Reason: "keyword parser", Parser: "keyword",
	}, nil
}

// ClampDuration returns seconds within [MinDurationSeconds, MaxDurationSeconds],
// using def when seconds is zero or negative.
func ClampDuration(seconds, def int) int {
	if seconds <= 0 {
		seconds = def
	}
	if seconds < MinDurationSeconds {
		return MinDurationSeconds
	}
	if seconds > MaxDurationSeconds {
		return MaxDurationSeconds
	}
	return seconds
}

// ToEngineRequest validates Parsed and returns an engine.Request. ids maps each
// category slug to its database id; the engine selects by id.
func (p Parsed) ToEngineRequest(ids map[string]string) (engine.Request, error) {
	if len(p.Categories) == 0 {
		return engine.Request{}, errors.New("no categories")
	}
	if p.Duration < MinDurationSeconds || p.Duration > MaxDurationSeconds {
		return engine.Request{}, errors.New("duration out of range")
	}
	start := p.Strategy
	if start == "" {
		start = engine.StrategyBalanced
	}
	if !engine.IsValidStrategy(start) {
		return engine.Request{}, errors.New("invalid strategy")
	}
	catIDs := make([]string, 0, len(p.Categories))
	for _, slug := range p.Categories {
		id, ok := ids[slug]
		if !ok {
			return engine.Request{}, errors.New("unknown category: " + slug)
		}
		catIDs = append(catIDs, id)
	}
	return engine.Request{
		CategoryIDs:     catIDs,
		DurationSeconds: p.Duration,
		Strategy:        engine.NormalizeStrategy(start),
		VoiceID:         p.VoiceID,
	}, nil
}

// numberWords covers how people say a session length out loud.
var numberWords = map[string]int{
	"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6, "seven": 7, "eight": 8, "nine": 9,
	"ten": 10, "eleven": 11, "twelve": 12, "fifteen": 15, "twenty": 20, "thirty": 30, "forty": 40,
	"forty-five": 45, "fortyfive": 45, "fifty": 50, "sixty": 60, "ninety": 90, "a": 1, "an": 1,
}

// Digits may take a bare unit ("15m", "2h"); words need a spelled-out one,
// so "I am anxious" is not read as "a m(inute)".
var durationPattern = regexp.MustCompile(`\b(?:(half an hour|half-hour)|(\d{1,3})\s*(minutes?|mins?|m|hours?|hrs?|h)\b|([a-z]+(?:-[a-z]+)?)\s+(minutes?|mins?|hours?|hrs?)\b)`)

// spokenDuration finds the first stated length in an utterance: "15 minutes",
// "fifteen minutes", "an hour", "2 hrs", "half an hour". It reports false when
// none is stated, so the caller keeps its default instead of guessing from a
// stray number ("Psalm 30" is not a length).
func spokenDuration(lower string) (int, bool) {
	for _, m := range durationPattern.FindAllStringSubmatch(lower, -1) {
		if m[1] != "" {
			return 1800, true
		}
		var n int
		unit := m[3]
		if m[2] != "" {
			n, _ = strconv.Atoi(m[2])
		} else {
			n, unit = numberWords[m[4]], m[5]
		}
		if n <= 0 {
			continue
		}
		if strings.HasPrefix(unit, "h") {
			return n * 3600, true
		}
		return n * 60, true
	}
	return 0, false
}
