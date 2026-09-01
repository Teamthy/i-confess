package deletion

import (
	"context"
	"database/sql"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/db"
	_ "modernc.org/sqlite"
)

var frozen = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

func newDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.InitSchema(conn, db.SchemaSQL); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func newService(t *testing.T, conn *sql.DB) *Service {
	t.Helper()
	s := NewService(conn)
	s.Now = func() time.Time { return frozen }
	return s
}

// seedUser creates an account with data spread across the tables deletion
// touches, so an erasure has something real to remove.
func seedUser(t *testing.T, conn *sql.DB, id, email string) {
	t.Helper()
	ctx := context.Background()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := conn.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}
	now := frozen.Format(time.RFC3339)

	exec(`INSERT INTO users (id,email,password_hash,display_name,timezone,status,created_at,updated_at)
	      VALUES (?,?,?,?,?,?,?,?)`,
		id, email, "$2a$10$abcdefghijklmnopqrstuv", "Grace Adeyemi", "Africa/Lagos", "active", now, now)

	exec(`INSERT INTO user_profiles (id,user_id,display_name,bio,timezone,locale,language,created_at,updated_at)
	      VALUES (?,?,?,?,?,?,?,?,?)`,
		"p-"+id, id, "Grace Adeyemi", "Walking in peace", "Africa/Lagos", "en-NG", "en", now, now)

	exec(`INSERT INTO user_preferences (id,user_id,updated_at) VALUES (?,?,?)`, "pref-"+id, id, now)
	exec(`INSERT INTO notification_preferences (user_id,updated_at) VALUES (?,?)`, id, now)
	exec(`INSERT INTO refresh_tokens (id,user_id,token_hash,expires_at,created_at)
	      VALUES (?,?,?,?,?)`, "rt-"+id, id, "hash-"+id, now, now)
	exec(`INSERT INTO user_devices (id,user_id,device_id,platform,last_seen_at,metadata,created_at)
	      VALUES (?,?,?,?,?,?,?)`, "d-"+id, id, "device-1", "ios", now, "push-token-secret", now)
	exec(`INSERT INTO user_collections (id,user_id,name,visibility,created_at,updated_at)
	      VALUES (?,?,?,?,?,?)`, "c-"+id, id, "Morning", "private", now, now)
	exec(`INSERT INTO favorites (id,user_id,entity_type,entity_id,created_at)
	      VALUES (?,?,?,?,?)`, "f-"+id, id, "confession", "conf-1", now)
	exec(`INSERT INTO playback_history (id,user_id,duration_seconds,completed,skipped,listened_at)
	      VALUES (?,?,?,?,?,?)`, "h-"+id, id, 120, 1, 0, now)
	exec(`INSERT INTO user_identities (id,user_id,provider,subject,email,created_at)
	      VALUES (?,?,?,?,?,?)`, "i-"+id, id, "google", "sub-"+id, email, now)

	// Retained categories.
	exec(`INSERT INTO subscriptions (id,user_id,plan,status,created_at)
	      VALUES (?,?,?,?,?)`, "s-"+id, id, "premium", "active", now)
	exec(`INSERT INTO consent_records (id,user_id,category,granted,version,created_at)
	      VALUES (?,?,?,?,?,?)`, "con-"+id, id, "terms", 1, "v1", now)
}

func count(t *testing.T, conn *sql.DB, table, where string, args ...any) int {
	t.Helper()
	var n int
	q := "SELECT COUNT(*) FROM " + table
	if where != "" {
		q += " WHERE " + where
	}
	if err := conn.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// ---------------------------------------------------------------------------
// Policy coverage
// ---------------------------------------------------------------------------

func TestPolicyIsWellFormed(t *testing.T) {
	if err := Validate(); err != nil {
		t.Fatal(err)
	}
}

// The load-bearing test: every table in the schema that holds a user reference
// must have an explicit policy. A table added later without one fails here
// rather than silently surviving a deletion request.
func TestEveryUserTableHasAPolicy(t *testing.T) {
	covered := map[string]bool{}
	for _, p := range Policies {
		covered[p.Table] = true
	}

	// Parse the schema for tables referencing users(id).
	tableRe := regexp.MustCompile(`CREATE TABLE IF NOT EXISTS ([a-z_]+)`)
	var current string
	var missing []string
	for _, line := range strings.Split(db.SchemaSQL, "\n") {
		if m := tableRe.FindStringSubmatch(line); m != nil {
			current = m[1]
		}
		if strings.Contains(line, "REFERENCES users(id)") && current != "" {
			// audit_logs and deletion_records deliberately outlive the account
			// and hold no foreign key to it.
			if current != "deletion_records" && !covered[current] {
				missing = append(missing, current)
			}
			current = ""
		}
	}

	if len(missing) > 0 {
		t.Fatalf("tables reference users(id) but have no deletion policy: %v\n"+
			"Add an explicit entry to deletion.Policies — erasure must never be accidental.", missing)
	}
}

// Retention must always be justified, or it is indistinguishable from an
// oversight.
func TestRetainedTablesStateAReason(t *testing.T) {
	found := 0
	for _, p := range Policies {
		if p.Action == Retain {
			found++
			if len(p.Reason) < 20 {
				t.Fatalf("table %q is retained with a thin reason: %q", p.Table, p.Reason)
			}
		}
	}
	if found == 0 {
		t.Fatal("expected at least one retained category (financial/consent records)")
	}
	if len(RetainedCategories()) != found {
		t.Fatal("RetainedCategories does not match the policy")
	}
}

// ---------------------------------------------------------------------------
// Request / cancel
// ---------------------------------------------------------------------------

// Requesting deletion must stop access immediately, not at the end of the grace
// period: an attacker who triggered it must not keep using the account.
func TestRequestRevokesSessionsImmediately(t *testing.T) {
	conn := newDB(t)
	s := newService(t, conn)
	seedUser(t, conn, "u1", "u1@example.com")

	if n := count(t, conn, "refresh_tokens", "user_id = ? AND revoked_at IS NULL", "u1"); n != 1 {
		t.Fatalf("expected a live session before the request, got %d", n)
	}

	st, err := s.Request(context.Background(), "u1", "no longer needed")
	if err != nil {
		t.Fatal(err)
	}
	if !st.Requested || st.ErasesAt == "" {
		t.Fatalf("status does not describe a scheduled deletion: %+v", st)
	}

	if n := count(t, conn, "refresh_tokens", "user_id = ? AND revoked_at IS NULL", "u1"); n != 0 {
		t.Fatal("sessions survived a deletion request")
	}
	var status string
	conn.QueryRow(`SELECT status FROM users WHERE id = ?`, "u1").Scan(&status)
	if status != "pending_deletion" {
		t.Fatalf("account status = %q, want pending_deletion", status)
	}
}

// The grace period is what makes deletion survivable when it was not the owner
// who asked.
func TestCancelRestoresAccount(t *testing.T) {
	conn := newDB(t)
	s := newService(t, conn)
	seedUser(t, conn, "u2", "u2@example.com")

	if _, err := s.Request(context.Background(), "u2", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.Cancel(context.Background(), "u2"); err != nil {
		t.Fatal(err)
	}

	st, err := s.StatusFor(context.Background(), "u2")
	if err != nil {
		t.Fatal(err)
	}
	if st.Requested || st.Deleted {
		t.Fatalf("deletion still scheduled after cancel: %+v", st)
	}

	var status string
	conn.QueryRow(`SELECT status FROM users WHERE id = ?`, "u2").Scan(&status)
	if status != "active" {
		t.Fatalf("status = %q, want active", status)
	}
	// The user's data must still be there.
	if count(t, conn, "user_profiles", "user_id = ?", "u2") != 1 {
		t.Fatal("cancelling deletion lost the profile")
	}
}

func TestRequestIsIdempotentAndCancelRequiresARequest(t *testing.T) {
	conn := newDB(t)
	s := newService(t, conn)
	seedUser(t, conn, "u3", "u3@example.com")

	if err := s.Cancel(context.Background(), "u3"); err != ErrNotRequested {
		t.Fatalf("cancel without a request: %v, want ErrNotRequested", err)
	}
	if _, err := s.Request(context.Background(), "u3", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Request(context.Background(), "u3", ""); err != ErrAlreadyRequested {
		t.Fatalf("double request: %v, want ErrAlreadyRequested", err)
	}
}

// ---------------------------------------------------------------------------
// Erasure
// ---------------------------------------------------------------------------

func TestErasureRemovesPersonalData(t *testing.T) {
	conn := newDB(t)
	s := newService(t, conn)
	seedUser(t, conn, "u4", "grace@example.com")

	if _, err := s.Request(context.Background(), "u4", "done with the app"); err != nil {
		t.Fatal(err)
	}
	report, err := s.Erase(context.Background(), "u4")
	if err != nil {
		t.Fatal(err)
	}
	if report.RowsDeleted == 0 {
		t.Fatal("erasure deleted nothing")
	}

	// Everything holding personal data must be gone.
	for _, table := range []string{
		"user_profiles", "user_preferences", "notification_preferences",
		"refresh_tokens", "user_devices", "user_collections",
		"favorites", "playback_history", "user_identities",
	} {
		if n := count(t, conn, table, "user_id = ?", "u4"); n != 0 {
			t.Fatalf("%s still holds %d rows after erasure", table, n)
		}
	}
}

// The identity must be destroyed, not merely flagged.
func TestErasureTombstonesTheIdentity(t *testing.T) {
	conn := newDB(t)
	s := newService(t, conn)
	seedUser(t, conn, "u5", "grace@example.com")

	s.Request(context.Background(), "u5", "")
	if _, err := s.Erase(context.Background(), "u5"); err != nil {
		t.Fatal(err)
	}

	var email, hash, status string
	var name sql.NullString
	err := conn.QueryRow(
		`SELECT email, password_hash, COALESCE(display_name,''), status FROM users WHERE id = ?`, "u5").
		Scan(&email, &hash, &name, &status)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(email, "grace@example.com") {
		t.Fatalf("original email survived: %q", email)
	}
	if !strings.HasSuffix(email, "@invalid") {
		// The tombstone must not resemble a real address someone could later
		// register and thereby inherit.
		t.Fatalf("tombstone email is not clearly unusable: %q", email)
	}
	if hash != "" {
		t.Fatal("password hash survived erasure")
	}
	if name.String != "" {
		t.Fatalf("display name survived: %q", name.String)
	}
	if status != "deleted" {
		t.Fatalf("status = %q, want deleted", status)
	}
}

// Two deleted accounts must not collide on the UNIQUE email column.
func TestTombstonesAreUnique(t *testing.T) {
	conn := newDB(t)
	s := newService(t, conn)
	seedUser(t, conn, "aaaa-1111", "one@example.com")
	seedUser(t, conn, "bbbb-2222", "two@example.com")

	for _, id := range []string{"aaaa-1111", "bbbb-2222"} {
		s.Request(context.Background(), id, "")
		if _, err := s.Erase(context.Background(), id); err != nil {
			t.Fatalf("erase %s: %v", id, err)
		}
	}

	var distinct int
	conn.QueryRow(`SELECT COUNT(DISTINCT email) FROM users WHERE status = 'deleted'`).Scan(&distinct)
	if distinct != 2 {
		t.Fatalf("tombstone emails collided: %d distinct for 2 accounts", distinct)
	}
}

// Retained records must survive — erasing them would destroy the evidence that
// the deletion itself was lawful, and the financial record tax law requires.
func TestRetainedRecordsSurvive(t *testing.T) {
	conn := newDB(t)
	s := newService(t, conn)
	seedUser(t, conn, "u6", "u6@example.com")

	s.Request(context.Background(), "u6", "")
	report, err := s.Erase(context.Background(), "u6")
	if err != nil {
		t.Fatal(err)
	}

	if n := count(t, conn, "subscriptions", "user_id = ?", "u6"); n != 1 {
		t.Fatalf("subscription record was destroyed (%d rows); tax obligations require it", n)
	}
	if n := count(t, conn, "consent_records", "user_id = ?", "u6"); n != 1 {
		t.Fatalf("consent record was destroyed (%d rows); it proves the deletion was lawful", n)
	}
	if len(report.Retained) == 0 {
		t.Fatal("report does not disclose what was retained")
	}
}

// A durable record must remain that the deletion happened, holding no personal
// data.
func TestDeletionIsRecorded(t *testing.T) {
	conn := newDB(t)
	s := newService(t, conn)
	seedUser(t, conn, "u7", "traceable@example.com")

	s.Request(context.Background(), "u7", "privacy")
	if _, err := s.Erase(context.Background(), "u7"); err != nil {
		t.Fatal(err)
	}

	var erasedAt sql.NullString
	var rows int
	var retained sql.NullString
	err := conn.QueryRow(
		`SELECT erased_at, rows_deleted, retained_categories FROM deletion_records WHERE user_ref = ?`, "u7").
		Scan(&erasedAt, &rows, &retained)
	if err != nil {
		t.Fatal(err)
	}
	if !erasedAt.Valid || erasedAt.String == "" {
		t.Fatal("deletion record has no erasure timestamp")
	}
	if rows == 0 {
		t.Fatal("deletion record reports no rows deleted")
	}
	if !retained.Valid || retained.String == "" {
		t.Fatal("deletion record does not state what was retained")
	}

	// The record itself must not contain the person.
	var all string
	conn.QueryRow(`SELECT COALESCE(user_ref,'') || COALESCE(reason,'') || COALESCE(tables_affected,'')
	               FROM deletion_records WHERE user_ref = ?`, "u7").Scan(&all)
	if strings.Contains(all, "traceable@example.com") {
		t.Fatal("deletion record retained the user's email")
	}
}

// Erasing one account must not touch another.
func TestErasureIsScopedToOneAccount(t *testing.T) {
	conn := newDB(t)
	s := newService(t, conn)
	seedUser(t, conn, "victim", "victim@example.com")
	seedUser(t, conn, "bystander", "bystander@example.com")

	s.Request(context.Background(), "victim", "")
	if _, err := s.Erase(context.Background(), "victim"); err != nil {
		t.Fatal(err)
	}

	for _, table := range []string{"user_profiles", "user_collections", "favorites", "playback_history"} {
		if n := count(t, conn, table, "user_id = ?", "bystander"); n != 1 {
			t.Fatalf("%s: bystander lost data during another user's erasure (%d rows)", table, n)
		}
	}
	var email string
	conn.QueryRow(`SELECT email FROM users WHERE id = ?`, "bystander").Scan(&email)
	if email != "bystander@example.com" {
		t.Fatalf("bystander's identity was altered: %q", email)
	}
}

func TestDoubleErasureIsRefused(t *testing.T) {
	conn := newDB(t)
	s := newService(t, conn)
	seedUser(t, conn, "u8", "u8@example.com")

	s.Request(context.Background(), "u8", "")
	if _, err := s.Erase(context.Background(), "u8"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Erase(context.Background(), "u8"); err != ErrAlreadyDeleted {
		t.Fatalf("second erase: %v, want ErrAlreadyDeleted", err)
	}
	if err := s.Cancel(context.Background(), "u8"); err != ErrAlreadyDeleted {
		t.Fatalf("cancel after erasure: %v, want ErrAlreadyDeleted", err)
	}
}

// ---------------------------------------------------------------------------
// Scheduling
// ---------------------------------------------------------------------------

// Only accounts past the grace period may be erased by the sweeper.
func TestDueForErasureRespectsGracePeriod(t *testing.T) {
	conn := newDB(t)
	s := newService(t, conn)
	seedUser(t, conn, "fresh", "fresh@example.com")
	seedUser(t, conn, "ripe", "ripe@example.com")

	s.Request(context.Background(), "fresh", "")

	// The second request is backdated past the grace period.
	old := frozen.Add(-GracePeriod - time.Hour)
	s.Now = func() time.Time { return old }
	s.Request(context.Background(), "ripe", "")
	s.Now = func() time.Time { return frozen }

	due, err := s.DueForErasure(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 || due[0] != "ripe" {
		t.Fatalf("due = %v, want exactly [ripe]", due)
	}
}

func TestDueExcludesAlreadyErased(t *testing.T) {
	conn := newDB(t)
	s := newService(t, conn)
	seedUser(t, conn, "u9", "u9@example.com")

	old := frozen.Add(-GracePeriod - time.Hour)
	s.Now = func() time.Time { return old }
	s.Request(context.Background(), "u9", "")
	s.Now = func() time.Time { return frozen }

	if _, err := s.Erase(context.Background(), "u9"); err != nil {
		t.Fatal(err)
	}
	due, _ := s.DueForErasure(context.Background(), 10)
	if len(due) != 0 {
		t.Fatalf("an already-erased account is still due: %v", due)
	}
}
