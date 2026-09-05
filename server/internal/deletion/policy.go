// Package deletion implements account erasure with an explicit, auditable
// per-table policy (§40, §50, §84, §85).
//
// The design rule: nothing is erased or retained by accident. Every table
// holding user data appears in exactly one policy below, and a test asserts
// that the union covers every table in the schema that references users. A new
// table added without a policy fails that test rather than silently surviving a
// user's deletion request — which is precisely the failure regulators care
// about.
package deletion

import "fmt"

// Action is what happens to a table's rows when a user is erased.
type Action int

const (
	// Erase deletes the rows outright. The default for personal data.
	Erase Action = iota
	// Anonymise keeps the rows but severs them from the person, used where
	// aggregate integrity matters and the row carries no identity once
	// detached.
	Anonymise
	// Retain keeps the rows intact because law or contract requires it. Every
	// entry must state which obligation, in prose, in Reason.
	Retain
)

func (a Action) String() string {
	switch a {
	case Erase:
		return "erase"
	case Anonymise:
		return "anonymise"
	case Retain:
		return "retain"
	}
	return "unknown"
}

// TablePolicy is the deletion rule for one table.
type TablePolicy struct {
	Table  string
	Action Action
	// Column is the user reference. Defaults to "user_id".
	Column string
	// Reason explains the choice. Required for Retain and Anonymise: an
	// unexplained retention is indistinguishable from an oversight.
	Reason string
}

func (p TablePolicy) column() string {
	if p.Column == "" {
		return "user_id"
	}
	return p.Column
}

// Policies is the complete deletion policy.
//
// Ordered so that dependent rows go before the rows they reference, which keeps
// the erasure valid under foreign keys even where the schema does not cascade.
var Policies = []TablePolicy{
	// ---- Credentials and sessions. Erased first: an in-flight session must
	// not survive the account it belongs to.
	{Table: "refresh_tokens", Action: Erase},
	{Table: "email_verification_tokens", Action: Erase},
	{Table: "password_reset_tokens", Action: Erase},
	{Table: "mfa_secrets", Action: Erase},
	{Table: "user_identities", Action: Erase},
	{Table: "admin_users", Action: Erase},

	// ---- Profile and preferences: the person themselves.
	{Table: "user_profiles", Action: Erase},
	{Table: "user_preferences", Action: Erase},
	{Table: "user_interests", Action: Erase},
	{Table: "user_voice_preferences", Action: Erase},
	{Table: "notification_preferences", Action: Erase},
	{Table: "session_preferences", Action: Erase},

	// ---- Devices. Push tokens must go promptly: a retained token can still
	// deliver a notification to a phone whose owner has left.
	{Table: "user_devices", Action: Erase},

	// ---- User-created content and library.
	{Table: "user_collection_items", Action: Erase, Column: "collection_id",
		Reason: "removed via their parent collection"},
	{Table: "user_collections", Action: Erase},
	{Table: "user_confessions", Action: Erase},
	{Table: "favorites", Action: Erase},
	{Table: "schedules", Action: Erase},
	{Table: "session_items", Action: Erase, Column: "session_id",
		Reason: "removed via their parent session"},
	{Table: "sessions", Action: Erase},

	// ---- Notification dispatch log. Erased with the account: it records when
	// someone was reminded to pray, which is behavioural data about them, and
	// the schedules it references are going too.
	{Table: "scheduled_deliveries", Action: Erase},

	// ---- Listening behaviour. Erased rather than anonymised: a listening
	// history is a record of what a person prayed about, which is exactly the
	// kind of data a deletion request is usually about.
	{Table: "playback_history", Action: Erase},
	{Table: "playback_progress", Action: Erase},
	{Table: "audio_playback_sessions", Action: Erase},
	{Table: "audio_downloads", Action: Erase},

	// ---- Idempotency and templates. The listener's own working data.
	{Table: "idempotency_keys", Action: Erase},
	{Table: "user_templates", Action: Erase},

	// ---- Moderation. The *records* outlive the account because safety and
	// accountability reviews depend on them, but the identity of the person
	// who filed a report or took a moderation action is personal data and is
	// severed. Both columns are already nullable and declared
	// ON DELETE SET NULL, so this states in policy what the schema enforces.
	{Table: "reports", Action: Anonymise, Column: "reporter_id",
		Reason: "the report itself is a safety record about third-party content and " +
			"must survive; the reporter's identity is not required to act on it"},
	{Table: "moderation_cases", Action: Anonymise, Column: "actor",
		Reason: "moderation decisions must remain reviewable for accountability; " +
			"the individual moderator's identity is not part of that record"},
	{Table: "content_moderation_history", Action: Anonymise, Column: "actor",
		Reason: "audit trail of content status changes, retained for editorial " +
			"accountability with the acting individual detached"},

	// ---- Telemetry. Detached rather than deleted: the rows are needed for
	// aggregate service-quality reporting and carry no identity once the user
	// reference is cleared.
	{Table: "audio_qoe_metrics", Action: Anonymise,
		Reason: "aggregate quality-of-experience reporting; no identity remains once detached"},
	{Table: "audio_events", Action: Anonymise,
		Reason: "aggregate analytics; user reference already nullable"},
	{Table: "signed_urls", Action: Anonymise,
		Reason: "short-lived access audit; expires on its own"},

	// ---- Retained. Each states the obligation it satisfies.
	{Table: "subscriptions", Action: Retain,
		Reason: "financial record required for tax and accounting obligations; " +
			"retained against the tombstoned id, not the person"},
	{Table: "consent_records", Action: Retain,
		Reason: "proof that consent was obtained and later withdrawn; erasing it " +
			"would destroy the evidence that the deletion itself was lawful"},
}

// Validate checks the policy table is internally coherent.
//
// Called from a test rather than at runtime, because a malformed policy is a
// programming error that should never reach production.
func Validate() error {
	seen := map[string]bool{}
	for _, p := range Policies {
		if p.Table == "" {
			return fmt.Errorf("policy with empty table name")
		}
		if seen[p.Table] {
			return fmt.Errorf("duplicate policy for table %q", p.Table)
		}
		seen[p.Table] = true

		// Retention and anonymisation must be justified in writing. Erasure is
		// the default and needs no defence.
		if p.Action != Erase && p.Reason == "" {
			return fmt.Errorf("table %q has action %s with no stated reason", p.Table, p.Action)
		}
	}
	return nil
}

// RetainedCategories lists what deletion deliberately keeps, for the response
// shown to the user and for the deletion record.
//
// Telling someone what is retained, and why, is part of honouring the request
// rather than a footnote to it.
func RetainedCategories() []string {
	var out []string
	for _, p := range Policies {
		if p.Action == Retain {
			out = append(out, p.Table+": "+p.Reason)
		}
	}
	return out
}
