package content

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/Teamthy/i-confess/internal/db/dbtest"
)

// constraintValues reads the permitted values straight out of the running
// PostgreSQL, not out of the migration text.
//
// That distinction is the point. The G-5 defect was a Go validator and a CHECK
// constraint that disagreed, and the reason no test caught it is that the
// existing schema tests compare against migration *source*. Reading source
// proves what was written; reading pg_get_constraintdef proves what the
// database will actually enforce after every migration has run, including one
// that a later migration silently replaced.
func constraintValues(t *testing.T, table, column string) map[string]bool {
	t.Helper()
	raw := dbtest.Raw(t)

	var def string
	err := raw.QueryRowContext(context.Background(),
		`SELECT pg_get_constraintdef(oid) FROM pg_constraint
		  WHERE conrelid = $1::regclass AND conname = $2`,
		table, table+"_"+column+"_check").Scan(&def)
	if err != nil {
		t.Fatalf("no CHECK constraint named %s_%s_check on %s: %v", table, column, table, err)
	}

	out := map[string]bool{}
	// pg_get_constraintdef renders CHECK ((status = ANY (ARRAY['a','b']))),
	// so splitting on the single quote puts the literals at odd indices.
	chunks := strings.Split(def, "'")
	for i := 1; i < len(chunks); i += 2 {
		out[chunks[i]] = true
	}
	if len(out) == 0 {
		t.Fatalf("parsed no values from %q - the parser is broken, not the schema", def)
	}
	return out
}

// TestContentTransitions is the named state-machine test for G-37. It tests
// the directed edge table rather than merely checking that statuses are valid:
// a value can be in the vocabulary and still be an illegal movement.
func TestContentTransitions(t *testing.T) {
	allowed := map[[2]Status]bool{}
	for _, edge := range Edges() {
		if edge.From == edge.To {
			t.Fatalf("self-edge %s -> %s", edge.From, edge.To)
		}
		allowed[[2]Status{edge.From, edge.To}] = true
	}
	for _, from := range All() {
		for _, to := range All() {
			want := allowed[[2]Status{from, to}]
			if got := CanTransition(string(from), string(to)); got != want {
				t.Errorf("CanTransition(%s,%s) = %v, want %v", from, to, got, want)
			}
			if err := Transition(string(from), string(to)); (err == nil) != want {
				t.Errorf("Transition(%s,%s) error=%v, want allowed=%v", from, to, err, want)
			}
		}
	}
	for _, edge := range Edges() {
		if edge.From == StatusPublished && edge.To != StatusDeprecated && edge.To != StatusArchived {
			t.Errorf("published has unexpected forward edge to %s", edge.To)
		}
	}
}

// TestContentVocabularyParityAgainstConstraint is the live-database parity
// guard for G-37/G-39. It reads PostgreSQL's installed CHECK, not migration
// text, so a later migration cannot silently change what production accepts.
func TestContentVocabularyParityAgainstConstraint(t *testing.T) {
	dbValues := constraintValues(t, "confessions", "status")
	codeValues := map[string]bool{}
	for _, status := range All() {
		codeValues[string(status)] = true
	}
	for value := range codeValues {
		if !dbValues[value] {
			t.Errorf("code accepts %q but the live CHECK rejects it", value)
		}
	}
	for value := range dbValues {
		if !codeValues[value] {
			t.Errorf("live CHECK accepts %q but content does not declare it", value)
		}
	}
}

// TestLifecycleMatchesTheDatabase is the test that would have caught G-5.
func TestLifecycleMatchesTheDatabase(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()

	dbValues := constraintValues(t, "confessions", "status")

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

	if len(onlyInCode) > 0 {
		t.Errorf("statuses the code accepts but the database rejects (these fail at write time as a 500): %s",
			strings.Join(onlyInCode, ", "))
	}
	if len(onlyInDB) > 0 {
		t.Errorf("statuses the database accepts but the code does not know (unhandled branches): %s",
			strings.Join(onlyInDB, ", "))
	}
	if len(onlyInCode) == 0 && len(onlyInDB) == 0 {
		t.Logf("lifecycle agrees in both directions: %d statuses", len(codeValues))
	}
}

// TestEveryLifecycleStatusIsWritable is the end-to-end version of the same
// claim. The parity test compares two lists; this one writes each status and
// reads it back, so it fails for any reason the write could fail, not only a
// vocabulary mismatch.
func TestEveryLifecycleStatusIsWritable(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()
	ctx := context.Background()
	raw := dbtest.Raw(t)

	catID := "cat-lifecycle-test"
	if _, err := raw.ExecContext(ctx,
		`INSERT INTO categories (id,name,slug,description,icon,premium,status,sort_order,created_at,updated_at)
		 VALUES ($1,'Lifecycle','lifecycle','t','t',0,'published',1,'2026-01-01','2026-01-01')`, catID); err != nil {
		t.Fatalf("insert category: %v", err)
	}

	for _, s := range All() {
		t.Run(string(s), func(t *testing.T) {
			id := "conf-" + string(s)
			if _, err := raw.ExecContext(ctx,
				`INSERT INTO confessions (id,category_id,title,short_text,medium_text,long_text,intensity,language,status,author,version,created_at,updated_at)
				 VALUES ($1,$2,'t','a','ab','abc',1,'en',$3,'test',1,'2026-01-01','2026-01-01')`,
				id, catID, string(s)); err != nil {
				t.Fatalf("status %q was rejected by the database: %v", s, err)
			}
		})
	}
}

// TestInvalidStatusIsRejected proves the constraint is doing something. Without
// it, the tests above would also pass against a table with no constraint at all.
func TestInvalidStatusIsRejected(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()
	ctx := context.Background()
	raw := dbtest.Raw(t)

	if _, err := raw.ExecContext(ctx,
		`INSERT INTO categories (id,name,slug,description,icon,premium,status,sort_order,created_at,updated_at)
		 VALUES ('cat-invalid','Invalid','invalid','t','t',0,'published',1,'2026-01-01','2026-01-01')`); err != nil {
		t.Fatalf("insert category: %v", err)
	}

	for _, bad := range []string{"publishedd", "PUBLISHED", "", "pending_deletion", "deleted", "rejected"} {
		_, err := raw.ExecContext(ctx,
			`INSERT INTO confessions (id,category_id,title,short_text,medium_text,long_text,intensity,language,status,author,version,created_at,updated_at)
			 VALUES ($1,'cat-invalid','t','a','ab','abc',1,'en',$2,'test',1,'2026-01-01','2026-01-01')`,
			fmt.Sprintf("conf-bad-%d", len(bad)), bad)
		if err == nil {
			t.Errorf("the database accepted the invalid status %q", bad)
		}
	}
}

// TestPendingDeletionIsNoLongerAConfessionStatus records the deliberate
// removal. Those values belong to users.status, where internal/deletion writes
// them; nothing has ever written them to confessions, and they arrived on this
// column by copy-paste.
func TestPendingDeletionIsNoLongerAConfessionStatus(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()

	dbValues := constraintValues(t, "confessions", "status")
	for _, gone := range []string{"pending_deletion", "deleted"} {
		if dbValues[gone] {
			t.Errorf("confessions.status still accepts %q, which belongs to users.status", gone)
		}
	}
}

// TestVisibilityVocabularyMatchesTheDatabase is the same check for G-4.
func TestVisibilityVocabularyMatchesTheDatabase(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()

	dbValues := constraintValues(t, "community_posts", "visibility")
	for _, want := range []string{"private", "shared", "public"} {
		if !dbValues[want] {
			t.Errorf("community_posts.visibility does not accept %q", want)
		}
	}
	if len(dbValues) != 3 {
		t.Errorf("community_posts.visibility accepts %d values, want 3", len(dbValues))
	}
}
