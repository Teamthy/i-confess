package ratelimit

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Redis-backed rate limiting (§20, §55).
//
// The in-memory limiter is correct for one process and silently wrong the
// moment a second replica starts: each instance would allow the full burst, so
// N replicas multiply every limit by N. Shared counters fix that.
//
// This is a deliberately minimal RESP client rather than a full Redis library.
// Rate limiting needs exactly two commands (INCR and EXPIRE, pipelined), and
// hand-rolling that is a few dozen lines against a stable wire protocol —
// cheaper than taking a dependency for 1% of its surface. If the product later
// needs pub/sub, cluster support or Lua, swap in a real client behind Store;
// nothing above this file changes.

// Store is the shared counter backend.
type Store interface {
	// Incr increments key and returns the new count, setting an expiry on
	// first write so the window resets on its own.
	Incr(key string, window time.Duration) (int, error)
	// Del clears a key.
	Del(key string) error
	// Close releases resources.
	Close() error
}

// RedisStore is a small connection-pooled RESP client.
type RedisStore struct {
	addr     string
	password string

	mu    sync.Mutex
	conns []net.Conn

	// MaxIdle bounds the pool.
	MaxIdle int
	// Timeout bounds a single command.
	Timeout time.Duration
}

// NewRedisStore creates a store. It does not dial: the first command connects,
// so a Redis outage at boot does not prevent the service from starting.
func NewRedisStore(addr, password string) *RedisStore {
	return &RedisStore{
		addr: addr, password: password,
		MaxIdle: 8, Timeout: 2 * time.Second,
	}
}

func (r *RedisStore) get() (net.Conn, error) {
	r.mu.Lock()
	if n := len(r.conns); n > 0 {
		c := r.conns[n-1]
		r.conns = r.conns[:n-1]
		r.mu.Unlock()
		return c, nil
	}
	r.mu.Unlock()

	c, err := net.DialTimeout("tcp", r.addr, r.Timeout)
	if err != nil {
		return nil, err
	}
	if r.password != "" {
		if _, err := roundTrip(c, r.Timeout, "AUTH", r.password); err != nil {
			c.Close()
			return nil, err
		}
	}
	return c, nil
}

func (r *RedisStore) put(c net.Conn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.conns) >= r.MaxIdle {
		c.Close()
		return
	}
	r.conns = append(r.conns, c)
}

// Incr implements Store.
func (r *RedisStore) Incr(key string, window time.Duration) (int, error) {
	c, err := r.get()
	if err != nil {
		return 0, err
	}

	// INCR then EXPIRE with NX, so the TTL is set once per window rather than
	// being pushed forward by every request. Without NX a steady stream of
	// traffic would keep renewing the expiry and the window would never reset.
	secs := int(window.Seconds())
	if secs < 1 {
		secs = 1
	}
	n, err := roundTripPipeline(c, r.Timeout,
		[]string{"INCR", key},
		[]string{"EXPIRE", key, strconv.Itoa(secs), "NX"},
	)
	if err != nil {
		c.Close()
		return 0, err
	}
	r.put(c)
	return n, nil
}

// Del implements Store.
func (r *RedisStore) Del(key string) error {
	c, err := r.get()
	if err != nil {
		return err
	}
	if _, err := roundTrip(c, r.Timeout, "DEL", key); err != nil {
		c.Close()
		return err
	}
	r.put(c)
	return nil
}

// Close implements Store.
func (r *RedisStore) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.conns {
		c.Close()
	}
	r.conns = nil
	return nil
}

// Ping verifies connectivity, for readiness checks (§85).
func (r *RedisStore) Ping() error {
	c, err := r.get()
	if err != nil {
		return err
	}
	if _, err := roundTrip(c, r.Timeout, "PING"); err != nil {
		c.Close()
		return err
	}
	r.put(c)
	return nil
}

// ---------------------------------------------------------------------------
// RESP wire protocol
// ---------------------------------------------------------------------------

func encodeCommand(args ...string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "*%d\r\n", len(args))
	for _, a := range args {
		fmt.Fprintf(&b, "$%d\r\n%s\r\n", len(a), a)
	}
	return []byte(b.String())
}

func roundTrip(c net.Conn, timeout time.Duration, args ...string) (int, error) {
	if err := c.SetDeadline(time.Now().Add(timeout)); err != nil {
		return 0, err
	}
	if _, err := c.Write(encodeCommand(args...)); err != nil {
		return 0, err
	}
	return readReply(bufio.NewReader(c))
}

// roundTripPipeline writes several commands then reads all replies, returning
// the first. Pipelining keeps the limiter to one network round trip, which
// matters when it runs on every authentication request.
func roundTripPipeline(c net.Conn, timeout time.Duration, cmds ...[]string) (int, error) {
	if err := c.SetDeadline(time.Now().Add(timeout)); err != nil {
		return 0, err
	}
	var buf []byte
	for _, cmd := range cmds {
		buf = append(buf, encodeCommand(cmd...)...)
	}
	if _, err := c.Write(buf); err != nil {
		return 0, err
	}

	rd := bufio.NewReader(c)
	first := 0
	for i := range cmds {
		n, err := readReply(rd)
		if err != nil {
			return 0, err
		}
		if i == 0 {
			first = n
		}
	}
	return first, nil
}

// readReply parses the RESP subset these commands produce: integers, simple
// strings, errors and nil bulk strings.
func readReply(rd *bufio.Reader) (int, error) {
	line, err := rd.ReadString('\n')
	if err != nil {
		return 0, err
	}
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		return 0, errors.New("empty reply")
	}
	switch line[0] {
	case ':':
		return strconv.Atoi(line[1:])
	case '+':
		return 0, nil
	case '-':
		return 0, fmt.Errorf("redis: %s", line[1:])
	case '$':
		n, err := strconv.Atoi(line[1:])
		if err != nil {
			return 0, err
		}
		if n < 0 {
			return 0, nil
		}
		body := make([]byte, n+2)
		if _, err := readFull(rd, body); err != nil {
			return 0, err
		}
		return 0, nil
	default:
		return 0, fmt.Errorf("unexpected reply type %q", line[0])
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

// ---------------------------------------------------------------------------
// Distributed limiter
// ---------------------------------------------------------------------------

// Distributed enforces limits across replicas using a shared Store.
//
// It falls back to a local limiter when the store is unreachable. That is a
// deliberate availability choice: if Redis is down, throttling degrades to
// per-instance rather than the service refusing all logins. A rate limiter that
// takes the site down when its backing store fails is worse than one that
// briefly enforces N× the intended limit.
type Distributed struct {
	store    Store
	fallback *Limiter

	mu       sync.Mutex
	degraded bool
}

// NewDistributed wraps a Store.
func NewDistributed(store Store) *Distributed {
	return &Distributed{store: store, fallback: New()}
}

// Allow reports whether an event is within the rule.
func (d *Distributed) Allow(key string, rule Rule) (bool, time.Duration) {
	if rule.Burst <= 0 || rule.Window <= 0 {
		return true, 0
	}
	if d.store == nil {
		return d.fallback.Allow(key, rule)
	}

	n, err := d.store.Incr("rl:"+key, rule.Window)
	if err != nil {
		d.setDegraded(true)
		return d.fallback.Allow(key, rule)
	}
	d.setDegraded(false)

	if n > rule.Burst {
		return false, rule.Window
	}
	return true, 0
}

// Reset clears a key in both the shared store and the local fallback.
func (d *Distributed) Reset(key string) {
	if d.store != nil {
		_ = d.store.Del("rl:" + key)
	}
	d.fallback.Reset(key)
}

// Degraded reports whether the shared store is currently unreachable, so it can
// be surfaced as a metric rather than failing silently (§84).
func (d *Distributed) Degraded() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.degraded
}

func (d *Distributed) setDegraded(v bool) {
	d.mu.Lock()
	d.degraded = v
	d.mu.Unlock()
}
