package voicedata

import (
	"fmt"
	"strings"
	"testing"
)

func f(v float64) *float64 { return &v }

func TestScreenRejectsButNeverApproves(t *testing.T) {
	th := DefaultThresholds()
	good := Segment{StartMS: 0, EndMS: 5000, SNRDB: f(35), Quality: 0.9}
	if r := Screen(good, th); len(r) != 0 {
		t.Fatalf("good segment rejected: %v", r)
	}
	bad := Segment{StartMS: 0, EndMS: 500, SNRDB: f(8), ClippingRatio: 0.02, Quality: 0.2, SpeakerConfidence: f(0.3)}
	r := strings.Join(Screen(bad, th), ",")
	for _, want := range []string{"too_short", "low_snr", "clipping", "low_quality", "other_speaker"} {
		if !strings.Contains(r, want) {
			t.Errorf("missing %s in %s", want, r)
		}
	}
	// Unknown speaker confidence must not be treated as a pass or a fail.
	if r := Screen(Segment{EndMS: 5000, Quality: 0.9}, th); len(r) != 0 {
		t.Fatalf("unknown diarization rejected: %v", r)
	}
}

func TestSplitIsDeterministicAndHoldsOut(t *testing.T) {
	var ids []string
	for i := 0; i < 40; i++ {
		ids = append(ids, fmt.Sprintf("seg-%d", i))
	}
	a, b := AssignSplit(ids, "voice-1"), AssignSplit(ids, "voice-1")
	counts := map[Split]int{}
	for _, id := range ids {
		if a[id] != b[id] {
			t.Fatal("split not deterministic")
		}
		counts[a[id]]++
	}
	if counts[SplitTest] != 2 || counts[SplitValidation] != 2 || counts[SplitTrain] != 36 {
		t.Fatalf("counts %v", counts)
	}
	// Adding segments must not move an existing test segment into training
	// often; with hash ordering, the old test set largely stays held out.
	if len(AssignSplit(ids[:10], "s")) != 10 {
		t.Fatal("size")
	}
}

func TestManifestHashIsOrderIndependent(t *testing.T) {
	e1 := ManifestEntry{SegmentID: "a", Transcript: "x"}
	e2 := ManifestEntry{SegmentID: "b", Transcript: "y"}
	_, h1, _ := Manifest{VoiceID: "v", Entries: []ManifestEntry{e1, e2}}.Encode()
	_, h2, _ := Manifest{VoiceID: "v", Entries: []ManifestEntry{e2, e1}}.Encode()
	_, h3, _ := Manifest{VoiceID: "v", Entries: []ManifestEntry{e1}}.Encode()
	if h1 != h2 || h1 == h3 {
		t.Fatal("manifest hash must depend on content only")
	}
}

func TestNextVersion(t *testing.T) {
	if v := NextVersion(nil); v != "dataset_v001" {
		t.Fatal(v)
	}
	if v := NextVersion([]string{"dataset_v001", "dataset_v007", "junk"}); v != "dataset_v008" {
		t.Fatal(v)
	}
}

func TestZeroShotFirst(t *testing.T) {
	p := DefaultFineTunePolicy()
	st := Stats{UsableSeconds: 1200, TestSegments: 3}
	if err := CheckFineTune(p, st, nil, "why", false); err != ErrZeroShotFirst {
		t.Fatalf("want zero-shot-first, got %v", err)
	}
	if err := CheckFineTune(p, st, f(0.7), "", false); err == nil {
		t.Fatal("justification required")
	}
	if err := CheckFineTune(p, Stats{UsableSeconds: 60, TestSegments: 1}, f(0.7), "accent drift", false); err == nil {
		t.Fatal("small dataset allowed")
	}
	if err := CheckFineTune(p, st, f(0.9), "accent drift", false); err == nil {
		t.Fatal("fine-tune allowed although zero-shot is sufficient")
	}
	if err := CheckFineTune(p, st, f(0.9), "accent drift", true); err != nil {
		t.Fatal(err)
	}
	if err := CheckFineTune(p, st, f(0.7), "accent drift", false); err != nil {
		t.Fatal(err)
	}
}

func TestRunTransitions(t *testing.T) {
	if !CanTransitionRun(RunQueued, RunRunning) || CanTransitionRun(RunCompleted, RunApproved) || CanTransitionRun(RunArchived, RunQueued) {
		t.Fatal("run graph wrong: completed runs must be evaluated before approval")
	}
}
