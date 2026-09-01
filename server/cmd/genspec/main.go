package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Teamthy/i-confess/internal/api"
	"github.com/Teamthy/i-confess/internal/db"
)

// Emits the OpenAPI document from the live route table so the checked-in spec
// can never disagree with the server.
func main() {
	conn, err := db.Open(":memory:")
	if err != nil {
		panic(err)
	}
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
