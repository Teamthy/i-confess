package voiceeval

import (
	"strings"
	"testing"
)

func perfect() Metrics {
	m := Metrics{}
	for d := range DefaultWeights() {
		m[d] = 1
	}
	return m
}

func TestDefaultWeightsMatchSpec(t *testing.T) {
	w := DefaultWeights()
	total := 0.0
	for _, v := range w {
		total += v
	}
	if total < 0.999 || total > 1.001 {
		t.Fatalf("default weights sum to %v", total)
	}
	if w[SpeakerSimilarity] != 0.30 {
		t.Fatal("speaker similarity should default to 30%")
	}
}

func TestComputeWeightedMean(t *testing.T) {
	m := Metrics{SpeakerSimilarity: 0.8, Naturalness: 0.6}
	w := Weights{SpeakerSimilarity: 3, Naturalness: 1}
	s, err := Compute(m, w)
	if err != nil {
		t.Fatal(err)
	}
	if s.ICFVoiceScore != 0.75 || s.Coverage != 1 {
		t.Fatalf("%+v", s)
	}
	if !strings.Contains(s.Disclaimer, "not an objective") {
		t.Fatal("score must carry its disclaimer")
	}
}

func TestComputeReportsCoverage(t *testing.T) {
	s, err := Compute(Metrics{SpeakerSimilarity: 0.9}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.Coverage != 0.3 || len(s.Unmeasured) != 7 {
		t.Fatalf("%+v", s)
	}
}

func TestComputeRejectsBadInput(t *testing.T) {
	if _, err := Compute(Metrics{SpeakerSimilarity: 1.2}, nil); err == nil {
		t.Error("out-of-range metric accepted")
	}
	if _, err := Compute(perfect(), Weights{"vibes": 1}); err == nil {
		t.Error("unknown dimension accepted")
	}
	if _, err := Compute(perfect(), Weights{Naturalness: -1}); err == nil {
		t.Error("negative weight accepted")
	}
	if _, err := Compute(perfect(), Weights{Naturalness: 0}); err == nil {
		t.Error("zero-sum weights accepted")
	}
}

// No score, however high, promotes a model without a human.
func TestGateNeverAutoPasses(t *testing.T) {
	s, _ := Compute(perfect(), nil)
	r := Gate(GateInput{Candidate: s, RightsAllowed: true}, Thresholds{MinScore: 0.9})
	if r.Verdict != NeedsReview || len(r.Pending) != 2 {
		t.Fatalf("%+v", r)
	}
	r = Gate(GateInput{Candidate: s, RightsAllowed: true, HumanApproved: true, BlindEvalCompleted: true}, Thresholds{MinScore: 0.9})
	if r.Verdict != Pass {
		t.Fatalf("%+v", r)
	}
}

func TestGateFailsWithoutRights(t *testing.T) {
	s, _ := Compute(perfect(), nil)
	r := Gate(GateInput{Candidate: s, HumanApproved: true, BlindEvalCompleted: true}, Thresholds{})
	if r.Verdict != Fail {
		t.Fatalf("%+v", r)
	}
}

func TestGateThresholdsAndRegression(t *testing.T) {
	m := perfect()
	m[Pronunciation] = 0.7
	s, _ := Compute(m, nil)
	base, _ := Compute(perfect(), nil)
	r := Gate(GateInput{Candidate: s, Baseline: &base, RightsAllowed: true, HumanApproved: true, BlindEvalCompleted: true},
		Thresholds{Min: map[Dimension]float64{Pronunciation: 0.8, Intelligibility: 0.5}, MaxRegression: 0.05})
	if r.Verdict != Fail || len(r.Failures) != 3 {
		t.Fatalf("want pronunciation threshold + unmeasured intelligibility + regression, got %+v", r.Failures)
	}
}

func TestGateRejectsLowCoverage(t *testing.T) {
	s, _ := Compute(Metrics{SpeakerSimilarity: 1}, nil)
	if r := Gate(GateInput{Candidate: s, RightsAllowed: true, HumanApproved: true, BlindEvalCompleted: true}, Thresholds{}); r.Verdict != Fail {
		t.Fatalf("%+v", r)
	}
}

func TestBlindAndUnblind(t *testing.T) {
	clips := []Clip{{ID: "c1", Source: "REAL"}, {ID: "c2", Source: "model_a"}, {ID: "c3", Source: "model_b"}}
	b, err := Blind(clips)
	if err != nil {
		t.Fatal(err)
	}
	labels := map[string]bool{}
	for _, c := range b {
		if c.BlindLabel == "" || strings.Contains(c.BlindLabel, "REAL") || strings.Contains(c.BlindLabel, "model") {
			t.Fatalf("label leaks source: %q", c.BlindLabel)
		}
		labels[c.BlindLabel] = true
	}
	if len(labels) != 3 {
		t.Fatal("labels not unique")
	}
	sum, err := Unblind(b, []Rating{
		{ClipID: "c1", Dimension: Naturalness, Value: 5},
		{ClipID: "c2", Dimension: Naturalness, Value: 3},
		{ClipID: "c2", Dimension: Naturalness, Value: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	if sum["model_a"].MOS[Naturalness] != 3.5 || sum["model_a"].Normalised[Naturalness] != 0.625 {
		t.Fatalf("%+v", sum["model_a"])
	}
	if _, err := Unblind(b, []Rating{{ClipID: "ghost", Value: 3}}); err == nil {
		t.Fatal("unknown clip accepted")
	}
	if _, err := Unblind(b, []Rating{{ClipID: "c1", Value: 9}}); err == nil {
		t.Fatal("out-of-scale rating accepted")
	}
}

func TestLabels(t *testing.T) {
	if label(0) != "A" || label(25) != "Z" || label(26) != "AA" {
		t.Fatal(label(26))
	}
}
