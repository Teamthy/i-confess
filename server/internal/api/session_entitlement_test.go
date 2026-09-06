package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Teamthy/i-confess/internal/engine"
	"github.com/Teamthy/i-confess/internal/models"
)

// The property under test: session length is a plan capability, and the server
// decides it. Hiding the control in one handler is not a control — PHASE 13
// found that the ad-hoc path returned 402 while the schedule path built a free
// user a three-hour session, because the check lived in a handler rather than
// in the engine.

func jsonBody(t *testing.T, res *http.Response) map[string]any {
	t.Helper()
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var top map[string]any
	if err := json.Unmarshal(raw, &top); err != nil {
		t.Fatalf("body is not JSON: %s", string(raw))
	}
	if d, ok := top["data"].(map[string]any); ok {
		return d
	}
	return top
}

func TestFreeUserCannotExceedPlanLengthThroughASchedule(t *testing.T) {
	f := newAudioFixture(t)
	ctx := context.Background()

	// A schedule saved at three hours — twelve times the free limit of 900s.
	// It is written through the store rather than the API because the API now
	// refuses to save it, and the case that matters is the one where it
	// already exists: a user who saved it while premium and has since
	// downgraded.
	sched := &models.Schedule{
		UserID: userIDForEmail(t, f.h, "free@test.com"), Label: "long", Time: "06:00",
		DaysOfWeek: []int{1}, Timezone: "UTC", DurationSeconds: 10800,
		VoiceID: f.voiceStd, CategoryIDs: []string{f.catID}, Enabled: true,
	}
	if err := f.h.sched.Create(ctx, sched); err != nil {
		t.Fatalf("store schedule: %v", err)
	}

	req, _ := http.NewRequest(http.MethodPost, f.srv.URL+"/schedules/"+sched.ID+"/start", nil)
	req.Header.Set("Authorization", "Bearer "+f.freeTok)
	res, err := f.srv.Client().Do(req)
	if err != nil {
		t.Fatalf("trigger: %v", err)
	}

	if res.StatusCode != http.StatusPaymentRequired {
		body := jsonBody(t, res)
		t.Fatalf("triggering a 3h schedule as a free user returned %d, want 402: %v", res.StatusCode, body)
	}
	body := jsonBody(t, res)
	if body["reason"] != "session_duration_exceeds_plan_limit" {
		t.Errorf("reason = %v, want session_duration_exceeds_plan_limit", body["reason"])
	}
}

func TestCreateScheduleRefusesAnOverPlanDuration(t *testing.T) {
	f := newAudioFixture(t)

	body := fmt.Sprintf(
		`{"label":"long","category_ids":[%q],"duration_seconds":10800,"time":"06:00","timezone":"UTC","days_of_week":[1],"voice_id":%q}`,
		f.catID, f.voiceStd)
	req, _ := http.NewRequest(http.MethodPost, f.srv.URL+"/schedules", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+f.freeTok)
	req.Header.Set("Content-Type", "application/json")

	res, err := f.srv.Client().Do(req)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if res.StatusCode != http.StatusPaymentRequired {
		t.Errorf("saving a 3h schedule as a free user returned %d, want 402", res.StatusCode)
	}
	_ = jsonBody(t, res)
}

func TestScheduleWithinThePlanStillBuilds(t *testing.T) {
	f := newAudioFixture(t)

	// Guards the two tests above from passing because the endpoint now refuses
	// everything.
	body := fmt.Sprintf(
		`{"label":"ok","category_ids":[%q],"duration_seconds":900,"time":"06:00","timezone":"UTC","days_of_week":[1],"voice_id":%q}`,
		f.catID, f.voiceStd)
	req, _ := http.NewRequest(http.MethodPost, f.srv.URL+"/schedules", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+f.freeTok)
	req.Header.Set("Content-Type", "application/json")
	res, err := f.srv.Client().Do(req)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("saving a 15m schedule returned %d, want 201: %v", res.StatusCode, jsonBody(t, res))
	}
	created := jsonBody(t, res)
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatalf("no schedule id: %v", created)
	}

	req2, _ := http.NewRequest(http.MethodPost, f.srv.URL+"/schedules/"+id+"/start", nil)
	req2.Header.Set("Authorization", "Bearer "+f.freeTok)
	res2, err := f.srv.Client().Do(req2)
	if err != nil {
		t.Fatalf("trigger: %v", err)
	}
	if res2.StatusCode != http.StatusCreated {
		t.Fatalf("triggering a 15m schedule returned %d, want 201: %v", res2.StatusCode, jsonBody(t, res2))
	}
	_ = jsonBody(t, res2)
}

func TestEngineRefusesDurationsOutsideItsBounds(t *testing.T) {
	// The engine is the last line: whatever a caller forgets, Build refuses.
	cases := []struct {
		name    string
		req     engine.Request
		wantErr error
	}{
		{"too short", engine.Request{DurationSeconds: 30}, engine.ErrDurationTooShort},
		{"too long", engine.Request{DurationSeconds: 4 * 3600}, engine.ErrDurationTooLong},
		{"over plan cap", engine.Request{DurationSeconds: 3600, MaxDurationSeconds: 900}, engine.ErrDurationExceedsPlan},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newAudioFixture(t)
			tc.req.UserID = userIDForEmail(t, f.h, "free@test.com")
			tc.req.CategoryIDs = []string{f.catID}
			tc.req.VoiceID = f.voiceStd

			_, err := f.h.engn.Build(context.Background(), tc.req)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("Build returned %v, want %v", err, tc.wantErr)
			}
		})
	}
}
