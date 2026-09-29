package voiceengine

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/voicegov"
)

// ---------------------------------------------------------------- markup

func TestParseMarkup(t *testing.T) {
	segs, err := ParseMarkup("Be still. {pause 700}Know that {emph}I am God{/emph}. {say Habakkuk as ha-BAK-kuk} wrote.")
	if err != nil {
		t.Fatal(err)
	}
	if PlainText(segs) != "Be still. Know that I am God. Habakkuk wrote." {
		t.Fatalf("plain = %q", PlainText(segs))
	}
	var sawPause, sawEmph, sawSay bool
	for _, s := range segs {
		sawPause = sawPause || s.PauseMS == 700
		sawEmph = sawEmph || (s.Emphasis && s.Text == "I am God")
		sawSay = sawSay || s.Say == "ha-BAK-kuk"
	}
	if !sawPause || !sawEmph || !sawSay {
		t.Fatalf("%+v", segs)
	}
}

func TestParseMarkupRejectsMalformed(t *testing.T) {
	for _, bad := range []string{"{pause abc}", "{pause 999999}", "{emph}open", "close{/emph}", "{say Habakkuk}", "{speed 9}x{/speed}", "{pitch 40}x{/pitch}"} {
		if _, err := ParseMarkup(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	// Unknown braces are text, not errors.
	if segs, err := ParseMarkup("Psalm {23}"); err != nil || PlainText(segs) != "Psalm {23}" {
		t.Fatalf("%v %v", segs, err)
	}
}

func TestRenderWithoutMarkupSupportNeverLeaksTags(t *testing.T) {
	segs, _ := ParseMarkup("Pray {pause 1000} with {emph}faith{/emph} like {say Habakkuk as ha-BAK-kuk}.")
	chunks := Render(segs, ProviderCapabilities{}, ProsodyProfile{PauseMultiplier: 1.5})
	var all strings.Builder
	for _, c := range chunks {
		all.WriteString(c.Text + " ")
		if c.Emphasis || c.Phonemes != "" {
			t.Fatalf("unsupported feature passed through: %+v", c)
		}
	}
	if strings.ContainsAny(all.String(), "{}") {
		t.Fatalf("tag leaked: %q", all.String())
	}
	if !strings.Contains(all.String(), "ha bak kuk") {
		t.Fatalf("respelling not spoken: %q", all.String())
	}
	if chunks[0].SilenceAfterMS != 1500 {
		t.Fatalf("pause multiplier not applied: %+v", chunks[0])
	}
}

func TestRenderWithPhonemeSupport(t *testing.T) {
	segs, _ := ParseMarkup("{say Gethsemane as /ɡɛθˈsɛməni/} garden")
	chunks := Render(segs, ProviderCapabilities{Phonemes: true, Emphasis: true}, ProsodyProfile{})
	if chunks[0].Phonemes != "/ɡɛθˈsɛməni/" || chunks[0].Text != "Gethsemane" {
		t.Fatalf("%+v", chunks)
	}
}

func TestRenderSplitsLongText(t *testing.T) {
	long := strings.Repeat("The Lord is my shepherd. ", 40)
	segs, _ := ParseMarkup(long)
	chunks := Render(segs, ProviderCapabilities{MaxChunkChars: 100}, ProsodyProfile{})
	if len(chunks) < 5 {
		t.Fatalf("expected splitting, got %d chunks", len(chunks))
	}
	for _, c := range chunks {
		if len(c.Text) > 100 {
			t.Fatalf("chunk too long: %d", len(c.Text))
		}
	}
}

// ---------------------------------------------------------------- dictionary

func testDict() *Dictionary {
	return NewDictionary([]PronunciationEntry{
		{Term: "Habakkuk", Locale: "", Respelling: "ha-BAK-kuk"},
		{Term: "Nebuchadnezzar", Locale: "en", Respelling: "ne-bu-kad-NEZ-ar", IPA: "nɛbjʊkədˈnɛzər"},
		{Term: "Onitsha", Locale: "en-NG", Respelling: "o-NI-cha", Category: "nigerian_place"},
		{Term: "wetin", Locale: "en-NG-PIDGIN", Respelling: "WEH-tin"},
		{Term: "Gethsemane", Locale: "en", Respelling: "geth-SEM-a-nee", ProviderOverrides: map[Engine]string{EngineVoxCPM: "geth sem uh nee"}},
	})
}

func TestDictionaryLocaleSpecificity(t *testing.T) {
	d := testDict()
	if _, ok := d.Lookup("Onitsha", "en-NG"); !ok {
		t.Fatal("en-NG entry not found")
	}
	if _, ok := d.Lookup("Onitsha", "en-US"); ok {
		t.Fatal("en-NG entry leaked into en-US")
	}
	// Nigerian English and Nigerian Pidgin are distinct varieties.
	if _, ok := d.Lookup("wetin", "en-NG"); ok {
		t.Fatal("Pidgin entry applied to Nigerian English")
	}
	if _, ok := d.Lookup("Nebuchadnezzar", "en-NG-PIDGIN"); ok {
		t.Fatal("bare-language entry applied to Pidgin implicitly")
	}
	if _, ok := d.Lookup("habakkuk", "en-NG"); !ok {
		t.Fatal("universal entry, case-insensitive")
	}
}

func TestDictionaryApply(t *testing.T) {
	d := testDict()
	segs, _ := ParseMarkup("Nebuchadnezzar met Habakkuk in Gethsemane.")
	out := d.Apply(segs, "en-NG", EngineCosyVoice, ProviderCapabilities{Phonemes: true})
	says := map[string]string{}
	for _, s := range out {
		if s.Say != "" {
			says[s.Text] = s.Say
		}
	}
	if says["Nebuchadnezzar"] != "/nɛbjʊkədˈnɛzər/" || says["Habakkuk"] != "ha-BAK-kuk" || says["Gethsemane"] != "geth-SEM-a-nee" {
		t.Fatalf("%v", says)
	}
	out = d.Apply(segs, "en-NG", EngineVoxCPM, ProviderCapabilities{})
	for _, s := range out {
		if s.Text == "Gethsemane" && s.Say != "geth sem uh nee" {
			t.Fatalf("provider override ignored: %+v", s)
		}
	}
	if PlainText(out) != PlainText(segs) {
		t.Fatal("apply changed the words")
	}
}

// ---------------------------------------------------------------- hash

func TestContentHashStableAndSensitive(t *testing.T) {
	base := HashInput{VoiceID: "v", ModelID: "m", Text: "Let us  pray.", Language: "en", Style: "prayer", Speed: 0.9}
	a := ContentHash(base)
	b := base
	b.Text = "Let us pray."
	b.Language = "EN"
	if ContentHash(b) != a {
		t.Fatal("whitespace/case changed the hash")
	}
	for _, mut := range []func(*HashInput){
		func(h *HashInput) { h.ModelID = "m2" },
		func(h *HashInput) { h.Style = "reflection" },
		func(h *HashInput) { h.Speed = 1 },
		func(h *HashInput) { h.ReferenceID = "r2" },
		func(h *HashInput) { h.AudioVersion = "m2" },
		func(h *HashInput) { h.Text = "Let us sing." },
	} {
		c := base
		mut(&c)
		if ContentHash(c) == a {
			t.Fatalf("mutation did not change hash: %+v", c)
		}
	}
}

// ---------------------------------------------------------------- registry

func TestPromoteRetiresPreviousAndRollbackRestores(t *testing.T) {
	t0 := time.Now()
	models := []Model{
		{ID: "v1", VoiceID: "x", Status: ModelProduction, LicenseReviewed: true, PromotedAt: t0},
		{ID: "v2", VoiceID: "x", Status: ModelApproved, LicenseReviewed: true},
		{ID: "other", VoiceID: "y", Status: ModelProduction, LicenseReviewed: true},
	}
	ch, err := Promote(models, "v2")
	if err != nil {
		t.Fatal(err)
	}
	if ch["v2"] != ModelProduction || ch["v1"] != ModelRetired || len(ch) != 2 {
		t.Fatalf("%v", ch)
	}
	models[0].Status, models[1].Status = ModelRetired, ModelProduction
	ch, err = Rollback(models, "x")
	if err != nil || ch["v1"] != ModelProduction || ch["v2"] != ModelRetired {
		t.Fatalf("%v %v", ch, err)
	}
}

func TestPromoteGuards(t *testing.T) {
	models := []Model{
		{ID: "cand", VoiceID: "x", Status: ModelCandidate, LicenseReviewed: true},
		{ID: "nolic", VoiceID: "x", Status: ModelApproved},
		{ID: "ft", VoiceID: "x", Status: ModelApproved, LicenseReviewed: true, Mode: "fine_tuned"},
	}
	for _, id := range []string{"cand", "nolic", "ft", "missing"} {
		if _, err := Promote(models, id); err == nil {
			t.Errorf("promoted %s", id)
		}
	}
	if _, err := Rollback(models, "x"); err == nil {
		t.Error("rollback without retired model")
	}
}

func TestCandidatesNeverCrossVoicesAndRespectApproval(t *testing.T) {
	models := []Model{
		{ID: "p", VoiceID: "x", Engine: EngineCosyVoice, Status: ModelProduction, ICFVoiceScore: 0.9, LicenseReviewed: true},
		{ID: "ok", VoiceID: "x", Engine: EngineGPTSoVITS, Status: ModelApproved, FallbackApproved: true, LicenseReviewed: true, ICFVoiceScore: 0.87},
		{ID: "notfb", VoiceID: "x", Engine: EngineVoxCPM, Status: ModelApproved, LicenseReviewed: true, ICFVoiceScore: 0.95},
		{ID: "low", VoiceID: "x", Engine: EngineVoxCPM, Status: ModelApproved, FallbackApproved: true, LicenseReviewed: true, ICFVoiceScore: 0.6},
		{ID: "othervoice", VoiceID: "y", Engine: EngineVoxCPM, Status: ModelApproved, FallbackApproved: true, LicenseReviewed: true, ICFVoiceScore: 0.99},
	}
	p, fb, err := Candidates(models, "x", FallbackPolicy{Enabled: true, MinScore: 0.8, MaxScoreDrop: 0.05})
	if err != nil || p.ID != "p" || len(fb) != 1 || fb[0].ID != "ok" {
		t.Fatalf("%v %v %v", p, fb, err)
	}
	_, fb, _ = Candidates(models, "x", FallbackPolicy{})
	if len(fb) != 0 {
		t.Fatal("fallback used while disabled")
	}
	if _, _, err := Candidates(models, "z", FallbackPolicy{}); err == nil {
		t.Fatal("voice without production model")
	}
}

func TestRegistryRefusesDevEnginesInProduction(t *testing.T) {
	r := NewRegistry(true)
	if err := r.Register(EngineFake, &fakeProvider{}); err == nil {
		t.Fatal("fake engine allowed in production")
	}
	if err := r.Register(EngineVoiceStudio, &fakeProvider{}); err == nil {
		t.Fatal("VoiceStudio allowed in production")
	}
}

func TestQueueFor(t *testing.T) {
	if QueueFor(PriorityInteractive) != QueueTTSHigh || QueueFor(PriorityBatch) != QueueTTSLow || QueueFor("") != QueueTTSNormal {
		t.Fatal("queue routing")
	}
}

func TestSelectReference(t *testing.T) {
	refs := []Reference{
		{ID: "n", VoiceID: "x", Style: "neutral", RightsOK: true, Quality: 0.8},
		{ID: "p1", VoiceID: "x", Style: "prayer", RightsOK: true, Quality: 0.7},
		{ID: "p2", VoiceID: "x", Style: "prayer", RightsOK: true, Quality: 0.9},
		{ID: "p3", VoiceID: "x", Style: "prayer", RightsOK: false, Quality: 0.99},
		{ID: "y", VoiceID: "y", Style: "reflection", RightsOK: true, Quality: 1},
	}
	if r, _ := SelectReference(refs, "x", "prayer"); r.ID != "p2" {
		t.Fatalf("got %s", r.ID)
	}
	if r, _ := SelectReference(refs, "x", "reflection"); r.ID != "n" {
		t.Fatalf("neutral fallback, got %s", r.ID)
	}
	if _, err := SelectReference(refs, "z", "prayer"); err == nil {
		t.Fatal("reference from nowhere")
	}
}

// ---------------------------------------------------------------- orchestrator

type fakeProvider struct {
	caps  ProviderCapabilities
	err   error
	calls int
	last  GenerateRequest
}

func (f *fakeProvider) Generate(_ context.Context, r GenerateRequest) (*GenerateResult, error) {
	f.calls++
	f.last = r
	if f.err != nil {
		return nil, f.err
	}
	return &GenerateResult{Audio: []byte("RIFF"), ModelID: r.ModelID, Engine: f.caps.Engine}, nil
}
func (f *fakeProvider) Clone(context.Context, CloneRequest) (*CloneResult, error) {
	return &CloneResult{SpeakerHandle: "h"}, nil
}
func (f *fakeProvider) Health(context.Context) error                      { return f.err }
func (f *fakeProvider) Capabilities(context.Context) ProviderCapabilities { return f.caps }

func approvedGrant() *voicegov.Grant {
	caps := map[voicegov.Capability]bool{}
	for _, c := range voicegov.AllCapabilities {
		caps[c] = true
	}
	return &voicegov.Grant{VoiceID: "x", Status: voicegov.StatusApproved, Capabilities: caps}
}

func setup() (*Orchestrator, *fakeProvider, *fakeProvider, []Model, []Reference) {
	cosy := &fakeProvider{caps: ProviderCapabilities{Engine: EngineCosyVoice, Languages: []string{"en"}, SelfHosted: true}}
	sovits := &fakeProvider{caps: ProviderCapabilities{Engine: EngineGPTSoVITS, Languages: []string{"en"}, SelfHosted: true}}
	reg := NewRegistry(false)
	_ = reg.Register(EngineCosyVoice, cosy)
	_ = reg.Register(EngineGPTSoVITS, sovits)
	models := []Model{
		{ID: "m1", VoiceID: "x", Engine: EngineCosyVoice, Status: ModelProduction, ICFVoiceScore: 0.9, LicenseReviewed: true},
		{ID: "m2", VoiceID: "x", Engine: EngineGPTSoVITS, Status: ModelApproved, FallbackApproved: true, LicenseReviewed: true, ICFVoiceScore: 0.88},
	}
	refs := []Reference{{ID: "r", VoiceID: "x", Style: "reflection", RightsOK: true, Quality: 0.9}}
	o := &Orchestrator{Registry: reg, Dict: testDict(), Fallback: FallbackPolicy{Enabled: true, MinScore: 0.8}}
	return o, cosy, sovits, models, refs
}

func req() Request {
	return Request{VoiceID: "x", Markup: "Reflect on Habakkuk.", Language: "en", Locale: "en-NG", Style: "reflection", Purpose: voicegov.PurposeReflection}
}

func TestOrchestratorHappyPath(t *testing.T) {
	o, cosy, _, models, refs := setup()
	plan, res, err := o.Generate(context.Background(), approvedGrant(), models, refs, req())
	if err != nil || res == nil {
		t.Fatal(err)
	}
	if plan.FellBack || plan.Model.ID != "m1" || plan.Reference.ID != "r" || plan.ContentHash == "" {
		t.Fatalf("%+v", plan)
	}
	if cosy.last.Prosody.Name != "reflective" || !strings.Contains(cosy.last.Chunks[0].Text, "ha bak kuk") {
		t.Fatalf("%+v", cosy.last)
	}
}

// Revocation is checked on every call; no provider is ever reached.
func TestOrchestratorRefusesRevokedVoice(t *testing.T) {
	o, cosy, sovits, models, refs := setup()
	g := approvedGrant()
	g.Status = voicegov.StatusRevoked
	_, _, err := o.Generate(context.Background(), g, models, refs, req())
	var rd *ErrRightsDenied
	if !errors.As(err, &rd) || rd.Decision.Reason != voicegov.ReasonRevoked {
		t.Fatalf("%v", err)
	}
	if cosy.calls+sovits.calls != 0 {
		t.Fatal("provider reached after revocation")
	}
}

func TestOrchestratorFallsBackOnGPUFailureAndSaysSo(t *testing.T) {
	o, cosy, sovits, models, refs := setup()
	cosy.err = &Error{Class: ClassGPU, Msg: "CUDA OOM"}
	plan, res, err := o.Generate(context.Background(), approvedGrant(), models, refs, req())
	if err != nil || res == nil {
		t.Fatal(err)
	}
	if !plan.FellBack || plan.Model.ID != "m2" || !strings.Contains(plan.FallbackReason, "gpu") || sovits.calls != 1 {
		t.Fatalf("%+v", plan)
	}
}

func TestOrchestratorDoesNotFallBackOnContentFailure(t *testing.T) {
	o, cosy, sovits, models, refs := setup()
	cosy.err = &Error{Class: ClassContent, Msg: "text rejected"}
	if _, _, err := o.Generate(context.Background(), approvedGrant(), models, refs, req()); err == nil {
		t.Fatal("expected error")
	}
	if sovits.calls != 0 {
		t.Fatal("content failure retried on another engine")
	}
}

func TestOrchestratorThirdPartyEngineNeedsCapability(t *testing.T) {
	o, cosy, _, models, refs := setup()
	cosy.caps.SelfHosted = false
	g := approvedGrant()
	g.Capabilities[voicegov.CanUseThirdPartyInfra] = false
	_, _, err := o.Generate(context.Background(), g, models, refs, req())
	var rd *ErrRightsDenied
	if !errors.As(err, &rd) {
		t.Fatalf("%v", err)
	}
}

func TestOrchestratorRejectsBadMarkup(t *testing.T) {
	o, _, _, models, refs := setup()
	r := req()
	r.Markup = "{emph}unclosed"
	if _, _, err := o.Generate(context.Background(), approvedGrant(), models, refs, r); Classify(err) != ClassContent {
		t.Fatalf("%v", err)
	}
}

// ---------------------------------------------------------------- HTTP adapter

func TestHTTPProviderContract(t *testing.T) {
	var got GenerateRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(401)
			return
		}
		switch r.URL.Path {
		case "/v1/synthesize":
			_ = json.NewDecoder(r.Body).Decode(&got)
			if strings.Contains(got.Chunks[0].Text, "oom") {
				w.WriteHeader(503)
				_, _ = w.Write([]byte(`{"error":{"class":"gpu","message":"CUDA out of memory"}}`))
				return
			}
			w.Header().Set("Content-Type", "audio/wav")
			w.Header().Set("X-Sample-Rate", "24000")
			w.Header().Set("X-Duration-Ms", "1500")
			w.Header().Set("X-Engine-Version", "3.0.1")
			_, _ = w.Write([]byte("RIFFdata"))
		case "/v1/capabilities":
			_ = json.NewEncoder(w).Encode(ProviderCapabilities{Engine: "impostor", Languages: []string{"en"}, SelfHosted: true})
		case "/v1/health":
			w.WriteHeader(503)
		}
	}))
	defer srv.Close()

	p := NewCosyVoiceProvider(srv.URL, "tok")
	res, err := p.Generate(context.Background(), GenerateRequest{ModelID: "m", Chunks: []Chunk{{Text: "hi"}}, Prosody: ProsodyProfile{Speed: 0.9}, Style: "prayer"})
	if err != nil {
		t.Fatal(err)
	}
	if res.SampleRate != 24000 || res.DurationMS != 1500 || res.EngineVersion != "3.0.1" || string(res.Audio) != "RIFFdata" {
		t.Fatalf("%+v", res)
	}
	if got.Params["speed"] != 0.9 || got.Params["instruct_style"] != "prayer" {
		t.Fatalf("engine params not mapped: %v", got.Params)
	}
	_, err = p.Generate(context.Background(), GenerateRequest{Chunks: []Chunk{{Text: "oom"}}})
	if Classify(err) != ClassGPU || !Classify(err).Retryable() {
		t.Fatalf("%v", err)
	}
	if c := p.Capabilities(context.Background()); c.Engine != EngineCosyVoice {
		t.Fatalf("worker renamed engine: %s", c.Engine)
	}
	if p.Health(context.Background()) == nil {
		t.Fatal("unhealthy worker reported healthy")
	}
	bad := NewCosyVoiceProvider(srv.URL, "wrong")
	if _, err := bad.Generate(context.Background(), GenerateRequest{Chunks: []Chunk{{Text: "x"}}}); Classify(err) != ClassPermanent {
		t.Fatalf("401 should be permanent: %v", err)
	}
	down := NewCosyVoiceProvider("http://127.0.0.1:1", "")
	if c := down.Capabilities(context.Background()); c.Engine != EngineCosyVoice || !c.ZeroShot {
		t.Fatalf("fallback caps: %+v", c)
	}
}

func TestErrorClassRetryable(t *testing.T) {
	for c, want := range map[ErrorClass]bool{ClassTransient: true, ClassGPU: true, ClassStorage: true,
		ClassRights: false, ClassContent: false, ClassModel: false, ClassPermanent: false} {
		if c.Retryable() != want {
			t.Errorf("%s retryable=%v", c, !want)
		}
	}
}

// ---------------------------------------------------------------- resolve & safety

func TestResolveChecksRightsWithoutCallingProvider(t *testing.T) {
	o, cosy, _, models, refs := setup()
	plan, err := o.Resolve(context.Background(), approvedGrant(), models, refs, req())
	if err != nil || plan.ContentHash == "" || plan.Model.ID != "m1" {
		t.Fatalf("%+v %v", plan, err)
	}
	if cosy.calls != 0 {
		t.Fatal("Resolve reached the provider")
	}
	g := approvedGrant()
	g.Capabilities[voicegov.CanUseInReflections] = false
	var rd *ErrRightsDenied
	if _, err := o.Resolve(context.Background(), g, models, refs, req()); !errors.As(err, &rd) {
		t.Fatalf("%v", err)
	}
	// Same request, same hash: the cache key is deterministic.
	p2, _ := o.Resolve(context.Background(), approvedGrant(), models, refs, req())
	if p2.ContentHash != plan.ContentHash {
		t.Fatal("hash not deterministic")
	}
}

func TestValidateScript(t *testing.T) {
	ok := []string{
		"Take a moment and reflect on God's presence.",
		"Let us pray. Father, we thank you for this day.",
		"The Lord is my shepherd; I shall not want.",
	}
	for _, s := range ok {
		if err := ValidateScript(s, true); err != nil {
			t.Errorf("refused %q: %v", s, err)
		}
	}
	bad := map[string]string{
		"Pastor Adeyemi says you should vote for him.": "impersonation",
		"This is Pastor speaking to you directly.":     "impersonation",
		"I personally endorse this product.":           "impersonation",
		"Please send money to my account today.":       "solicitation",
		"Share your BVN so we can bless you.":          "solicitation",
		"Visit https://example.com for blessings.":     "contact_details",
		"Call me on +234 803 123 4567.":                "contact_details",
		strings.Repeat("a", MaxScriptChars+1):          "too_long",
		"   ":                                          "empty",
	}
	for s, code := range bad {
		err := ValidateScript(s, true)
		var v *SafetyViolation
		if !errors.As(err, &v) || v.Code != code {
			t.Errorf("%q: got %v, want %s", s[:min(len(s), 40)], err, code)
		}
	}
	// Editorial scripts may contain a URL (e.g. a church website in a devotional).
	if err := ValidateScript("Learn more at https://church.example.org", false); err != nil {
		t.Fatalf("editorial URL refused: %v", err)
	}
}
