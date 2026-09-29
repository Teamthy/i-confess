package voiceengine

// GenerationStatus is the lifecycle of one synthetic render job (§23).
type GenerationStatus string

const (
	GenQueued         GenerationStatus = "QUEUED"
	GenProcessing     GenerationStatus = "PROCESSING"
	GenGenerated      GenerationStatus = "GENERATED"
	GenPostProcessing GenerationStatus = "POST_PROCESSING"
	GenUploading      GenerationStatus = "UPLOADING"
	GenCompleted      GenerationStatus = "COMPLETED"
	GenFailed         GenerationStatus = "FAILED"
	GenCancelled      GenerationStatus = "CANCELLED"
)

// GenerationStatuses in lifecycle order.
var GenerationStatuses = []GenerationStatus{GenQueued, GenProcessing, GenGenerated,
	GenPostProcessing, GenUploading, GenCompleted, GenFailed, GenCancelled}

// Terminal reports whether no further transition is possible.
func (s GenerationStatus) Terminal() bool {
	return s == GenCompleted || s == GenFailed || s == GenCancelled
}

// Cancellable reports whether a job in s can still be cancelled. Once audio
// is uploading, cancelling would leave an orphaned object; let it finish.
func (s GenerationStatus) Cancellable() bool {
	return s == GenQueued || s == GenProcessing || s == GenGenerated || s == GenPostProcessing
}

// TrainingStatus is the lifecycle of a training run (§21).
type TrainingStatus string

const (
	TrainQueued     TrainingStatus = "QUEUED"
	TrainRunning    TrainingStatus = "RUNNING"
	TrainFailed     TrainingStatus = "FAILED"
	TrainCompleted  TrainingStatus = "COMPLETED"
	TrainEvaluating TrainingStatus = "EVALUATING"
	TrainApproved   TrainingStatus = "APPROVED"
	TrainRejected   TrainingStatus = "REJECTED"
	TrainArchived   TrainingStatus = "ARCHIVED"
)

// TrainingStatuses lists all training statuses.
var TrainingStatuses = []TrainingStatus{TrainQueued, TrainRunning, TrainFailed, TrainCompleted,
	TrainEvaluating, TrainApproved, TrainRejected, TrainArchived}

// ModelStatuses lists all model statuses.
var ModelStatuses = []ModelStatus{ModelCandidate, ModelEvaluating, ModelApproved,
	ModelProduction, ModelRetired, ModelRejected}
