package api

import (
	"context"
	"time"
)

const trialExpiryBatchSize = 500

// RunTrialExpirySweep advances every due trial and its subscription projection.
// It drains fixed-size pages so startup can recover a backlog without loading
// the whole trial table into memory. TrialStore.Refresh owns row locking and
// idempotent event emission, making this safe on multiple API replicas.
func (h *Handler) RunTrialExpirySweep(ctx context.Context) (int, error) {
	at := time.Now().UTC()
	total := 0
	for {
		expired, err := h.trials.SweepExpired(ctx, at, trialExpiryBatchSize)
		total += expired
		if err != nil {
			return total, err
		}
		if expired < trialExpiryBatchSize {
			return total, nil
		}
	}
}
