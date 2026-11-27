package analytics

import (
	"reflect"
	"testing"
)

// TestAllowedEventsCoversEveryConstant ensures the allowlist is kept in sync
// with the event constants. If a new event is added, it must be registered
// here or it will be rejected by the store.
func TestAllowedEventsCoversEveryConstant(t *testing.T) {
	allowed := AllowedEvents()
	constants := []string{
		EventAppOpened,
		EventSessionCreated,
		EventSessionStarted,
		EventSessionPaused,
		EventSessionResumed,
		EventSessionCompleted,
		EventSessionSkipped,
		EventScheduleCreated,
		EventScheduleTriggered,
		EventDownloadCreated,
		EventDownloadRevoked,
		EventTrialDayViewed,
		EventTrialStarted,
		EventTrialDayCompleted,
		EventTrialConverted,
		EventTrialExpired,
		EventSubscriptionStarted,
		EventSubscriptionVerified,
		EventSubscriptionCancelled,
		EventSearchPerformed,
		EventCategoryViewed,
		EventVoicePreviewed,
		EventTemplateCreated,
		EventTemplateShared,
		EventPlaybackProgressSynced,
	}
	for _, name := range constants {
		if !allowed[name] {
			t.Errorf("event %q missing from allowlist", name)
		}
	}
}

// TestClientSubmittableExcludesServerOnlyEvents ensures events that are
// emitted only by the server (trial_day_completed) cannot be asserted by a
// client through the batch endpoint.
func TestClientSubmittableExcludesServerOnlyEvents(t *testing.T) {
	client := ClientSubmittableEvents()
	if client[EventTrialDayCompleted] {
		t.Errorf("trial_day_completed must not be client-submittable")
	}
	// Every other allowed event must be client-submittable
	for name := range AllowedEvents() {
		if name != EventTrialDayCompleted && !client[name] {
			t.Errorf("event %q should be client-submittable", name)
		}
	}
}

// TestIsAllowed validates the predicate form of the allowlist.
func TestIsAllowed(t *testing.T) {
	if !IsAllowed(EventAppOpened) {
		t.Errorf("IsAllowed(%q) = false, want true", EventAppOpened)
	}
	if !IsAllowed(EventSubscriptionCancelled) {
		t.Errorf("IsAllowed(%q) = false, want true", EventSubscriptionCancelled)
	}
	if IsAllowed("confession_body") {
		t.Errorf("IsAllowed(%q) = true, want false", "confession_body")
	}
	if IsAllowed("private_prayer_text") {
		t.Errorf("IsAllowed(%q) = true, want false", "private_prayer_text")
	}
	if IsAllowed("") {
		t.Errorf("IsAllowed(%q) = true, want false", "")
	}
	if IsAllowed("trial_day_completed_by_client") {
		t.Errorf("IsAllowed(%q) = true, want false", "trial_day_completed_by_client")
	}
}

// TestIsClientSubmittable validates the client-submittable predicate.
func TestIsClientSubmittable(t *testing.T) {
	if !IsClientSubmittable(EventAppOpened) {
		t.Errorf("IsClientSubmittable(%q) = false, want true", EventAppOpened)
	}
	if !IsClientSubmittable(EventSessionCompleted) {
		t.Errorf("IsClientSubmittable(%q) = false, want true", EventSessionCompleted)
	}
	if IsClientSubmittable(EventTrialDayCompleted) {
		t.Errorf("IsClientSubmittable(%q) = true, want false", EventTrialDayCompleted)
	}
	if IsClientSubmittable("unknown_event") {
		t.Errorf("IsClientSubmittable(%q) = true, want false", "unknown_event")
	}
}

// TestStripPII removes known private-text keys from event props.
func TestStripPII(t *testing.T) {
	// Nothing to strip
	props := map[string]any{"day": 3, "session_id": "abc"}
	out := StripPII(props)
	if !reflect.DeepEqual(out, props) {
		t.Errorf("StripPII(%v) = %v, want unchanged", props, out)
	}

	// Strip known PII keys
	props = map[string]any{
		"body":           "forgive me",
		"email":          "user@example.com",
		"scripture_text": "Psalm 23",
		"scripture":      "John 3:16",
		"confession":     "I have sinned",
		"note":           "personal note",
		"highlight":      "key verse",
		"prayer":         "please help",
		"day":            5,
		"session_id":     "abc",
	}
	out = StripPII(props)
	for _, key := range PIIPropKeys {
		if _, ok := out[key]; ok {
			t.Errorf("StripPII still contains key %q", key)
		}
	}
	if out["day"] != 5 {
		t.Errorf("StripPII removed non-PII key day: %v", out)
	}
	if out["session_id"] != "abc" {
		t.Errorf("StripPII removed non-PII key session_id: %v", out)
	}

	// Nil map is safe
	if StripPII(nil) != nil {
		t.Errorf("StripPII(nil) = %v, want nil", StripPII(nil))
	}
}
