package cache

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Teamthy/i-confess/internal/backoff"
	"github.com/Teamthy/i-confess/internal/log"
)

// DefaultInvalidationChannel is the Redis channel invalidations are published
// on. It is namespaced because a Redis server is rarely dedicated to one
// application, and a bare "invalidate" would collide with anything else in the
// same database.
const DefaultInvalidationChannel = "iconfess:cache:invalidate"

// RedisBus carries invalidations between instances over Redis pub/sub.
//
// Redis is already in the deployment for shared rate limiting
// (REDIS_ADDR, internal/ratelimit), so pub/sub adds no new infrastructure: one
// more connection and one more command from a server the process is already
// allowed to talk to.
//
// The RESP client is written here rather than taken as a dependency for the
// same reason ratelimit's is: pub/sub needs three commands (AUTH, SUBSCRIBE,
// PUBLISH) and a reader for the reply shapes those three produce. What this
// client deliberately does *not* do is try to be a general Redis client - no
// cluster, no sentinel, no resumption of missed messages (pub/sub gives an
// at-most-once delivery guarantee by design, and the TTL is the backstop for a
// message that never arrives).
//
// One Bus holds at most one subscription, matching the Bus contract, plus one
// reusable connection for publishing.
type RedisBus struct {
	addr     string
	password string

	// Channel is where invalidations are published. Set before Subscribe or
	// Publish; DefaultInvalidationChannel is the zero value's channel.
	Channel string
	// Timeout bounds a single publish. It does not bound a subscription,
	// which by definition idles between messages.
	Timeout time.Duration
	// Policy is the reconnect schedule when the subscription drops. The
	// default doubles from 250ms to 30s.
	Policy backoff.Policy
	// Logf reports reconnects and publish failures. Defaults to the shared
	// logger at warn level, so a lost subscription is visible in the service
	// log rather than silent.
	Logf func(format string, args ...any)

	// dial connects to Redis. It is a field so tests can put a stand-in
	// server in front of the bus and drop the connection on demand.
	dial func(ctx context.Context, addr string, timeout time.Duration) (net.Conn, error)

	mu      sync.Mutex
	pubConn net.Conn
	pubRd   *bufio.Reader
	closed  bool

	subMu     sync.Mutex
	subCancel context.CancelFunc
	subConn   net.Conn
	subDone   chan struct{}
	wg        sync.WaitGroup
}

// NewRedisBus creates a bus. It does not dial: a Redis outage at startup must
// not stop the server from booting, and the first publish or subscribe reports
// the problem where it can be acted on.
func NewRedisBus(addr, password string) *RedisBus {
	return &RedisBus{
		addr:     addr,
		password: password,
		Channel:  DefaultInvalidationChannel,
		Timeout:  2 * time.Second,
		Policy:   backoff.New(250*time.Millisecond, 30*time.Second),
		Logf: func(format string, args ...any) {
			log.Warn(fmt.Sprintf(format, args...))
		},
		dial: func(ctx context.Context, addr string, timeout time.Duration) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "tcp", addr)
		},
	}
}

func (b *RedisBus) channel() string {
	if b.Channel == "" {
		return DefaultInvalidationChannel
	}
	return b.Channel
}

func (b *RedisBus) logf(format string, args ...any) {
	if b.Logf != nil {
		b.Logf(format, args...)
	}
}

// Publish implements Bus.
//
// One attempt, not a retry loop: this runs inside an admin write that has
// already committed, and a message that does not get out costs staleness until
// the receiving cache's TTL expires - the exact behaviour before invalidation
// existed. Retrying inside the request would trade that for latency on the
// write path.
func (b *RedisBus) Publish(ctx context.Context, msg Message) error {
	if msg.Sent.IsZero() {
		msg.Sent = time.Now().UTC()
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("cache: encode invalidation: %w", err)
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return ErrBusClosed
	}
	conn, err := b.publisher(ctx)
	if err != nil {
		return err
	}
	if err := conn.SetDeadline(time.Now().Add(b.timeout())); err != nil {
		b.dropPublisher()
		return err
	}
	if _, err := conn.Write(encodeCommand("PUBLISH", b.channel(), string(payload))); err != nil {
		b.dropPublisher()
		return fmt.Errorf("cache: publish invalidation: %w", err)
	}
	if _, err := readValue(b.pubRd); err != nil {
		b.dropPublisher()
		return fmt.Errorf("cache: publish invalidation: %w", err)
	}
	return nil
}

// publisher returns the reusable publish connection, dialling if needed. The
// caller holds b.mu.
func (b *RedisBus) publisher(ctx context.Context) (net.Conn, error) {
	if b.pubConn != nil {
		return b.pubConn, nil
	}
	conn, err := b.dial(ctx, b.addr, b.timeout())
	if err != nil {
		return nil, fmt.Errorf("cache: dial %s: %w", b.addr, err)
	}
	rd := bufio.NewReader(conn)
	if err := b.authenticate(conn, rd); err != nil {
		_ = conn.Close()
		return nil, err
	}
	b.pubConn, b.pubRd = conn, rd
	return conn, nil
}

// dropPublisher discards the publish connection after a failure, so the next
// publish dials a fresh one. The caller holds b.mu.
func (b *RedisBus) dropPublisher() {
	if b.pubConn != nil {
		_ = b.pubConn.Close()
		b.pubConn, b.pubRd = nil, nil
	}
}

func (b *RedisBus) timeout() time.Duration {
	if b.Timeout <= 0 {
		return 2 * time.Second
	}
	return b.Timeout
}

func (b *RedisBus) authenticate(conn net.Conn, rd *bufio.Reader) error {
	if b.password == "" {
		return nil
	}
	if err := conn.SetDeadline(time.Now().Add(b.timeout())); err != nil {
		return err
	}
	if _, err := conn.Write(encodeCommand("AUTH", b.password)); err != nil {
		return fmt.Errorf("cache: redis auth: %w", err)
	}
	if _, err := readValue(rd); err != nil {
		return fmt.Errorf("cache: redis auth: %w", err)
	}
	return nil
}

// Subscribe implements Bus. It returns once Redis has confirmed the
// subscription, so a caller that logs "shared cache invalidation enabled" has
// evidence for the claim. A dropped connection is retried on the reconnect
// policy until ctx is done or Close is called.
func (b *RedisBus) Subscribe(ctx context.Context, handle func(Message)) error {
	if handle == nil {
		return errors.New("cache: nil subscription handler")
	}

	subCtx, cancel := context.WithCancel(ctx)
	conn, rd, err := b.dialSubscription(subCtx)
	if err != nil {
		cancel()
		return err
	}

	b.subMu.Lock()
	if b.closed {
		b.subMu.Unlock()
		cancel()
		_ = conn.Close()
		return ErrBusClosed
	}
	b.subCancel, b.subConn, b.subDone = cancel, conn, make(chan struct{})
	done := b.subDone
	b.subMu.Unlock()

	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		defer close(done)
		b.consume(subCtx, conn, rd, handle)
	}()
	return nil
}

// dialSubscription opens a connection with the channel subscription already
// confirmed.
//
// It returns the reader along with the connection, and every later read uses
// that same reader. A fresh bufio.Reader per read looks harmless and is not:
// bufio reads ahead, so a message that arrived in the same TCP segment as the
// SUBSCRIBE confirmation would sit in the discarded reader's buffer and the
// subscription would look healthy while silently dropping it.
func (b *RedisBus) dialSubscription(ctx context.Context) (net.Conn, *bufio.Reader, error) {
	conn, err := b.dial(ctx, b.addr, b.timeout())
	if err != nil {
		return nil, nil, fmt.Errorf("cache: dial %s: %w", b.addr, err)
	}
	rd := bufio.NewReader(conn)
	if err := b.authenticate(conn, rd); err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	// No deadline from here on: a subscription is idle until a message
	// arrives, so a read deadline would tear it down on every quiet period.
	if err := conn.SetDeadline(time.Time{}); err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	if _, err := conn.Write(encodeCommand("SUBSCRIBE", b.channel())); err != nil {
		_ = conn.Close()
		return nil, nil, fmt.Errorf("cache: subscribe: %w", err)
	}
	if _, err := readValue(rd); err != nil {
		_ = conn.Close()
		return nil, nil, fmt.Errorf("cache: subscribe: %w", err)
	}
	return conn, rd, nil
}

// consume reads messages until the connection fails or ctx is done, then
// reconnects until one of those is still true.
func (b *RedisBus) consume(ctx context.Context, conn net.Conn, rd *bufio.Reader, handle func(Message)) {
	attempt := 0
	for {
		for {
			reply, err := readValue(rd)
			if err != nil {
				if ctx.Err() != nil {
					_ = conn.Close()
					return
				}
				b.logf("cache: invalidation subscription lost (%v); reconnecting", err)
				break
			}
			if msg, ok := decodeMessage(reply, b.channel()); ok {
				handle(msg)
			}
		}
		_ = conn.Close()

		reconnected := false
		for !reconnected {
			if ctx.Err() != nil {
				return
			}
			attempt++
			delay := b.Policy.Next(attempt)
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
			// The connect attempt gets its own short deadline; the resulting
			// connection must not inherit it, which is why dialSubscription
			// clears the deadline before reading messages.
			dialCtx, cancel := context.WithTimeout(ctx, b.timeout())
			next, nextRd, err := b.dialSubscription(dialCtx)
			cancel()
			if err != nil {
				b.logf("cache: invalidation resubscribe failed (%v); retrying", err)
				continue
			}
			conn, rd, reconnected = next, nextRd, true
			b.setSubConn(next)
			attempt = 0
			b.logf("cache: invalidation subscription restored")
		}
	}
}

// setSubConn records the connection currently subscribed, so Close can close it.
// Without this, Close blocks until Redis next writes something: the reader is
// parked in Read, and cancelling a context does not interrupt a blocked read.
// On SIGTERM that is a hung shutdown.
func (b *RedisBus) setSubConn(conn net.Conn) {
	b.subMu.Lock()
	if !b.closed {
		b.subConn = conn
	}
	b.subMu.Unlock()
}

// decodeMessage turns a pub/sub push into a Message. Anything that is not a
// message for this channel - the subscribe confirmation, a pong, a reply shape
// from a future Redis version - is ignored rather than treated as an error,
// because none of them mean the subscription is broken.
func decodeMessage(reply value, channel string) (Message, bool) {
	if !reply.isArray() || len(reply.items) != 3 {
		return Message{}, false
	}
	if reply.items[0].kind != '$' || reply.items[0].text != "message" {
		return Message{}, false
	}
	if reply.items[1].text != channel {
		return Message{}, false
	}
	var msg Message
	if err := json.Unmarshal([]byte(reply.items[2].text), &msg); err != nil {
		// A malformed payload is another publisher's problem, not a reason to
		// drop a healthy subscription.
		return Message{}, false
	}
	if msg.Key == "" {
		return Message{}, false
	}
	return msg, true
}

// Close implements Bus.
func (b *RedisBus) Close() error {
	b.mu.Lock()
	b.closed = true
	b.dropPublisher()
	b.mu.Unlock()

	b.subMu.Lock()
	if b.subCancel != nil {
		b.subCancel()
	}
	if b.subConn != nil {
		// Unblocks the reader parked in Read so the goroutine can exit.
		_ = b.subConn.Close()
		b.subConn = nil
	}
	b.subMu.Unlock()

	// Wait for the reader to notice its context is done, so a test that closes
	// a bus and immediately reuses the channel is not racing the old
	// subscription's teardown.
	b.wg.Wait()
	return nil
}

// ---------------------------------------------------------------------------
// RESP wire protocol (pub/sub subset)
// ---------------------------------------------------------------------------

// encodeCommand renders a command in RESP's client format: a length-prefixed
// array of length-prefixed bulk strings.
func encodeCommand(args ...string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "*%d\r\n", len(args))
	for _, a := range args {
		fmt.Fprintf(&b, "$%d\r\n%s\r\n", len(a), a)
	}
	return []byte(b.String())
}

// value is one parsed RESP reply. Only the kinds pub/sub produces are
// modelled: arrays, bulk strings and simple strings. Integers and errors are
// not kept as values - an error is returned as an error, and the only integer
// this client ever receives is PUBLISH's subscriber count, which it does not
// act on.
type value struct {
	kind  byte
	text  string
	items []value
}

func (v value) isArray() bool { return v.kind == '*' }

// readValue parses one reply. It is stricter than the ping-pong client in
// internal/ratelimit - which only ever needs an integer back - because pub/sub
// pushes arrive as three-element arrays of bulk strings, and a subscription
// that silently mis-parsed them would look healthy while delivering nothing.
func readValue(rd *bufio.Reader) (value, error) {
	line, err := rd.ReadString('\n')
	if err != nil {
		return value{}, err
	}
	if len(line) < 3 || line[len(line)-2] != '\r' {
		return value{}, fmt.Errorf("cache: malformed RESP line %q", line)
	}
	body := line[1 : len(line)-2]

	switch line[0] {
	case '*':
		n, err := strconv.Atoi(body)
		if err != nil {
			return value{}, fmt.Errorf("cache: malformed RESP array %q", body)
		}
		if n < 0 {
			return value{kind: '*'}, nil
		}
		items := make([]value, 0, n)
		for i := 0; i < n; i++ {
			item, err := readValue(rd)
			if err != nil {
				return value{}, err
			}
			items = append(items, item)
		}
		return value{kind: '*', items: items}, nil
	case '$':
		n, err := strconv.Atoi(body)
		if err != nil {
			return value{}, fmt.Errorf("cache: malformed RESP bulk string %q", body)
		}
		if n < 0 {
			return value{kind: '$'}, nil
		}
		buf := make([]byte, n+2)
		if _, err := readFull(rd, buf); err != nil {
			return value{}, err
		}
		return value{kind: '$', text: string(buf[:n])}, nil
	case '+':
		return value{kind: '+', text: body}, nil
	case ':':
		if _, err := strconv.ParseInt(body, 10, 64); err != nil {
			return value{}, fmt.Errorf("cache: malformed RESP integer %q", body)
		}
		return value{kind: ':', text: body}, nil
	case '-':
		return value{}, fmt.Errorf("cache: redis: %s", body)
	default:
		return value{}, fmt.Errorf("cache: unexpected RESP type %q", line[0])
	}
}

func readFull(rd *bufio.Reader, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := rd.Read(buf[total:])
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}
