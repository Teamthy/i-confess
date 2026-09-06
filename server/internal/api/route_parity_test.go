package api

import (
	"strings"
	"testing"

	"github.com/Teamthy/i-confess/internal/db/dbtest"
)

// The router's own comment promises that every API route is also served under
// /v1/* for versioned clients. Nothing enforced it, and it had already broken:
// 22 routes existed only under /v1/, including the entire playback surface —
// start, pause, resume, complete, queue, progress, skip.
//
// The shipped Flutter client calls unprefixed paths exclusively (69 of them,
// zero under /v1/), so a client written against the documented convention would
// have received a 404 the moment it tried to play a session. It had not surfaced
// only because playback controls are not built in the client yet.
//
// The duplication is a deliberate compatibility measure, not an accident, so the
// fix is parity rather than removing one side. This test is what keeps it that
// way: adding an endpoint at one prefix and forgetting the other now fails the
// build instead of shipping a 404.
//
// It reads the live route table rather than the source file, so it checks what
// the mux actually serves. A source scan would pass on a route that was
// registered under a typo'd pattern.

func TestRouteParityBetweenPrefixes(t *testing.T) {
	h := NewHandler(Config{JWTSecret: "test-secret-value", TokenTTL: "24h"}, dbtest.New(t))
	h.BuildEngine()
	h.Routes() // populates the route table

	table := h.RouteTable()
	if len(table) == 0 {
		t.Fatal("route table is empty - Routes() did not record anything, so this test proves nothing")
	}

	type key struct{ method, path string }
	have := make(map[key]bool, len(table))
	for _, r := range table {
		have[key{r.Method, r.Path}] = true
	}

	strip := func(p string) string { return strings.Replace(p, "/v1/", "/", 1) }

	var v1Only, bareOnly []string
	for _, r := range table {
		if strings.HasPrefix(r.Path, "/v1/") {
			if !have[key{r.Method, strip(r.Path)}] {
				v1Only = append(v1Only, r.Method+" "+r.Path)
			}
		} else if !have[key{r.Method, "/v1" + r.Path}] {
			bareOnly = append(bareOnly, r.Method+" "+r.Path)
		}
	}

	for _, r := range v1Only {
		t.Errorf("%s exists only under /v1/; a client using the documented unprefixed path gets a 404", r)
	}
	for _, r := range bareOnly {
		t.Errorf("%s exists only unprefixed; versioned clients cannot reach it", r)
	}

	t.Logf("route table: %d registrations, %d distinct unprefixed endpoints", len(table), len(table)/2)
}

// TestRouteTableHasNoDuplicateRegistrations catches the other failure mode of
// maintaining two prefixes by hand: the same method and path registered twice.
// Go's ServeMux panics on a duplicate pattern at registration time, so this is
// cheap insurance that the panic stays a test failure rather than a crash on
// boot in production.
func TestRouteTableHasNoDuplicateRegistrations(t *testing.T) {
	h := NewHandler(Config{JWTSecret: "test-secret-value", TokenTTL: "24h"}, dbtest.New(t))
	h.BuildEngine()
	h.Routes()

	seen := map[string]int{}
	for _, r := range h.RouteTable() {
		seen[r.Method+" "+r.Path]++
	}
	for route, n := range seen {
		if n > 1 {
			t.Errorf("%s registered %d times", route, n)
		}
	}
}
