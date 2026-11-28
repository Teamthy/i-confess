package featureflags

import (
	"context"
	"sync"
)

// FakeRegister is an in-memory Register implementation for tests. It lets
// callers enable or disable any feature flag without touching the database.
// Zero value is usable: every feature is disabled by default.
//
// This is the "label fake": tests inject it in place of DBStore so feature
// gating can be exercised without a database, and so the same handler can be
// tested with a feature both on and off.
type FakeRegister struct {
	mu    sync.RWMutex
	flags map[string]bool
}

// NewFakeRegister creates a fake register with the given features enabled.
func NewFakeRegister(enabled ...string) *FakeRegister {
	f := &FakeRegister{flags: map[string]bool{}}
	for _, k := range enabled {
		f.flags[k] = true
	}
	return f
}

// Enable sets a feature flag to enabled.
func (f *FakeRegister) Enable(key string) {
	f.mu.Lock()
	if f.flags == nil {
		f.flags = map[string]bool{}
	}
	f.flags[key] = true
	f.mu.Unlock()
}

// Disable sets a feature flag to disabled.
func (f *FakeRegister) Disable(key string) {
	f.mu.Lock()
	if f.flags == nil {
		f.flags = map[string]bool{}
	}
	f.flags[key] = false
	f.mu.Unlock()
}

// IsEnabled reports whether the named feature is enabled.
func (f *FakeRegister) IsEnabled(ctx context.Context, key string) (bool, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.flags[key], nil
}

// EnabledKeys returns the set of feature keys that are enabled.
func (f *FakeRegister) EnabledKeys() map[string]bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := map[string]bool{}
	for k, v := range f.flags {
		if v {
			out[k] = true
		}
	}
	return out
}

// compile-time check.
var _ Register = (*FakeRegister)(nil)
