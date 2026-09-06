package db

import "testing"

func TestRebind(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  string
	}{
		{
			name:  "no placeholders is returned unchanged",
			query: "SELECT id FROM users",
			want:  "SELECT id FROM users",
		},
		{
			name:  "sequential placeholders",
			query: "SELECT * FROM t WHERE a = ? AND b = ? AND c = ?",
			want:  "SELECT * FROM t WHERE a = $1 AND b = $2 AND c = $3",
		},
		{
			name:  "placeholder order follows position, not argument meaning",
			query: "INSERT INTO t (a, b) VALUES (?, ?) ON CONFLICT (a) DO UPDATE SET b = ?",
			want:  "INSERT INTO t (a, b) VALUES ($1, $2) ON CONFLICT (a) DO UPDATE SET b = $3",
		},
		{
			name:  "question mark inside a single-quoted literal is data",
			query: "SELECT * FROM t WHERE note = 'why?' AND id = ?",
			want:  "SELECT * FROM t WHERE note = 'why?' AND id = $1",
		},
		{
			name:  "escaped quote inside a literal does not end it",
			query: "SELECT * FROM t WHERE note = 'it''s here?' AND id = ?",
			want:  "SELECT * FROM t WHERE note = 'it''s here?' AND id = $1",
		},
		{
			name:  "question mark in a line comment does not consume a number",
			query: "SELECT * FROM t -- why?\nWHERE id = ?",
			want:  "SELECT * FROM t -- why?\nWHERE id = $1",
		},
		{
			name:  "question mark in a block comment does not consume a number",
			query: "SELECT /* TODO: why? */ * FROM t WHERE id = ? AND n = ?",
			want:  "SELECT /* TODO: why? */ * FROM t WHERE id = $1 AND n = $2",
		},
		{
			name:  "question mark in a dollar-quoted body is untouched",
			query: "DO $$ BEGIN RAISE NOTICE 'why?'; END $$; SELECT ?",
			want:  "DO $$ BEGIN RAISE NOTICE 'why?'; END $$; SELECT $1",
		},
		{
			name:  "tagged dollar quoting",
			query: "SELECT $fn$ is it? $fn$ AS q, ? AS p",
			want:  "SELECT $fn$ is it? $fn$ AS q, $1 AS p",
		},
		{
			name:  "question mark in a double-quoted identifier",
			query: `SELECT "col?" FROM t WHERE id = ?`,
			want:  `SELECT "col?" FROM t WHERE id = $1`,
		},
		{
			name:  "existing positional parameters are left alone",
			query: "SELECT * FROM t WHERE a = $1 AND b = $2",
			want:  "SELECT * FROM t WHERE a = $1 AND b = $2",
		},
		{
			name:  "a real store query shape",
			query: "UPDATE session_items SET status = ? WHERE id = ? AND session_id = ?",
			want:  "UPDATE session_items SET status = $1 WHERE id = $2 AND session_id = $3",
		},
		{
			name:  "empty query",
			query: "",
			want:  "",
		},
		{
			name:  "unterminated literal does not panic",
			query: "SELECT 'abc",
			want:  "SELECT 'abc",
		},
		{
			name:  "unterminated dollar quote does not panic",
			query: "SELECT $$ abc",
			want:  "SELECT $$ abc",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Rebind(tc.query); got != tc.want {
				t.Errorf("Rebind()\n got: %s\nwant: %s", got, tc.want)
			}
		})
	}
}

// TestRebindDoesNotShiftNumbering is the property that a naive
// strings.ReplaceAll gets wrong, and the reason the scanner tracks literals and
// comments at all: a '?' that is not a placeholder must not advance the
// counter, or every later parameter binds to the wrong argument. That presents
// as corrupted data rather than a syntax error, so it is worth its own test.
func TestRebindDoesNotShiftNumbering(t *testing.T) {
	const query = `
-- what is this for?
SELECT id FROM confessions
WHERE category_id = ?          /* which category? */
  AND body <> 'is it enough?'
  AND status = ?`

	got := Rebind(query)

	// Exactly two placeholders exist, so exactly $1 and $2 may appear.
	if n := countOccurrences(got, "$1"); n != 1 {
		t.Errorf("expected exactly one $1, got %d in:\n%s", n, got)
	}
	if n := countOccurrences(got, "$2"); n != 1 {
		t.Errorf("expected exactly one $2, got %d in:\n%s", n, got)
	}
	for _, bad := range []string{"$3", "$4"} {
		if countOccurrences(got, bad) != 0 {
			t.Errorf("unexpected %s — a non-placeholder '?' consumed a number:\n%s", bad, got)
		}
	}
	// The comment and the literal must survive verbatim.
	for _, keep := range []string{"-- what is this for?", "/* which category? */", "'is it enough?'"} {
		if countOccurrences(got, keep) != 1 {
			t.Errorf("lost %q in:\n%s", keep, got)
		}
	}
}

func TestRebindIsIdempotent(t *testing.T) {
	const query = "SELECT * FROM t WHERE a = ? AND note = 'x?' AND b = ?"
	once := Rebind(query)
	if twice := Rebind(once); twice != once {
		t.Errorf("Rebind is not idempotent:\n once: %s\ntwice: %s", once, twice)
	}
}

// TestRebindMatchesStoreUsage guards against the rebinding drifting from the
// SQL the store layer actually contains: every '?' in a real query must become
// a numbered parameter.
func TestRebindMatchesStoreUsage(t *testing.T) {
	q := "INSERT INTO user_preferences (user_id, default_duration, default_voice_id) " +
		"VALUES (?, COALESCE((SELECT default_duration FROM user_preferences WHERE user_id = ?), 1800), '')"
	want := "INSERT INTO user_preferences (user_id, default_duration, default_voice_id) " +
		"VALUES ($1, COALESCE((SELECT default_duration FROM user_preferences WHERE user_id = $2), 1800), '')"
	if got := Rebind(q); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func countOccurrences(s, sub string) int {
	n := 0
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			n++
		}
	}
	return n
}
