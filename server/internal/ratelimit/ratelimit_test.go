package ratelimit

import (
	"testing"
	"time"
)

func TestAllowsUpToBurstThenRefuses(t *testing.T) {
	l := New()
	rule := Rule{Burst: 3, Window: time.Minute}

	for i := 0; i < 3; i++ {
		if ok, _ := l.Allow("k", rule); !ok {
			t.Fatalf("attempt %d refused within burst", i+1)
		}
	}
	ok, retry := l.Allow("k", rule)
	if ok {
		t.Fatal("attempt beyond burst allowed")
	}
	if retry <= 0 {
		t.Fatal("no retry hint given")
	}
}

func TestWindowExpiryRestoresAccess(t *testing.T) {
	l := New()
	now := time.Unix(1_700_000_000, 0)
	l.SetClock(func() time.Time { return now })
	rule := Rule{Burst: 2, Window: time.Minute}

	l.Allow("k", rule)
	l.Allow("k", rule)
	if ok, _ := l.Allow("k", rule); ok {
		t.Fatal("expected throttling")
	}

	now = now.Add(61 * time.Second)
	if ok, _ := l.Allow("k", rule); !ok {
		t.Fatal("still throttled after the window elapsed")
	}
}

// Keys must be independent, or one noisy client would throttle everyone.
func TestKeysAreIndependent(t *testing.T) {
	l := New()
	rule := Rule{Burst: 1, Window: time.Minute}

	if ok, _ := l.Allow("a", rule); !ok {
		t.Fatal("first key refused")
	}
	if ok, _ := l.Allow("a", rule); ok {
		t.Fatal("key a should be throttled")
	}
	if ok, _ := l.Allow("b", rule); !ok {
		t.Fatal("key b was throttled by key a's usage")
	}
}

// A successful login clears the counter, so someone who finally remembers
// their password is not left locked out.
func TestResetClearsCounter(t *testing.T) {
	l := New()
	rule := Rule{Burst: 1, Window: time.Minute}

	l.Allow("k", rule)
	if ok, _ := l.Allow("k", rule); ok {
		t.Fatal("expected throttling")
	}
	l.Reset("k")
	if ok, _ := l.Allow("k", rule); !ok {
		t.Fatal("reset did not clear the counter")
	}
}

func TestZeroRuleIsUnlimited(t *testing.T) {
	l := New()
	for i := 0; i < 100; i++ {
		if ok, _ := l.Allow("k", Rule{}); !ok {
			t.Fatal("a zero rule should not throttle")
		}
	}
}

// The account rule must be tighter than the IP rule: many people can share an
// address, but one account does not need many password attempts a minute.
func TestAccountRuleIsTighterThanIPRule(t *testing.T) {
	if LoginPerAccount.Burst >= LoginPerIP.Burst {
		t.Fatalf("per-account burst (%d) should be below per-IP (%d)",
			LoginPerAccount.Burst, LoginPerIP.Burst)
	}
}

// Bucket count must not grow without bound when an attacker varies the key.
func TestHighCardinalityKeysAreReclaimed(t *testing.T) {
	l := New()
	now := time.Unix(1_700_000_000, 0)
	l.SetClock(func() time.Time { return now })
	rule := Rule{Burst: 1, Window: time.Second}

	for i := 0; i < 10001; i++ {
		l.Allow(string(rune(i%1000))+itoa(i), rule)
	}
	now = now.Add(2 * time.Second)
	l.Allow("trigger-sweep", rule)

	l.mu.Lock()
	n := len(l.buckets)
	l.mu.Unlock()
	if n > 10000 {
		t.Fatalf("buckets not reclaimed: %d entries", n)
	}
}
