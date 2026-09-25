package cache

import (
	"context"
	"testing"
	"time"
)

func TestMemoryBusDeliversToEverySubscriber(t *testing.T) {
	bus := NewMemoryBus()
	defer func() { _ = bus.Close() }()

	first := make(chan Message, 4)
	second := make(chan Message, 4)
	if err := bus.Subscribe(context.Background(), func(m Message) { first <- m }); err != nil {
		t.Fatalf("subscribe first: %v", err)
	}
	if err := bus.Subscribe(context.Background(), func(m Message) { second <- m }); err != nil {
		t.Fatalf("subscribe second: %v", err)
	}

	want := Message{Key: "categories:published", Origin: "instance-a"}
	if err := bus.Publish(context.Background(), want); err != nil {
		t.Fatalf("publish: %v", err)
	}

	for name, ch := range map[string]chan Message{"first": first, "second": second} {
		select {
		case got := <-ch:
			if got.Key != want.Key || got.Origin != want.Origin {
				t.Fatalf("%s subscriber got %+v, want key %q origin %q", name, got, want.Key, want.Origin)
			}
		case <-time.After(time.Second):
			t.Fatalf("%s subscriber received nothing", name)
		}
	}
}

func TestMemoryBusForwardsThePrefixFlag(t *testing.T) {
	bus := NewMemoryBus()
	defer func() { _ = bus.Close() }()

	got := make(chan Message, 1)
	if err := bus.Subscribe(context.Background(), func(m Message) { got <- m }); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if err := bus.Publish(context.Background(), Message{Key: "catconf:", Prefix: true}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	select {
	case m := <-got:
		if !m.Prefix || m.Key != "catconf:" {
			t.Fatalf("prefix flag lost in transit: %+v", m)
		}
	case <-time.After(time.Second):
		t.Fatal("no delivery")
	}
}

func TestMemoryBusStopsDeliveringAfterContextCancel(t *testing.T) {
	bus := NewMemoryBus()
	defer func() { _ = bus.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	got := make(chan Message, 4)
	if err := bus.Subscribe(ctx, func(m Message) { got <- m }); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if err := bus.Publish(context.Background(), Message{Key: "voices:all"}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	select {
	case <-got:
	case <-time.After(time.Second):
		t.Fatal("no delivery before cancel")
	}

	cancel()
	// Unregistration is asynchronous - the subscription's own goroutine does
	// it - so a publish immediately after cancel may still be delivered. What
	// must hold is that delivery stops, not that it stops within a nanosecond.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if err := bus.Publish(context.Background(), Message{Key: "voices:all"}); err != nil {
			t.Fatalf("publish: %v", err)
		}
		select {
		case <-got:
		case <-time.After(20 * time.Millisecond):
			return // delivery stopped: exactly one subscriber was removed
		}
	}
	t.Fatal("a cancelled subscription kept receiving messages")
}

func TestMemoryBusRefusesWorkAfterClose(t *testing.T) {
	bus := NewMemoryBus()
	if err := bus.Subscribe(context.Background(), func(Message) {}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if err := bus.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := bus.Publish(context.Background(), Message{Key: "voices:all"}); err != ErrBusClosed {
		t.Fatalf("publish after close: got %v, want ErrBusClosed", err)
	}
	if err := bus.Subscribe(context.Background(), func(Message) {}); err != ErrBusClosed {
		t.Fatalf("subscribe after close: got %v, want ErrBusClosed", err)
	}
	if err := bus.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}

func TestSubscribeRejectsANilHandler(t *testing.T) {
	bus := NewMemoryBus()
	defer func() { _ = bus.Close() }()
	if err := bus.Subscribe(context.Background(), nil); err == nil {
		t.Fatal("expected an error for a nil handler")
	}
}
