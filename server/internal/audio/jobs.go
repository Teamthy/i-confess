package audio

import "fmt"

// JobStatus is a generation request's place in its lifecycle. These are the
// values audio_generation_jobs_status_check accepts.
type JobStatus string

const (
	JobQueued     JobStatus = "queued"
	JobProcessing JobStatus = "processing"
	JobSucceeded  JobStatus = "succeeded"
	JobFailed     JobStatus = "failed"
	JobCancelled  JobStatus = "cancelled"
)

// AllJobStatuses is every status the database constraint accepts.
func AllJobStatuses() []JobStatus {
	return []JobStatus{JobQueued, JobProcessing, JobSucceeded, JobFailed, JobCancelled}
}

// ValidJobStatus reports whether the status is one the schema permits. Checked
// before a write so a typo is rejected here rather than by a CHECK constraint
// deep inside a request.
func ValidJobStatus(s string) bool {
	for _, v := range AllJobStatuses() {
		if JobStatus(s) == v {
			return true
		}
	}
	return false
}

var jobTransitions = map[JobStatus][]JobStatus{
	JobQueued: {JobProcessing, JobCancelled},
	// Processing may return to queued when a retryable provider fault leaves
	// attempts remaining; the caller checks AttemptCount before doing so.
	JobProcessing: {JobSucceeded, JobFailed, JobQueued, JobCancelled},
	// A failed job can be requeued while attempts remain.
	JobFailed:    {JobQueued, JobCancelled},
	JobSucceeded: {}, // terminal
	JobCancelled: {}, // terminal
}

// CanTransitionJob reports whether a job may move between two statuses.
func CanTransitionJob(from, to JobStatus) bool {
	if from == to {
		return true
	}
	for _, allowed := range jobTransitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}

// ValidateJobTransition explains a refusal the API can return.
func ValidateJobTransition(from, to JobStatus) error {
	if !ValidJobStatus(string(to)) {
		return fmt.Errorf("%q is not a recognised generation job status", to)
	}
	if !CanTransitionJob(from, to) {
		allowed := jobTransitions[from]
		if len(allowed) == 0 {
			return fmt.Errorf("a generation job in %q is terminal and cannot change status", from)
		}
		return fmt.Errorf("a generation job cannot move from %q to %q; allowed: %v", from, to, allowed)
	}
	return nil
}
