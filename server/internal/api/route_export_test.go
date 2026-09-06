package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Teamthy/i-confess/internal/db/dbtest"
)

// TestExportRouteTable writes the live route table to design/routes.json when
// EXPORT_ROUTES=1.
//
// The information architecture in design/ia.json names the endpoints each
// screen depends on. A screen that references an endpoint the backend does not
// have is a bug that surfaces as a spinner, usually in production. Exporting
// the table from the real handler — rather than transcribing it — means
// design/test_ia.py checks the IA against what is actually registered.
//
// It skips silently without the env var so the normal suite is unaffected.
func TestExportRouteTable(t *testing.T) {
	if os.Getenv("EXPORT_ROUTES") == "" {
		t.Skip("set EXPORT_ROUTES=1 to regenerate design/routes.json")
	}

	h := NewHandler(Config{JWTSecret: "test-secret-value", TokenTTL: "24h"}, dbtest.New(t))
	h.Routes() // populates the route table

	routes := h.RouteTable()
	// An empty table exports cleanly and proves nothing, so refuse to write it.
	if len(routes) == 0 {
		t.Fatal("route table is empty - Routes() recorded nothing, refusing to export")
	}

	type entry struct {
		Method  string `json:"method"`
		Path    string `json:"path"`
		Auth    string `json:"auth"`
		Summary string `json:"summary"`
		Tag     string `json:"tag"`
	}
	out := make([]entry, 0, len(routes))
	for _, r := range routes {
		out = append(out, entry(r))
	}

	payload, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		t.Fatalf("marshal route table: %v", err)
	}

	dest := os.Getenv("EXPORT_ROUTES_PATH")
	if dest == "" {
		t.Fatal("EXPORT_ROUTES_PATH is required")
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatalf("create dir: %v", err)
	}
	if err := os.WriteFile(dest, append(payload, '\n'), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Logf("wrote %d routes to %s", len(out), dest)
}
