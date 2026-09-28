// Command bible-structure emits the canonical Bible structure as JSON.
//
// The web app needs the canon — books, sections, chapter counts, the
// reference verse distribution and the alias table its reference parser
// resolves against — before it has fetched anything, so that the reader can
// draw a complete Bible the moment it renders and validate a typed reference
// without a round trip. Hand-copying that table into TypeScript is how the
// two drift apart, so it is generated from internal/bible and checked in.
//
// Usage, from server/:
//
//	go run ./cmd/bible-structure ../web/lib/canon.json
//
// With no argument it writes to stdout. TestWebCanonJSONIsCurrent in
// internal/bible fails when the checked-in file no longer matches this output.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Teamthy/i-confess/internal/bible"
)

func main() {
	document, err := bible.StructureJSON()
	if err != nil {
		fmt.Fprintln(os.Stderr, "bible-structure:", err)
		os.Exit(1)
	}
	if len(os.Args) < 2 || os.Args[1] == "-" {
		if _, err := os.Stdout.Write(document); err != nil {
			fmt.Fprintln(os.Stderr, "bible-structure:", err)
			os.Exit(1)
		}
		return
	}
	if err := os.WriteFile(os.Args[1], document, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "bible-structure:", err)
		os.Exit(1)
	}
	var structure bible.Structure
	if err := json.Unmarshal(document, &structure); err != nil {
		fmt.Fprintln(os.Stderr, "bible-structure:", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "bible-structure: wrote %s (%d books, %d chapters, %d reference verses)\n",
		os.Args[1], structure.BookCount, structure.ChapterCount, structure.VerseCount)
}
