package sessions

import (
	"regexp"
	"sort"
	"testing"

	"github.com/Teamthy/i-confess/internal/db"
)

// The session vocabulary lives in two places that can drift: the State
// constants here, and the CHECK constraint on sessions.status in the schema.
// Drift is not a cosmetic problem in either direction. A constant the database
// rejects makes a legal transition fail at write time, in production, on the
// one table the product's retention metric is derived from. A value the
// database accepts but the code does not know is an unhandled branch.
//
// Nothing else compares the two. The state machine tests validate the graph
// against itself, and the schema tests count constraints rather than reading
// them, so both suites pass while the two vocabularies disagree.

// The constraint is declared inline in the CREATE TABLE, not as a named
// ALTER TABLE, so it has to be located inside the sessions block. Reading the
// whole schema for "status IN (...)" would match the partial index on
// voice_rights and several UPDATE statements that mention legacy spellings.
var (
	sessionsTable = regexp.MustCompile(`(?is)CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?sessions\s*\((.*?)\n\)`)
	statusCheck   = regexp.MustCompile(`(?is)CHECK\s*\(\s*status\s+IN\s*\(([^)]*)\)`)
	valueLit      = regexp.MustCompile(`'([^']*)'`)
)

// sessionStatusValues returns the set of status values the database accepts.
func sessionStatusValues(t *testing.T) map[string]bool {
	t.Helper()
	tbl := sessionsTable.FindStringSubmatch(db.SchemaPostgresSQL)
	if tbl == nil {
		t.Fatal("no CREATE TABLE sessions block found in the schema")
	}
	chk := statusCheck.FindStringSubmatch(tbl[1])
	if chk == nil {
		t.Fatal("the sessions table has no CHECK on status; the constraint was removed or moved")
	}
	out := map[string]bool{}
	for _, v := range valueLit.FindAllStringSubmatch(chk[1], -1) {
		out[v[1]] = true
	}
	if len(out) == 0 {
		t.Fatal("parsed no values out of the CHECK constraint - the parser is broken, not the schema")
	}
	return out
}

func TestDomainVocabularyMatchesTheDatabase(t *testing.T) {
	dbValues := sessionStatusValues(t)

	codeValues := map[string]bool{}
	for _, s := range All() {
		codeValues[string(s)] = true
	}

	var onlyInCode, onlyInDB []string
	for v := range codeValues {
		if !dbValues[v] {
			onlyInCode = append(onlyInCode, v)
		}
	}
	for v := range dbValues {
		if !codeValues[v] {
			onlyInDB = append(onlyInDB, v)
		}
	}
	sort.Strings(onlyInCode)
	sort.Strings(onlyInDB)

	for _, v := range onlyInCode {
		t.Errorf("state %q exists in Go but the database would reject it - a legal transition would fail at write time", v)
	}
	for _, v := range onlyInDB {
		t.Errorf("state %q is accepted by the database but unknown to Go - it would arrive as an unhandled value", v)
	}

	t.Logf("session vocabulary agrees across Go and PostgreSQL: %d states", len(codeValues))
}

// TestEveryTransitionTargetIsPersistable is the sharper version of the same
// concern. All() covers the declared vocabulary, but the transition table is
// what actually gets written, so it is checked independently. A state reachable
// by an edge but absent from the CHECK constraint would be a write failure that
// only appears once a user takes that path.
func TestEveryTransitionTargetIsPersistable(t *testing.T) {
	allowed := sessionStatusValues(t)

	for from, targets := range transitions {
		if !allowed[string(from)] {
			t.Errorf("transition source %q is not accepted by the database", from)
		}
		for _, to := range targets {
			if !allowed[string(to)] {
				t.Errorf("transition %s -> %s would be rejected by sessions_status_check", from, to)
			}
		}
	}
}

// TestLegacySpellingsAreNotPersistable documents a deliberate asymmetry: the
// code accepts legacy spellings on the way in via Normalize, but the database
// must not accept them, because a normalised value is the only one that should
// ever be written. If the constraint ever widens to include a legacy spelling,
// unnormalised data becomes possible and the migration that cleaned it up is
// quietly undone.
func TestLegacySpellingsAreNotPersistable(t *testing.T) {
	allowed := sessionStatusValues(t)
	// The check is against the accepted set, not the schema text: the schema
	// contains an UPDATE that folds legacy spellings onto canonical states, so
	// searching the file for the literal would match that instead.

	for _, legacy := range []string{"created", "playing", "abandoned", "queued"} {
		if allowed[legacy] {
			t.Errorf("the database accepts the legacy spelling %q; it should only accept normalised states", legacy)
		}
	}
}
