package db

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Three tables — subscription_plans, community_posts, community_reactions —
// were queried by Go code but existed only in server/migrations/postgres/, a
// directory nothing in the application applies. The server runs the embedded
// schema and nothing else, so a fresh deployment would have started cleanly and
// then failed on the first query that touched them.
//
// It went unnoticed because internal/community and internal/billing have no
// tests. A green suite over untested packages proves nothing about them, which
// is the failure mode this test removes: it checks the schema against the SQL
// the code actually issues, whether or not anything exercises it.
//
// Only string literals passed as arguments to database methods are inspected.
// That precision is not cosmetic. Scanning every literal in a file also matches
// route descriptions such as "Update profile", which a table-name regex reads as
// an UPDATE against a table called "profile", and doc comments where "from the
// database" reads as a table called "the". Anchoring on the call site is what
// makes the result trustworthy enough to fail a build over.

var (
	tableRefRe = regexp.MustCompile(`(?i)\b(?:FROM|INTO|UPDATE|JOIN)\s+([a-z_][a-z0-9_]*)`)
	createRe   = regexp.MustCompile(`(?i)CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?([a-z_][a-z0-9_]*)`)
	cteRe      = regexp.MustCompile(`(?i)\bWITH\s+([a-z_][a-z0-9_]*)\s+AS\s*\(`)
)

// dbMethods are the database/sql entry points that take SQL as an argument.
// db.DB and db.Tx mirror these names exactly, which is what makes matching on
// the method name alone sufficient.
var dbMethods = map[string]bool{
	"Query": true, "QueryContext": true,
	"QueryRow": true, "QueryRowContext": true,
	"Exec": true, "ExecContext": true,
	"Prepare": true, "PrepareContext": true,
}

// sqlKeywords can follow FROM/INTO/UPDATE/JOIN without naming a table.
var sqlKeywords = map[string]bool{
	"select": true, "set": true, "values": true, "where": true, "dual": true,
	"only": true, "lateral": true, "unnest": true,
}

func TestEveryTableReferencedInGoExistsInTheSchema(t *testing.T) {
	defined := map[string]bool{}
	for _, m := range createRe.FindAllStringSubmatch(SchemaPostgresSQL, -1) {
		defined[strings.ToLower(m[1])] = true
	}
	if len(defined) == 0 {
		t.Fatal("no CREATE TABLE statements found in the embedded schema - the parser is broken")
	}

	referenced := map[string]map[string]bool{}
	root := findServerRoot(t)
	filesScanned, queriesScanned := 0, 0

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		f, perr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if perr != nil {
			// An unparseable file is a build failure, which the compiler
			// reports far more clearly than this test could.
			return nil
		}
		filesScanned++
		rel, _ := filepath.Rel(root, path)

		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || !dbMethods[sel.Sel.Name] {
				return true
			}
			// The SQL is usually the first argument after the context, but
			// taking every literal argument is harmless and survives signature
			// differences between Query, Exec and Prepare.
			for _, arg := range call.Args {
				sqlText := literalText(arg)
				if sqlText == "" {
					continue
				}
				queriesScanned++
				recordTables(sqlText, rel, referenced)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}

	if queriesScanned == 0 {
		t.Fatalf("found no SQL in %d Go files - the call-site matcher is broken, so this test proves nothing", filesScanned)
	}

	var missing []string
	for name := range referenced {
		if !defined[name] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)

	for _, name := range missing {
		files := make([]string, 0, len(referenced[name]))
		for f := range referenced[name] {
			files = append(files, f)
		}
		sort.Strings(files)
		t.Errorf("table %q is queried but not defined in schema.postgres.sql (referenced in %s)",
			name, strings.Join(files, ", "))
	}

	t.Logf("scanned %d Go files, %d query arguments; schema defines %d tables, code references %d distinct names",
		filesScanned, queriesScanned, len(defined), len(referenced))
}

// literalText returns the SQL carried by an argument, following string
// concatenation so queries built with "WHERE x = ?" + cond are still seen. It
// returns "" for anything it cannot resolve, which is a deliberate blind spot:
// a fully dynamic query cannot be checked statically, and guessing would be
// worse than skipping.
func literalText(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.BasicLit:
		if v.Kind != token.STRING {
			return ""
		}
		s, err := strconv.Unquote(v.Value)
		if err != nil {
			return ""
		}
		return s
	case *ast.BinaryExpr:
		if v.Op == token.ADD {
			return literalText(v.X) + literalText(v.Y)
		}
	case *ast.ParenExpr:
		return literalText(v.X)
	}
	return ""
}

func recordTables(sqlText, rel string, into map[string]map[string]bool) {
	ctes := map[string]bool{}
	for _, m := range cteRe.FindAllStringSubmatch(sqlText, -1) {
		ctes[strings.ToLower(m[1])] = true
	}
	for _, m := range tableRefRe.FindAllStringSubmatch(sqlText, -1) {
		name := strings.ToLower(m[1])
		if sqlKeywords[name] || ctes[name] {
			continue
		}
		if into[name] == nil {
			into[name] = map[string]bool{}
		}
		into[name][rel] = true
	}
}

func findServerRoot(t *testing.T) string {
	t.Helper()
	// Tests run with the package directory as the working directory, so walk up
	// to the directory that holds go.mod.
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate the module root (no go.mod in any parent)")
		}
		dir = parent
	}
}
