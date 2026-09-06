package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"

	"github.com/Teamthy/i-confess/internal/api"
	"github.com/Teamthy/i-confess/internal/db"
)

// Emits the OpenAPI document from the live route table so the checked-in spec
// can never disagree with the server.
func main() {
	// Generating the spec walks the route table; it never issues a query.
	// sql.Open validates the driver and DSN but does not dial, so this works
	// without a reachable PostgreSQL - which matters, because requiring a live
	// database just to emit a document would make the spec unregenerable in a
	// bare CI checkout.
	raw, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		panic(err)
	}
	conn := db.NewDB(raw)
	defer conn.Close()

	h := api.NewHandler(api.Config{JWTSecret: "spec", TokenTTL: "1h"}, conn)
	h.BuildEngine()
	h.Routes() // registering populates the route table

	spec := h.OpenAPISpec("https://api.iconfess.app")
	out, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(os.Args[1], append(out, '\n'), 0o644); err != nil {
		panic(err)
	}
	fmt.Printf("wrote %s (%d bytes, %d paths)\n", os.Args[1], len(out), len(spec["paths"].(map[string]any)))
}
