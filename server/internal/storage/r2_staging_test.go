//go:build r2staging

package storage

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestPrivateR2StagingDelivery is an operator-invoked release check, never a
// default CI network dependency. With the tag it fails (does not skip) for
// missing settings, an incomplete check, or unconfirmed cleanup.
func TestPrivateR2StagingDelivery(t *testing.T) {
	cfg, err := r2StagingConfig(os.Getenv)
	if err != nil {
		t.Fatalf("NOT VERIFIED: %v", err)
	}
	client := r2CheckClient(nil)
	defer client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	wait := func(ctx context.Context, until time.Time) error {
		timer := time.NewTimer(time.Until(until))
		defer timer.Stop()
		select {
		case <-timer.C:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err := checkPrivateR2Delivery(ctx, r2CheckStore(cfg, client), cfg.S3Endpoint, client, wait, t.Logf); err != nil {
		t.Fatalf("NOT VERIFIED: %v", err)
	}
	t.Log("Verified direct S3-compatible R2 delivery for the synthetic staging fixture; current fixture key removed. Public/custom routes require separate review.")
}
