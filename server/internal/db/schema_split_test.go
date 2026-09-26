package db

import (
	"database/sql"
	"strings"
	"testing"
)

type recordingSchemaExecutor struct{ statements []string }

func (r *recordingSchemaExecutor) Exec(query string, _ ...any) (sql.Result, error) {
	r.statements = append(r.statements, query)
	return nil, nil
}

func TestInitSchemaDoesNotSplitInsideSQLData(t *testing.T) {
	var recorder recordingSchemaExecutor
	script := `-- migration comment; is not SQL
INSERT INTO notes(value) VALUES ('Editorial; source ''quoted''; more'); -- inline; comment
/* block; /* nested; */ comment; */ INSERT INTO "a;b" VALUES ($body$keep; -- here$body$);
SELECT 'last';`
	if err := InitSchema(&recorder, script); err != nil {
		t.Fatal(err)
	}
	if len(recorder.statements) != 3 {
		t.Fatalf("got %d statements: %#v", len(recorder.statements), recorder.statements)
	}
	if !strings.Contains(recorder.statements[0], "Editorial; source ''quoted''; more") {
		t.Errorf("editorial note was split or rewritten: %q", recorder.statements[0])
	}
	if !strings.Contains(recorder.statements[1], `"a;b"`) || !strings.Contains(recorder.statements[1], "$body$keep; -- here$body$") {
		t.Errorf("quoted identifier or function body was split: %q", recorder.statements[1])
	}
}

func TestInitSchemaRejectsUnterminatedSQL(t *testing.T) {
	for _, script := range []string{"SELECT 'bad", "SELECT $body$bad", "SELECT 1 /* bad"} {
		var recorder recordingSchemaExecutor
		if err := InitSchema(&recorder, script); err == nil {
			t.Errorf("accepted %q", script)
		}
		if len(recorder.statements) != 0 {
			t.Errorf("executed partial migration: %#v", recorder.statements)
		}
	}
}
