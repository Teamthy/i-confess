// Package arch exists only to hold tests that constrain the shape of the
// codebase rather than its behaviour.
//
// Section 2 of the build directive specifies a modular monolith layered
// API -> application services -> domain -> repositories -> infrastructure. A
// layering that is only described decays: the first deadline puts a query in a
// handler, the second puts an HTTP status in the domain, and within a year the
// layers are a comment. These tests make four specific properties mechanical.
package arch

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const modulePath = "github.com/Teamthy/i-confess/internal/"

// packages returns every internal package name and the internal packages it
// imports, reading non-test sources only.
func packages(t *testing.T) map[string][]string {
	t.Helper()

	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve internal/: %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read %s: %v", root, err)
	}

	out := map[string][]string{}
	fset := token.NewFileSet()

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		dir := filepath.Join(root, name)

		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil || len(files) == 0 {
			continue
		}

		// Record the package even if it turns out to import nothing internal.
		// A missing key and a package with zero dependencies are different
		// facts, and conflating them made internal/models look absent.
		if _, ok := out[name]; !ok {
			out[name] = []string{}
		}

		seen := map[string]bool{}
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			parsed, err := parser.ParseFile(fset, f, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatalf("parse %s: %v", f, err)
			}
			for _, imp := range parsed.Imports {
				path := strings.Trim(imp.Path.Value, `"`)
				if !strings.HasPrefix(path, modulePath) {
					continue
				}
				dep := strings.TrimPrefix(path, modulePath)
				if dep != name && !seen[dep] {
					seen[dep] = true
					out[name] = append(out[name], dep)
				}
			}
		}
		sort.Strings(out[name])
	}

	if len(out) < 10 {
		t.Fatalf("found only %d packages - the scan is broken and every assertion below would pass vacuously", len(out))
	}
	return out
}

// TestTransportIsALeaf asserts nothing depends on the HTTP layer.
//
// If a domain package imports api, the domain has learned about request
// handling, and the two can no longer be tested or replaced separately.
func TestTransportIsALeaf(t *testing.T) {
	for pkg, deps := range packages(t) {
		for _, d := range deps {
			if d == "api" {
				t.Errorf("internal/%s imports internal/api - transport must be a leaf", pkg)
			}
		}
	}
}

// TestSharedKernelHasNoInternalDependencies asserts models stays at the bottom.
//
// Every layer needs the domain types. The moment models imports something
// internal it drags that dependency into every package that imports it, which
// is how a leaf becomes a knot.
func TestSharedKernelHasNoInternalDependencies(t *testing.T) {
	deps, ok := packages(t)["models"]
	if !ok {
		t.Fatal("internal/models not found")
	}
	if len(deps) > 0 {
		t.Errorf("internal/models imports %v - the shared kernel must depend on nothing internal", deps)
	}
}

// TestNoImportCycles guards against the failure mode that a layering diagram
// cannot show. Go rejects direct cycles at compile time, but an indirect cycle
// through a new package compiles fine and makes the two packages impossible to
// reuse apart.
func TestNoImportCycles(t *testing.T) {
	deps := packages(t)

	const (
		unvisited = 0
		visiting  = 1
		done      = 2
	)
	state := map[string]int{}
	var path []string
	var found []string

	var visit func(string)
	visit = func(n string) {
		state[n] = visiting
		path = append(path, n)
		for _, d := range deps[n] {
			switch state[d] {
			case visiting:
				cycle := append(append([]string{}, path...), d)
				found = append(found, strings.Join(cycle, " -> "))
			case unvisited:
				visit(d)
			}
		}
		path = path[:len(path)-1]
		state[n] = done
	}

	names := make([]string, 0, len(deps))
	for n := range deps {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if state[n] == unvisited {
			visit(n)
		}
	}

	if len(found) > 0 {
		t.Errorf("import cycles: %v", found)
	}
}

// TestDomainDoesNotKnowAboutHTTP is the property that keeps the domain testable
// without a server.
//
// Outbound clients legitimately import net/http - email, push, storage and the
// OAuth and voice providers all speak HTTP. What must not is the decision
// logic: session state, entitlements, rights and the engine. A rule expressed
// as an HTTP status code cannot be reused by the scheduler, which has no
// ResponseWriter and should not need one.
func TestDomainDoesNotKnowAboutHTTP(t *testing.T) {
	domain := []string{"sessions", "engine", "entitlements", "rights", "community", "billing", "models"}

	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve internal/: %v", err)
	}

	checked := 0
	for _, pkg := range domain {
		files, err := filepath.Glob(filepath.Join(root, pkg, "*.go"))
		if err != nil {
			t.Fatalf("glob %s: %v", pkg, err)
		}
		if len(files) == 0 {
			t.Errorf("internal/%s has no Go files - is this list stale?", pkg)
			continue
		}

		fset := token.NewFileSet()
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			parsed, err := parser.ParseFile(fset, f, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatalf("parse %s: %v", f, err)
			}
			for _, imp := range parsed.Imports {
				if strings.Trim(imp.Path.Value, `"`) == "net/http" {
					t.Errorf("internal/%s imports net/http - domain logic must not depend on HTTP (%s)",
						pkg, filepath.Base(f))
				}
			}
			checked++
		}
	}

	if checked == 0 {
		t.Fatal("checked no files - the package list does not match the repository")
	}
}
