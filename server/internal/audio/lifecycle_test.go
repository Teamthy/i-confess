package audio

import "testing"

func TestAssetLifecycleMatchesTheDatabaseVocabulary(t *testing.T) {
	// 0004_audio_qa.sql defines the CHECK. If these two drift, an asset can be
	// written that no query recognises, or refused that the schema allows.
	want := []AssetStatus{
		StatusUploading, StatusProcessing, StatusReady, StatusPublished,
		StatusFailed, StatusQARejected, StatusArchived,
	}
	got := All()
	if len(got) != len(want) {
		t.Fatalf("All() returned %d statuses, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("All()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if ValidAssetCount := len(got); ValidAssetCount != 7 {
		t.Errorf("expected 7 statuses, got %d", ValidAssetCount)
	}
	if (AssetStatus("bogus")).Valid() {
		t.Error(`"bogus" reported as a valid status`)
	}
}

func TestOnlyReadyAndPublishedAreServed(t *testing.T) {
	// Every one of these is a state where audio must not reach a listener: it is
	// still uploading, still in QA, failed, rejected by a human, or withdrawn.
	for _, st := range []AssetStatus{
		StatusUploading, StatusProcessing, StatusFailed, StatusQARejected, StatusArchived,
	} {
		if IsServed(string(st)) {
			t.Errorf("%q is served; it must not be", st)
		}
	}
	for _, st := range ServedStatuses() {
		if !IsServed(string(st)) {
			t.Errorf("%q is in ServedStatuses but IsServed reports false", st)
		}
	}
	// The absence of evidence is not evidence of approval. A session item with
	// no asset, or a row read before the status column existed, must not be
	// treated as approved.
	if IsServed("") {
		t.Error("an empty status was served")
	}
}

func TestQATransitions(t *testing.T) {
	allowed := [][2]AssetStatus{
		{StatusUploading, StatusProcessing},
		{StatusProcessing, StatusReady},
		{StatusProcessing, StatusQARejected},
		{StatusProcessing, StatusFailed},
		{StatusReady, StatusPublished},
		{StatusReady, StatusArchived},
		{StatusPublished, StatusArchived},
		{StatusFailed, StatusProcessing},     // a failed render is regenerated
		{StatusQARejected, StatusProcessing}, // a rejected one may be re-rendered
	}
	for _, tr := range allowed {
		if !CanTransition(tr[0], tr[1]) {
			t.Errorf("%q -> %q should be allowed", tr[0], tr[1])
		}
	}

	// The point of a QA workflow is that reaching a listener requires a human
	// step. Any edge that skips it defeats the workflow.
	forbidden := [][2]AssetStatus{
		{StatusProcessing, StatusPublished}, // skips approval
		{StatusUploading, StatusReady},      // skips QA entirely
		{StatusQARejected, StatusReady},     // overrides a human rejection
		{StatusQARejected, StatusPublished},
		{StatusArchived, StatusReady}, // terminal
		{StatusArchived, StatusPublished},
		{StatusPublished, StatusReady}, // cannot be un-published backwards
	}
	for _, tr := range forbidden {
		if CanTransition(tr[0], tr[1]) {
			t.Errorf("%q -> %q must not be allowed", tr[0], tr[1])
		}
		if err := ValidateTransition(tr[0], tr[1]); err == nil {
			t.Errorf("ValidateTransition(%q, %q) = nil, want a refusal", tr[0], tr[1])
		}
	}
}

func TestTransitionErrorNamesTheCurrentState(t *testing.T) {
	err := ValidateTransition(StatusQARejected, StatusReady)
	if err == nil {
		t.Fatal("expected a refusal")
	}
	var te *TransitionError
	if !asTransitionError(err, &te) {
		t.Fatalf("err = %T, want *TransitionError", err)
	}
	if te.From != StatusQARejected || te.To != StatusReady {
		t.Errorf("error lost context: from=%q to=%q", te.From, te.To)
	}
	// The message must tell the caller what they can do instead.
	if len(AllowedFrom(StatusQARejected)) == 0 {
		t.Error("a rejected render should be regenerable")
	}
}

func TestUnrecognisedTargetIsRefused(t *testing.T) {
	if err := ValidateTransition(StatusProcessing, AssetStatus("shipped")); err == nil {
		t.Error("an invented status was accepted")
	}
}

func TestJobLifecycle(t *testing.T) {
	for _, st := range AllJobStatuses() {
		if !ValidJobStatus(string(st)) {
			t.Errorf("%q not recognised as a job status", st)
		}
	}
	if ValidJobStatus("done") {
		t.Error(`"done" reported as a valid job status`)
	}

	allowed := [][2]JobStatus{
		{JobQueued, JobProcessing},
		{JobProcessing, JobSucceeded},
		{JobProcessing, JobFailed},
		{JobProcessing, JobQueued}, // retryable fault, attempts remain
		{JobFailed, JobQueued},     // requeue
		{JobQueued, JobCancelled},
	}
	for _, tr := range allowed {
		if !CanTransitionJob(tr[0], tr[1]) {
			t.Errorf("job %q -> %q should be allowed", tr[0], tr[1])
		}
	}

	forbidden := [][2]JobStatus{
		{JobSucceeded, JobProcessing}, // terminal
		{JobSucceeded, JobFailed},
		{JobCancelled, JobQueued}, // terminal
		{JobQueued, JobSucceeded}, // cannot skip the work
	}
	for _, tr := range forbidden {
		if CanTransitionJob(tr[0], tr[1]) {
			t.Errorf("job %q -> %q must not be allowed", tr[0], tr[1])
		}
		if err := ValidateJobTransition(tr[0], tr[1]); err == nil {
			t.Errorf("ValidateJobTransition(%q, %q) = nil, want a refusal", tr[0], tr[1])
		}
	}
}

// asTransitionError avoids importing errors twice across the package's tests.
func asTransitionError(err error, target **TransitionError) bool {
	te, ok := err.(*TransitionError)
	if ok {
		*target = te
	}
	return ok
}
