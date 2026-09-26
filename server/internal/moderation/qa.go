package moderation

import (
	"strconv"
	"strings"
	"time"

	"github.com/Teamthy/i-confess/internal/models"
)

// QAFacts is everything the §75 checklist inspects, gathered by the store in
// one transaction so the decision and the write that follows it see the same
// world.
type QAFacts struct {
	// HasText is true when at least one of the confession's short, medium or
	// long texts is non-empty after trimming. A confession with no text
	// cannot be voiced, so approving one would publish a silent track.
	HasText bool

	// ReadyRenders counts audio assets for this confession that passed their
	// own QA (status ready or published) with a positive duration.
	ReadyRenders int

	// RendersInFlight counts audio assets still being made (uploading or
	// processing). QA cannot conclude while the render it is judging may
	// still change.
	RendersInFlight int

	// UnlicensedVoices counts ready/published renders whose voice has no
	// active licence row. The generation path refuses unlicensed synthesis up
	// front (voice rights gate); this check is the backstop that catches a
	// licence revoked between generation and approval.
	UnlicensedVoices int
}

// RunQAChecklist evaluates the §75 audio QA checklist against facts and
// returns the report that will be persisted to confessions.qa_report —
// including on failure, because a failed gate that leaves no evidence looks
// exactly like a gate nobody ran.
//
// The gate is the last human check before APPROVED (route summary: "Run Audio
// QA checklist (§75) before APPROVED"). It deliberately does not verify what
// earlier lifecycle states own: wording belongs to content_review and
// doctrine to theological_review, which §22 makes mandatory and which a
// confession must pass through to reach audio_qa at all.
func RunQAChecklist(facts QAFacts, actor, note string, now time.Time) models.QAReport {
	checks := []models.QACheck{
		{
			Name:   "text_present",
			Passed: facts.HasText,
			Detail: boolDetail(facts.HasText, "at least one text variant is present", "no short, medium or long text - nothing to voice"),
		},
		{
			Name:   "audio_ready",
			Passed: facts.ReadyRenders > 0,
			Detail: boolDetail(facts.ReadyRenders > 0,
				plural(facts.ReadyRenders, "render has", "renders have")+" passed audio QA",
				"no audio render has passed audio QA yet"),
		},
		{
			Name:   "no_render_in_flight",
			Passed: facts.RendersInFlight == 0,
			Detail: boolDetail(facts.RendersInFlight == 0,
				"no render still uploading or processing",
				plural(facts.RendersInFlight, "render is", "renders are")+" still in flight"),
		},
		{
			Name:   "voices_licensed",
			Passed: facts.UnlicensedVoices == 0,
			Detail: boolDetail(facts.UnlicensedVoices == 0,
				"every servable render uses a licensed voice",
				plural(facts.UnlicensedVoices, "servable render uses a voice", "servable renders use voices")+" with no active licence"),
		},
	}

	passed := true
	for _, c := range checks {
		if !c.Passed {
			passed = false
			break
		}
	}

	return models.QAReport{
		RanAt:  now.UTC().Format(time.RFC3339),
		Actor:  actor,
		Passed: passed,
		Note:   strings.TrimSpace(note),
		Checks: checks,
	}
}

func boolDetail(ok bool, whenOK, whenFail string) string {
	if ok {
		return whenOK
	}
	return whenFail
}

func plural(n int, singular, plural string) string {
	word := plural
	if n == 1 {
		word = singular
	}
	return strconv.Itoa(n) + " " + word
}
