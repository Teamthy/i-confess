package cache

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/backoff"
)

// The Redis bus is the path that matters in a multi-replica deployment, and it
// has three things to get right: a message published on one instance reaches a
// subscriber on another, a payload that is not one of ours is ignored rather
// than treated as a broken subscription, and a dropped connection is
// re-established.
//
// The first and last are tested against a real Redis when REDIS_ADDR is set -
// CI provides one - and the reconnect is tested against a stand-in server that
// drops the connection on command, so the retry path is exercised even on a
// machine with no Redis.

// requireRedis skips - loudly - when no server is configured. It skips rather
// than fails because `go test ./...` has to work for a contributor who has
// PostgreSQL (which the suite requires) but no Redis, while CI sets REDIS_ADDR
// so the assertions actually run there.
func requireRedis(t *testing.T) string {
	t.Helper()
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		t.Skip("REDIS_ADDR is not set: skipping the live Redis pub/sub assertions, which CI runs")
	}
	return addr
}

func newTestRedisBus(t *testing.T, addr string) *RedisBus {
	t.Helper()
	bus := NewRedisBus(addr, os.Getenv("REDIS_PASSWORD"))
	// A unique channel per test: a Redis server is shared with the rate
	// limiter and possibly with other tests, and a subscriber that sees a
	// stranger's message would fail for reasons the test cannot explain.
	bus.Channel = fmt.Sprintf("iconfess:test:invalidate:%d:%s", os.Getpid(), t.Name())
	bus.Policy = backoff.New(5*time.Millisecond, 50*time.Millisecond)
	t.Cleanup(func() { _ = bus.Close() })
	return bus
}

func TestRedisBusDeliversAcrossInstances(t *testing.T) {
	addr := requireRedis(t)

	subscriber := newTestRedisBus(t, addr)
	publisher := newTestRedisBus(t, addr)

	got := make(chan Message, 4)
	if err := subscriber.Subscribe(context.Background(), func(m Message) { got <- m }); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	sent := Message{Key: "catconf:", Prefix: true, Origin: "instance-a", Sent: time.Now().UTC()}
	if err := publisher.Publish(context.Background(), sent); err != nil {
		t.Fatalf("publish: %v", err)
	}

	select {
	case m := <-got:
		if m.Key != sent.Key || !m.Prefix || m.Origin != sent.Origin {
			t.Fatalf("message arrived altered: got %+v, want %+v", m, sent)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("published invalidation never arrived")
	}
}

// TestRedisBusIgnoresOtherTraffic covers the decode path's tolerance. A channel
// carries whatever any publisher puts on it, and a subscription that treated a
// foreign payload as an error would tear itself down and reconnect forever.
func TestRedisBusIgnoresOtherTraffic(t *testing.T) {
	addr := requireRedis(t)

	bus := newTestRedisBus(t, addr)
	got := make(chan Message, 4)
	if err := bus.Subscribe(context.Background(), func(m Message) { got <- m }); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	// Payloads a stranger might publish on the same channel, written with a
	// real client connection so nothing in the bus is doing the writing.
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("dial redis: %v", err)
	}
	defer func() { _ = conn.Close() }()
	for _, payload := range []string{"not json at all", `{"key":""}`, `{"unexpected":"shape"}`, ""} {
		if _, err := conn.Write(encodeCommand("PUBLISH", bus.Channel, payload)); err != nil {
			t.Fatalf("publish raw: %v", err)
		}
		if _, err := readValue(bufio.NewReader(conn)); err != nil {
			t.Fatalf("read publish reply: %v", err)
		}
	}

	// None of that may be delivered, and the subscription must still work.
	select {
	case m := <-got:
		t.Fatalf("foreign payload was delivered as an invalidation: %+v", m)
	case <-time.After(300 * time.Millisecond):
	}

	want := Message{Key: "voices:all"}
	other := newTestRedisBus(t, addr)
	other.Channel = bus.Channel
	if err := other.Publish(context.Background(), want); err != nil {
		t.Fatalf("publish after foreign traffic: %v", err)
	}
	select {
	case m := <-got:
		if m.Key != want.Key {
			t.Fatalf("got %+v, want key %q", m, want.Key)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("subscription stopped delivering after foreign traffic")
	}
}

// TestRedisBusReconnectsAfterTheSubscriptionDrops uses a stand-in Redis that
// accepts a subscription, sends one message, then closes the connection. The
// bus has to notice, reconnect and deliver what the second connection carries.
func TestRedisBusReconnectsAfterTheSubscriptionDrops(t *testing.T) {
	server := newFakeRedis(t, 2)
	bus := NewRedisBus("fake:6379", "")
	bus.Channel = "iconfess:test:invalidate"
	bus.Policy = backoff.New(time.Millisecond, 5*time.Millisecond)
	bus.dial = server.dial
	defer func() { _ = bus.Close() }()

	got := make(chan Message, 4)
	if err := bus.Subscribe(context.Background(), func(m Message) { got <- m }); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	for _, want := range []string{"first-connection", "second-connection"} {
		select {
		case m := <-got:
			if m.Key != want {
				t.Fatalf("got key %q, want %q", m.Key, want)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("never received %q, so the bus did not recover from the dropped connection", want)
		}
	}

	if conns := server.connectionCount(); conns < 2 {
		t.Fatalf("the stand-in server saw %d connections, so nothing reconnected", conns)
	}

	// Close has to interrupt the live subscription rather than wait for Redis
	// to send something. The stand-in server holds its last connection open for
	// two seconds doing nothing, so a Close that blocks here is a shutdown that
	// would hang on SIGTERM.
	closed := make(chan struct{})
	go func() { _ = bus.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Close blocked on a live subscription connection")
	}
}

// TestRedisBusPublishWithoutASubscriberIsNotAnError: PUBLISH to a channel nobody
// listens on returns 0, which is a success, and an instance writing to a
// database while its peers are restarting must not log a failure for it.
func TestRedisBusPublishWithoutASubscriberIsNotAnError(t *testing.T) {
	addr := requireRedis(t)
	bus := newTestRedisBus(t, addr)
	if err := bus.Publish(context.Background(), Message{Key: "voices:all"}); err != nil {
		t.Fatalf("publish with no subscribers: %v", err)
	}
}

func TestRedisBusPublishReportsAnUnreachableServer(t *testing.T) {
	bus := NewRedisBus("127.0.0.1:1", "")
	bus.Timeout = 200 * time.Millisecond
	defer func() { _ = bus.Close() }()
	if err := bus.Publish(context.Background(), Message{Key: "voices:all"}); err == nil {
		t.Fatal("expected an error publishing to a closed port")
	}
}

func TestDecodeMessageRejectsMalformedReplies(t *testing.T) {
	channel := "iconfess:cache:invalidate"
	cases := map[string]struct {
		reply value
		ok    bool
	}{
		"subscribe confirmation": {
			reply: value{kind: '*', items: []value{
				{kind: '$', text: "subscribe"}, {kind: '$', text: channel}, {kind: ':', text: "1"},
			}},
			ok: false,
		},
		"message on another channel": {
			reply: value{kind: '*', items: []value{
				{kind: '$', text: "message"}, {kind: '$', text: "other"}, {kind: '$', text: `{"key":"x"}`},
			}},
			ok: false,
		},
		"not json": {
			reply: value{kind: '*', items: []value{
				{kind: '$', text: "message"}, {kind: '$', text: channel}, {kind: '$', text: "nonsense"},
			}},
			ok: false,
		},
		"empty key": {
			reply: value{kind: '*', items: []value{
				{kind: '$', text: "message"}, {kind: '$', text: channel}, {kind: '$', text: `{}`},
			}},
			ok: false,
		},
		"valid": {
			reply: value{kind: '*', items: []value{
				{kind: '$', text: "message"}, {kind: '$', text: channel},
				{kind: '$', text: `{"key":"voices:all","origin":"a"}`},
			}},
			ok: true,
		},
	}
	for name, tc := range cases {
		msg, ok := decodeMessage(tc.reply, channel)
		if ok != tc.ok {
			t.Errorf("%s: ok=%v, want %v", name, ok, tc.ok)
		}
		if ok && msg.Key == "" {
			t.Errorf("%s: decoded an empty key", name)
		}
	}
}

func TestEncodeCommandUsesTheLengthsItDeclares(t *testing.T) {
	got := string(encodeCommand("PUBLISH", "chan", "payload"))
	want := "*3\r\n$7\r\nPUBLISH\r\n$4\r\nchan\r\n$7\r\npayload\r\n"
	if got != want {
		t.Fatalf("encodeCommand mismatch\n got %q\nwant %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// Stand-in Redis: enough RESP to subscribe, push a message and drop the
// connection, so the reconnect loop is exercised without a server.
// ---------------------------------------------------------------------------

type fakeRedis struct {
	t      *testing.T
	mu     sync.Mutex
	conns  int
	keys   []string // one message key per connection, in order
	closed bool
}

func newFakeRedis(t *testing.T, connections int) *fakeRedis {
	t.Helper()
	keys := make([]string, 0, connections)
	keys = append(keys, "first-connection")
	if connections > 1 {
		keys = append(keys, "second-connection")
	}
	return &fakeRedis{t: t, keys: keys}
}

func (f *fakeRedis) connectionCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.conns
}

// dial hands the bus the client end of an in-memory connection and serves the
// other end. net.Pipe is deliberate: it has no kernel buffers, so a write is
// only complete once the bus has read it and the test cannot pass on unflushed
// bytes.
func (f *fakeRedis) dial(_ context.Context, _ string, _ time.Duration) (net.Conn, error) {
	f.mu.Lock()
	if f.closed || f.conns >= len(f.keys) {
		f.mu.Unlock()
		return nil, fmt.Errorf("fake redis: no more connections")
	}
	index := f.conns
	f.conns++
	f.mu.Unlock()

	client, server := net.Pipe()
	go f.serve(server, f.keys[index])
	return client, nil
}

func (f *fakeRedis) serve(conn net.Conn, key string) {
	defer func() { _ = conn.Close() }()
	rd := bufio.NewReader(conn)

	reply, err := readValue(rd)
	if err != nil {
		return
	}
	if len(reply.items) == 0 || !strings.EqualFold(reply.items[0].text, "SUBSCRIBE") {
		f.t.Errorf("stand-in redis received %v, expected SUBSCRIBE", reply)
		return
	}
	channel := reply.items[1].text

	if _, err := conn.Write([]byte("*3\r\n$9\r\nsubscribe\r\n" + bulk(channel) + ":1\r\n")); err != nil {
		return
	}
	payload, _ := json.Marshal(Message{Key: key, Sent: time.Now().UTC()})
	if _, err := conn.Write([]byte("*3\r\n$7\r\nmessage\r\n" + bulk(channel) + bulk(string(payload)))); err != nil {
		return
	}
	// The first connection drops straight after its message, which is what the
	// bus has to recover from. The last one stays up so the delivered message
	// is the only thing under test.
	if key != f.keys[len(f.keys)-1] {
		return
	}
	<-time.After(2 * time.Second)
}

func bulk(s string) string {
	return fmt.Sprintf("$%d\r\n%s\r\n", len(s), s)
}
