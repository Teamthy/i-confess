package workers

import (
	"context"
	"errors"
	"testing"

	"github.com/Teamthy/i-confess/internal/jobs"
	"github.com/Teamthy/i-confess/internal/push"
)

type fakeGenerator struct {
	got  GenerationRequest
	err  error
	call int
}

func (f *fakeGenerator) Generate(_ context.Context, req GenerationRequest) error {
	f.call++
	f.got = req
	return f.err
}

type fakeSender struct {
	got  push.Notification
	err  error
	call int
}

func (f *fakeSender) Name() string { return "fake" }
func (f *fakeSender) Send(_ context.Context, n push.Notification) error {
	f.call++
	f.got = n
	return f.err
}

type fakeProcessor struct {
	got  string
	err  error
	call int
}

func (f *fakeProcessor) Process(_ context.Context, assetID string) error {
	f.call++
	f.got = assetID
	return f.err
}

func handler(t *testing.T, s Services, typ string) jobs.Handler {
	t.Helper()
	q := jobs.NewMemoryQueue()
	Register(q, s)
	h, ok := q.HandlerFor(typ)
	if !ok {
		t.Fatalf("no handler registered for %q", typ)
	}
	return h
}

func TestGenerateHandlerPassesTheRequestThrough(t *testing.T) {
	g := &fakeGenerator{}
	h := handler(t, Services{Generate: g}, TypeAudioGenerate)

	err := h(context.Background(), map[string]any{
		"confession_id": "c-1",
		"variant_id":    "v-1",
		"voice_id":      "voice-1",
		"language":      "yo",
		"actor":         "admin-1",
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	want := GenerationRequest{
		ConfessionID: "c-1", VariantID: "v-1", VoiceID: "voice-1",
		Language: "yo", Actor: "admin-1",
	}
	if g.got != want {
		t.Errorf("generator got %+v, want %+v", g.got, want)
	}
}

func TestGenerateHandlerDefaultsLanguage(t *testing.T) {
	g := &fakeGenerator{}
	h := handler(t, Services{Generate: g}, TypeAudioGenerate)
	if err := h(context.Background(), map[string]any{"confession_id": "c", "voice_id": "v"}); err != nil {
		t.Fatal(err)
	}
	if g.got.Language != "en" {
		t.Errorf("language = %q, want the en default", g.got.Language)
	}
}

// TestMissingPayloadFieldsArePermanent is the distinction that makes the retry
// machinery behave. A payload missing a required field will not gain one by
// being retried, so it must park on the first attempt instead of consuming the
// whole backoff schedule first.
func TestMissingPayloadFieldsArePermanent(t *testing.T) {
	cases := []struct {
		typ     string
		payload map[string]any
	}{
		{TypeAudioGenerate, map[string]any{"voice_id": "v"}},
		{TypeAudioGenerate, map[string]any{"confession_id": "c"}},
		{TypeAudioGenerate, map[string]any{"confession_id": "  ", "voice_id": "v"}},
		{TypeNotificationSend, map[string]any{"title": "t"}},
		{TypeNotificationSend, map[string]any{"token": "tk"}},
		{TypeAudioProcess, map[string]any{}},
	}
	for _, c := range cases {
		s := Services{Generate: &fakeGenerator{}, Notify: &fakeSender{}, Process: &fakeProcessor{}}
		err := handler(t, s, c.typ)(context.Background(), c.payload)
		if err == nil {
			t.Errorf("%s with payload %v: no error", c.typ, c.payload)
			continue
		}
		if !jobs.IsPermanent(err) {
			t.Errorf("%s with payload %v: error %v is retryable; it can never succeed", c.typ, c.payload, err)
		}
	}
}

// TestUnconfiguredHandlerParksRatherThanSucceeding covers the exact defect this
// package replaces. The handlers it supersedes logged a line and returned nil,
// so the queue recorded a completed job for work that never happened - a
// generation "succeeded" with no audio, a notification "sent" with nothing sent.
func TestUnconfiguredHandlerParksRatherThanSucceeding(t *testing.T) {
	// Register leaves these types uninstalled, so the handlers are reached only
	// if something calls them directly. The guard is still worth having: a nil
	// collaborator must park the job, never report success for work that did
	// not happen.
	payload := map[string]any{
		"confession_id": "c", "voice_id": "v",
		"token": "tk", "title": "t", "audio_asset_id": "a",
	}
	var empty Services
	ctx := context.Background()
	for _, c := range []struct {
		typ string
		err error
	}{
		{TypeAudioGenerate, empty.handleGenerate(ctx, payload)},
		{TypeNotificationSend, empty.handleNotify(ctx, payload)},
		{TypeAudioProcess, empty.handleProcess(ctx, payload)},
	} {
		if c.err == nil {
			t.Errorf("%s with no collaborator configured returned nil; the queue would mark it completed", c.typ)
			continue
		}
		if !jobs.IsPermanent(c.err) {
			t.Errorf("%s with no collaborator: %v is retryable, but configuring one is not a retry", c.typ, c.err)
		}
	}
}

// TestTransportFaultIsRetryableButRejectedTokenIsNot is the other half: a
// permanent classification for everything would strand notifications that failed
// because a provider was briefly down.
func TestTransportFaultIsRetryableButRejectedTokenIsNot(t *testing.T) {
	retryable := &fakeSender{err: push.ErrRetryable}
	err := handler(t, Services{Notify: retryable}, TypeNotificationSend)(
		context.Background(), map[string]any{"token": "tk", "title": "t"})
	if err == nil {
		t.Fatal("no error from a failing sender")
	}
	if jobs.IsPermanent(err) {
		t.Error("a transport fault was marked permanent; it should be retried with backoff")
	}

	rejected := &fakeSender{err: errors.New("device token is not registered")}
	err = handler(t, Services{Notify: rejected}, TypeNotificationSend)(
		context.Background(), map[string]any{"token": "tk", "title": "t"})
	if err == nil {
		t.Fatal("no error from a rejecting sender")
	}
	if !jobs.IsPermanent(err) {
		t.Errorf("a rejected token was left retryable: %v", err)
	}
}

func TestNotifyBuildsTheNotification(t *testing.T) {
	s := &fakeSender{}
	h := handler(t, Services{Notify: s}, TypeNotificationSend)
	if err := h(context.Background(), map[string]any{
		"token": "tk-1", "title": "Time to pray", "body": "Your 7am session",
		"platform": "ios",
		"data":     map[string]any{"session_id": "s-9", "n": 3},
	}); err != nil {
		t.Fatal(err)
	}
	if s.got.Token != "tk-1" || s.got.Title != "Time to pray" || s.got.Body != "Your 7am session" {
		t.Errorf("notification = %+v", s.got)
	}
	if s.got.Platform != push.Platform("ios") {
		t.Errorf("platform = %q", s.got.Platform)
	}
	if s.got.Data["session_id"] != "s-9" {
		t.Errorf("deep-link data lost: %v", s.got.Data)
	}
	if _, ok := s.got.Data["n"]; ok {
		t.Error("a non-string data value was forwarded; push payloads are string maps")
	}
}

func TestProcessHandlerPassesTheAssetID(t *testing.T) {
	p := &fakeProcessor{}
	h := handler(t, Services{Process: p}, TypeAudioProcess)
	if err := h(context.Background(), map[string]any{"audio_asset_id": "asset-7"}); err != nil {
		t.Fatal(err)
	}
	if p.got != "asset-7" {
		t.Errorf("processor got %q, want asset-7", p.got)
	}
}

func TestHandlerErrorsPropagateSoTheQueueCanRetry(t *testing.T) {
	boom := errors.New("provider returned 503")
	err := handler(t, Services{Generate: &fakeGenerator{err: boom}}, TypeAudioGenerate)(
		context.Background(), map[string]any{"confession_id": "c", "voice_id": "v"})
	if !errors.Is(err, boom) {
		t.Errorf("handler error = %v, want the underlying cause preserved", err)
	}
	if jobs.IsPermanent(err) {
		t.Error("a plain provider error was marked permanent")
	}
}

// TestOnlyConfiguredTypesAreRegistered is the operational consequence of a
// Services with gaps: the missing types are left unregistered, so the queue
// parks their jobs with "no handler registered" - which names the real problem -
// rather than accepting work it cannot do.
func TestOnlyConfiguredTypesAreRegistered(t *testing.T) {
	q := jobs.NewMemoryQueue()
	got := Register(q, Services{Notify: &fakeSender{}})
	if len(got) != 1 || got[0] != TypeNotificationSend {
		t.Errorf("registered %v, want only %q", got, TypeNotificationSend)
	}
	for _, typ := range []string{TypeAudioGenerate, TypeAudioProcess} {
		if _, ok := q.HandlerFor(typ); ok {
			t.Errorf("%q was registered with no collaborator to back it", typ)
		}
	}

	full := jobs.NewMemoryQueue()
	all := Register(full, Services{
		Generate: &fakeGenerator{}, Notify: &fakeSender{}, Process: &fakeProcessor{},
	})
	if len(all) != len(AllTypes()) {
		t.Errorf("a fully configured Services registered %v, want all of %v", all, AllTypes())
	}
}
