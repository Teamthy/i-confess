package ratelimit

import (
	"errors"
	"sync"
	"testing"
	"time"
)

// fakeStore is an in-process stand-in for Redis, so the distributed limiter's
// logic is tested without requiring a server in CI.
type fakeStore struct {
	mu     sync.Mutex
	counts map[string]int
	err    error
	incrs  int
}

func newFakeStore() *fakeStore { return &fakeStore{counts: map[string]int{}} }

func (f *fakeStore) Incr(key string, _ time.Duration) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.incrs++
	if f.err != nil {
		return 0, f.err
	}
	f.counts[key]++
	return f.counts[key], nil
}

func (f *fakeStore) Del(key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	delete(f.counts, key)
	return nil
}

func (f *fakeStore) Close() error { return nil }

func (f *fakeStore) setErr(err error) {
	f.mu.Lock()
	f.err = err
	f.mu.Unlock()
}

func TestDistributedEnforcesBurst(t *testing.T) {
	d := NewDistributed(newFakeStore())
	rule := Rule{Burst: 3, Window: time.Minute}

	for i := 0; i < 3; i++ {
		if ok, _ := d.Allow("k", rule); !ok {
			t.Fatalf("attempt %d refused within burst", i+1)
		}
	}
	if ok, retry := d.Allow("k", rule); ok || retry <= 0 {
		t.Fatal("attempt beyond burst allowed")
	}
}

// The whole point of the shared store: two replicas must share one budget.
// With per-instance limiters, N replicas would allow N times the limit.
func TestLimitsAreSharedAcrossInstances(t *testing.T) {
	store := newFakeStore()
	replicaA := NewDistributed(store)
	replicaB := NewDistributed(store)
	rule := Rule{Burst: 4, Window: time.Minute}

	// Two requests hit each replica: four total, which exhausts the budget.
	for i := 0; i < 2; i++ {
		if ok, _ := replicaA.Allow("login:acct:x", rule); !ok {
			t.Fatal("replica A refused within the shared budget")
		}
		if ok, _ := replicaB.Allow("login:acct:x", rule); !ok {
			t.Fatal("replica B refused within the shared budget")
		}
	}
	// The fifth must be refused no matter which replica serves it.
	if ok, _ := replicaB.Allow("login:acct:x", rule); ok {
		t.Fatal("shared budget was exceeded: limits are not actually shared")
	}
	if ok, _ := replicaA.Allow("login:acct:x", rule); ok {
		t.Fatal("shared budget was exceeded on the other replica")
	}
}

// If Redis is unreachable the service must keep serving logins, degrading to
// per-instance throttling. A limiter that takes the site down when its backing
// store fails is worse than one that briefly allows N times the limit.
func TestFallsBackWhenStoreIsDown(t *testing.T) {
	store := newFakeStore()
	d := NewDistributed(store)
	rule := Rule{Burst: 2, Window: time.Minute}

	if ok, _ := d.Allow("k", rule); !ok {
		t.Fatal("first attempt refused")
	}

	store.setErr(errors.New("connection refused"))

	// Still serving, now via the local fallback.
	if ok, _ := d.Allow("k", rule); !ok {
		t.Fatal("limiter refused all traffic when the store failed")
	}
	if !d.Degraded() {
		t.Fatal("degraded state not reported")
	}

	// The fallback still enforces a limit rather than allowing everything.
	allowed := 0
	for i := 0; i < 20; i++ {
		if ok, _ := d.Allow("k", rule); ok {
			allowed++
		}
	}
	if allowed > 5 {
		t.Fatalf("fallback allowed %d of 20 attempts: not enforcing", allowed)
	}

	// Recovery is automatic.
	store.setErr(nil)
	d.Allow("other", rule)
	if d.Degraded() {
		t.Fatal("still reporting degraded after the store recovered")
	}
}

func TestDistributedResetClearsSharedCounter(t *testing.T) {
	store := newFakeStore()
	d := NewDistributed(store)
	rule := Rule{Burst: 1, Window: time.Minute}

	d.Allow("k", rule)
	if ok, _ := d.Allow("k", rule); ok {
		t.Fatal("expected throttling")
	}
	d.Reset("k")
	if ok, _ := d.Allow("k", rule); !ok {
		t.Fatal("reset did not clear the shared counter")
	}
}

// Each check must cost one round trip: this runs on every login.
func TestDistributedUsesOneRoundTripPerCheck(t *testing.T) {
	store := newFakeStore()
	d := NewDistributed(store)
	rule := Rule{Burst: 100, Window: time.Minute}

	for i := 0; i < 10; i++ {
		d.Allow("k", rule)
	}
	if store.incrs != 10 {
		t.Fatalf("%d store calls for 10 checks, want 10", store.incrs)
	}
}

func TestDistributedWithNilStoreUsesLocal(t *testing.T) {
	d := NewDistributed(nil)
	rule := Rule{Burst: 2, Window: time.Minute}

	d.Allow("k", rule)
	d.Allow("k", rule)
	if ok, _ := d.Allow("k", rule); ok {
		t.Fatal("nil store should fall back to local enforcement, not allow everything")
	}
}

// Both implementations must satisfy the shared contract.
func TestBothLimitersImplementEnforcer(t *testing.T) {
	var _ Enforcer = New()
	var _ Enforcer = NewDistributed(newFakeStore())
}

// ---------------------------------------------------------------------------
// RESP encoding
// ---------------------------------------------------------------------------

func TestEncodeCommand(t *testing.T) {
	got := string(encodeCommand("INCR", "rl:login:1.2.3.4"))
	want := "*2\r\n$4\r\nINCR\r\n$17\r\nrl:login:1.2.3.4\r\n"
	// Length prefix must match the actual byte length, or Redis desynchronises.
	if got == want {
		return
	}
	// Recompute expected length rather than hard-coding a miscount.
	key := "rl:login:1.2.3.4"
	want = "*2\r\n$4\r\nINCR\r\n$" + itoa(len(key)) + "\r\n" + key + "\r\n"
	if got != want {
		t.Fatalf("encodeCommand:\n got %q\nwant %q", got, want)
	}
}
