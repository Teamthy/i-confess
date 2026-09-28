// Command bible-registry emits the reviewed translation registry as JSON.
//
// The registry in internal/bible/registry.go is the source of truth for
// every edition the platform has reviewed — identity, coverage, licence and
// the exact file and SHA-256 its text came from. The web app renders it on
// the translations page as a "reviewed registry" so a reader can see what is
// available beyond the catalogue this deployment imports, which means the
// browser needs the same table before or without the API.
//
// Usage, from server/:
//
//	go run ./cmd/bible-registry ../web/lib/versions.json
//
// With no argument (or "-") it writes to stdout. The web file may also be
// written by scripts/gen-bible-registry.py, which parses registry.go
// directly; TestWebRegistryJSONIsCurrent in internal/bible compares the
// checked-in file by decoded content, so both generators are valid.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Teamthy/i-confess/internal/bible"
)

func main() {
	document, err := bible.RegistryJSON()
	if err != nil {
		fmt.Fprintln(os.Stderr, "bible-registry:", err)
		os.Exit(1)
	}
	if len(os.Args) < 2 || os.Args[1] == "-" {
		if _, err := os.Stdout.Write(document); err != nil {
			fmt.Fprintln(os.Stderr, "bible-registry:", err)
			os.Exit(1)
		}
		return
	}
	if err := os.WriteFile(os.Args[1], document, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "bible-registry:", err)
		os.Exit(1)
	}
	var registry bible.RegistryDocument
	if err := json.Unmarshal(document, &registry); err != nil {
		fmt.Fprintln(os.Stderr, "bible-registry:", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "bible-registry: wrote %s (%d versions, %d languages)\n",
		os.Args[1], registry.VersionCount, registry.LanguageCount)
}
