package voiceengine

// Natural-language style instructions (audit VE-006).
//
// The style system's strongest lever was unconnected: a CosyVoice render
// carried speed, pause and pitch numbers, so "prayer" and "preaching" differed
// only in how fast and how loudly the same sentence was read. CosyVoice's
// expressive control is `inference_instruct2`, which takes an instruction in
// words ("speak as a prayer, slowly and reverently"), and the adapter never
// sent one — the Go side had already been putting the style *name* on the wire
// as `instruct_style` and the worker ignored it.
//
// These are product-level texts, written to be model-agnostic and honest about
// delivery rather than to game one checkpoint's phrasing. They live here, not in
// the worker, because style is a product concept: the worker receives an
// instruction and passes it through, so swapping engines does not mean
// re-teaching the engine what a prayer sounds like.
//
// Keyed by the same names as DefaultProsody, and only sent for styles that have
// one: an unknown style falls back to the parameters alone rather than being
// described as something it is not.

// styleInstructions maps a style name to the sentence a instruct-capable engine
// is asked to follow.
var styleInstructions = map[string]string{
	"calm":          "Speak calmly and evenly, unhurried, with steady warmth.",
	"reflective":    "Speak reflectively, as if thinking aloud, with unhurried pauses.",
	"warm":          "Speak warmly and personally, with gentle encouragement.",
	"gentle":        "Speak gently and softly, with care and tenderness.",
	"authoritative": "Speak with quiet authority and conviction, clearly and firmly.",
	"energetic":     "Speak with energy and momentum, bright and lively.",
	"preaching":     "Speak as a preacher: confident and passionate, building emphasis on the key phrases.",
	"prayer":        "Speak as a prayer: slow, quiet and reverent, with soft pauses.",
	"teaching":      "Speak as a teacher: clear and measured, explaining with patience.",
	"scripture":     "Speak scripture slowly and reverently, honouring each phrase.",
}

// InstructionFor returns the natural-language instruction for a style, or "" for
// an unknown one. The style wins over the prosody profile's name when both are
// set, so an explicit request is never overridden by a default.
func InstructionFor(style string, profile ProsodyProfile) string {
	if s, ok := styleInstructions[style]; ok {
		return s
	}
	return styleInstructions[profile.Name]
}
