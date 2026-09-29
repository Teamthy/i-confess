package voiceengine

import (
	"fmt"
	"regexp"
	"strings"
)

// Content safety for synthetic minister speech (§36, §53).
//
// The minister's voice must not become a general impersonation endpoint.
// These checks are a deterministic floor, applied to every script before it is
// queued; they do not replace editorial review of catalogue content or the
// AI-response moderation step for generated reflections. They are
// deliberately conservative: a refused script can be reworded, a fabricated
// quotation in a real person's voice cannot be unpublished from the world.

// MaxScriptChars bounds one generation. Long-form audio is produced as an
// AudioSession of multiple items, not one enormous render.
const MaxScriptChars = 6000

// SafetyViolation explains a refusal with a stable code.
type SafetyViolation struct {
	Code   string `json:"code"`
	Detail string `json:"detail"`
}

func (v *SafetyViolation) Error() string {
	return fmt.Sprintf("content refused (%s): %s", v.Code, v.Detail)
}

var (
	// First-person identity claims and attributed quotations. Scripts voiced
	// by the minister are reflections and prayers, not statements that the
	// minister personally endorses something or said something.
	impersonationRes = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(pastor|minister|bishop|reverend|rev\.?|prophet|apostle|evangelist|dr\.?)\s+[A-Z][\w'-]*\s+(says|said|declares|endorses|announces|confirms|approves)\b`),
		regexp.MustCompile(`(?i)\bthis is (pastor|minister|bishop|reverend|prophet|apostle)\b`),
		regexp.MustCompile(`(?i)\bI,\s*(pastor|minister|bishop|reverend|prophet|apostle)\b`),
		regexp.MustCompile(`(?i)\b(my name is|i am)\s+(pastor|minister|bishop|reverend|prophet|apostle)\b`),
		regexp.MustCompile(`(?i)\bI (personally )?(endorse|recommend|approve of)\b`),
	}
	// Solicitation patterns are the classic voice-clone scam payload.
	solicitationRes = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(send|transfer|wire|donate|pay)\b.{0,40}\b(money|naira|dollars|funds|bitcoin|crypto|gift ?cards?|account|₦|\$)`),
		regexp.MustCompile(`(?i)\b(account number|bank details|routing number|sort code|bvn|otp|pin code|password)\b`),
	}
	contactRes = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bhttps?://|www\.[a-z0-9-]+\.[a-z]`),
		regexp.MustCompile(`\+?\d[\d\s().-]{8,}\d`),
		regexp.MustCompile(`(?i)[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}`),
	}
)

// ValidateScript applies the safety floor. userSubmitted scripts (words
// written by an end user, e.g. a personal confession) additionally refuse
// contact details and solicitation, which editorial scripts may legitimately
// contain (a church address in a devotional, say).
func ValidateScript(text string, userSubmitted bool) error {
	plain := strings.TrimSpace(text)
	if plain == "" {
		return &SafetyViolation{Code: "empty", Detail: "script is empty"}
	}
	if len(plain) > MaxScriptChars {
		return &SafetyViolation{Code: "too_long", Detail: fmt.Sprintf("script exceeds %d characters; split it into session items", MaxScriptChars)}
	}
	for _, re := range impersonationRes {
		if m := re.FindString(plain); m != "" {
			return &SafetyViolation{Code: "impersonation", Detail: fmt.Sprintf("scripts may not attribute statements or endorsements to a named person (%q)", m)}
		}
	}
	for _, re := range solicitationRes {
		if m := re.FindString(plain); m != "" {
			return &SafetyViolation{Code: "solicitation", Detail: fmt.Sprintf("scripts may not request money or credentials (%q)", m)}
		}
	}
	if userSubmitted {
		for _, re := range contactRes {
			if m := re.FindString(plain); m != "" {
				return &SafetyViolation{Code: "contact_details", Detail: fmt.Sprintf("personal scripts may not contain links, phone numbers or emails (%q)", m)}
			}
		}
	}
	return nil
}
