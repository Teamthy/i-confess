package cache

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Cache invalidation between API instances.
//
// The value caches in this package are per-process, which was correct while one
// instance ran and wrong the moment a second one started: an admin edit landed
// in the database and in the writing instance's memory, and every other
// instance kept serving its own copy until ttl+swr expired - up to fifteen
// minutes on the category cache. Gap G-10 in docs/03-TECHNOLOGY-DECISIONS.md
// recorded that. This file is the invalidation path that closes it.
//
// The design is deliberately narrow. A message names a key (or a prefix of
// keys) that is no longer true; the receiver drops it. Nothing is broadcast on
// read, no value travels between instances, and a lost message degrades to
// exactly today's behaviour - the entry expires on its own - so the bus is an
// optimisation for correctness *now* rather than a new availability
// dependency. That is why a publish failure is logged and swallowed instead of
// failing the admin write that triggered it: the write already committed.

// ErrBusClosed is returned by a Bus that has been closed.
var ErrBusClosed = errors.New("cache: bus is closed")

// Message is one invalidation. Key is either an exact cache key or, when
// Prefix is set, the shared prefix of every key to drop.
type Message struct {
	Key    string `json:"key"`
	Prefix bool   `json:"prefix,omitempty"`
	// Origin identifies the instance that did the write. A receiver drops
	// messages it sent itself: it has already applied them locally, and
	// re-applying would be a second delete of the same key - harmless, but
	// counting it as an invalidation would overstate what the bus delivered.
	Origin string    `json:"origin,omitempty"`
	Sent   time.Time `json:"sent"`
}

// Bus carries invalidations to the other instances of the API.
//
// Subscribe is separate from Publish because the two have different failure
// modes: a publish that fails must not fail the write, while a subscription
// that cannot be established means this instance will serve stale content for
// up to ttl+swr and the caller needs to say so in the log at startup.
type Bus interface {
	// Publish delivers one message to every other instance.
	Publish(ctx context.Context, msg Message) error
	// Subscribe registers handle and blocks until the subscription is
	// established. handle is called from a goroutine the Bus owns; it must
	// not block. The subscription ends when ctx is done or Close is called.
	Subscribe(ctx context.Context, handle func(Message)) error
	// Close releases the bus's connections and stops its subscriptions.
	Close() error
}

// MemoryBus delivers invalidations in-process.
//
// It is what a single-instance deployment gets, and it is what tests use to
// stand two Handlers up against one database and watch a write on one become
// visible on the other. Delivery is synchronous and in the caller's goroutine:
// every subscriber here is a map delete, and paying a channel and a goroutine
// to make that asynchronous would buy latency the write path does not need
// while making test timing nondeterministic.
type MemoryBus struct {
	mu          sync.Mutex
	subscribers map[*memorySubscription]struct{}
	closed      bool
}

type memorySubscription struct {
	handle func(Message)
	done   chan struct{}
}

func NewMemoryBus() *MemoryBus {
	return &MemoryBus{subscribers: map[*memorySubscription]struct{}{}}
}

// Publish implements Bus.
func (b *MemoryBus) Publish(_ context.Context, msg Message) error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return ErrBusClosed
	}
	handles := make([]func(Message), 0, len(b.subscribers))
	for sub := range b.subscribers {
		handles = append(handles, sub.handle)
	}
	b.mu.Unlock()

	// Outside the lock: a subscriber that subscribed or unsubscribed while
	// this publish was in flight must not be able to deadlock it, and an
	// invalidate never calls back into the bus.
	for _, handle := range handles {
		handle(msg)
	}
	return nil
}

// Subscribe implements Bus. It returns as soon as the handler is registered;
// the caller's ctx ending unregisters it.
func (b *MemoryBus) Subscribe(ctx context.Context, handle func(Message)) error {
	if handle == nil {
		return errors.New("cache: nil subscription handler")
	}
	sub := &memorySubscription{handle: handle, done: make(chan struct{})}

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return ErrBusClosed
	}
	b.subscribers[sub] = struct{}{}
	b.mu.Unlock()

	go func() {
		select {
		case <-ctx.Done():
		case <-sub.done:
		}
		b.mu.Lock()
		delete(b.subscribers, sub)
		b.mu.Unlock()
	}()
	return nil
}

// Close implements Bus. Subscriptions stop being delivered to rather than
// being signalled: Close has no error to report a subscriber's failure through,
// and a closed bus has nothing left to reach.
func (b *MemoryBus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	for sub := range b.subscribers {
		close(sub.done)
		delete(b.subscribers, sub)
	}
	return nil
}
