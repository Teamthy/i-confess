# Ledger 55 — `main` was red: thirteen unformatted files, and a gate that passed without `gofmt`

**Date:** 2026-09-29
**Branch:** `arena/01a0edf6-i-confess`
**Scope:** the CI gate, not the product. No behaviour, route, schema, contract
or response-shape change; `server/` was rewritten by `gofmt -w` and one Makefile
recipe was corrected.

---

## 1. The state on arrival

`gh run list` at the start of this ledger:

```
completed  failure  Merge pull request #80 …  CI  main  push  36596712529
```

The merge of PR #80 left `main` failing. That is the same condition
`.arena/AUDIT-2026-09-19.md` §2 called P0 and the reason for it is recorded
there verbatim: **with `main` red, every subsequent pull request reports a
failure that says nothing about its own content.** The branch has now been red
across four consecutive PR #80 runs, and the run that merged it went red too.

## 2. How the failing step was identified

`gh run view --log-failed` could not be used: the log archive is served from
`results-receiver.actions.githubusercontent.com`, which is unreachable from this
sandbox, and three attempts each ended in `EOF`. The step-level result is on the
job object and needs no archive:

```
$ gh run view 36596712529 --json jobs \
    -q '.jobs[]|{name,conclusion,steps:[.steps[]|{name,conclusion}]}'
```

`Build, vet & test (1.25.x)` was the only failed job, and inside it exactly one
step was red:

| Step | Result |
|---|---|
| Checkout / Set up Go / Verify dependencies | success |
| **Build** | **success** |
| **Vet** | **success** |
| **Test** | **success** |
| Design tokens / Dart symbols | success |
| Lint | success |
| **gofmt check** | **failure** |
| Flutter analyze & test / Dart test / web typecheck & build / container checks | success |

So: the Go code compiles, vets, passes the full race suite and passes lint on
the GitHub runner. One formatting gate is red. That is the whole finding.

## 3. Reproduced locally

The CI step is `gofmt -l .` in `server/` (`.github/workflows/ci.yml:139`). The
same command, same working directory, same Go toolchain family:

```
$ cd server && gofmt -l .
cmd/server/main.go
internal/api/admin_audio_generation.go
internal/api/admin_rbac.go
internal/api/handlers.go
internal/audio/generator.go
internal/audio/playback.go
internal/audio/processor.go
internal/audio/service.go
internal/audio/urls.go
internal/auth/rbac_test.go
internal/jobs/audio_handler.go
internal/jobs/audio_processor.go
internal/store/rbac.go
```

Thirteen files, 199 lines. `gofmt -d` over all of them yields 430 changed lines
and **no semantic edit**: sorted import blocks, struct-literal and field
alignment, trailing-whitespace removal. Two representative hunks:

```diff
 	"github.com/Teamthy/i-confess/internal/api"
-	"github.com/Teamthy/i-confess/internal/bible"
 	"github.com/Teamthy/i-confess/internal/audio"
+	"github.com/Teamthy/i-confess/internal/bible"
```
```diff
 type URLGeneratorConfig struct {
 	CDNDomain     string
 	StreamTTL     time.Duration // Default: 4 hours
 	DownloadTTL   time.Duration // Default: 24 hours
-	SigningSecret  string        // Secret key for signing URLs
+	SigningSecret string        // Secret key for signing URLs
```

### Where they came from

| File(s) | Arrived with |
|---|---|
| `internal/audio/urls.go` and the rest of `internal/audio/*`, `internal/api/admin_audio_generation.go`, `internal/jobs/audio_*` | PR #79 `feat: Complete Audio Platform Phases 1-4` (`d61f11e9`) and the PR #80 fix commits `2a93396a`, `be354785` |
| `internal/api/admin_rbac.go`, `internal/store/rbac.go`, `internal/auth/rbac_test.go` | PR #78 `feat: super admin RBAC and full admin console` (`332921d2`) |

Three large pull requests landed in one day. Each ran its own CI; each was
merged while `main` was already red, so no one was left holding a green light
for the tree. The gate was not weakened — `gofmt -l .` is still the CI step —
it was bypassed by merging past a red main, which is the failure the 2026-09-19
audit predicted and that is still unaddressed (§6).

## 4. The second defect: `make fmt-check` passed with no Go installed

`gofmt -l` is not only a CI step. `make fmt-check` is the local gate, and it is
the first prerequisite of `make verify`, the target `CONTRIBUTING.md` points
new contributors at. It read:

```make
@cd $(SERVER) && files=$$(gofmt -l .); \
  if [ -n "$$files" ]; then echo "not gofmt-formatted:"; echo "$$files"; exit 1; fi
```

If `gofmt` is not on `PATH`, the command substitution yields the empty string,
`-n ""` is false, and **the target exits 0 without looking at a single file.**
Proved, not reasoned — a deliberately unformatted file was dropped into
`internal/api/` and the target run with Go removed from `PATH`:

```
$ printf 'package api\n\nfunc  badFmt( ) {\n_ = 1\n}\n' > server/internal/api/zz_tmp_gofmt_probe.go
$ env PATH=/usr/bin:/bin make fmt-check ; echo "exit=$?"
/bin/bash: line 1: gofmt: command not found
exit=0                      # ← green, on a tree that is not formatted
$ PATH=/tmp/toolchain/go/bin:$PATH make fmt-check ; echo "exit=$?"
not gofmt-formatted:
internal/api/zz_tmp_gofmt_probe.go
exit=2                      # ← the same file, the same tree, Go on PATH
```

The probe file was removed; the tree was clean again before the fix was applied.
The target now refuses to answer when the tool is missing:

```make
fmt-check: ## Fail if anything is unformatted
	@command -v gofmt >/dev/null 2>&1 || { \
	  echo "fmt-check: gofmt is not on PATH; install the Go toolchain (go1.25+)."; exit 1; }
	@cd $(SERVER) && files=$$(gofmt -l .); \
	  if [ -n "$$files" ]; then echo "not gofmt-formatted:"; echo "$$files"; exit 1; fi
```

Re-proved after the change, all three cases:

| Tree | `gofmt` on `PATH` | Result |
|---|---|---|
| unformatted | no | `fmt-check: gofmt is not on PATH…` — exit 2 |
| clean | no | `fmt-check: gofmt is not on PATH…` — exit 2 |
| clean | yes | exit 0 |

A gate that cannot answer must not answer "pass". This is the one behavioural
change in the ledger, and it can only turn a false green into a loud failure.

## 5. Evidence for the repair

Go 1.27.1, PostgreSQL 17.10 and Redis 7.4.2, all built by
`scripts/sandbox-bootstrap.sh`; the module graph resolved through the sandbox
`-modfile` substitutions the script documents, so `go.mod`/`go.sum` are
untouched (`git status` confirms).

| Gate | Command | Result |
|---|---|---|
| Format | `make fmt-check` (Go on `PATH`) | exit 0, no files listed |
| Build | `go build ./...` | `build-ok` |
| Vet | `go vet ./...` | `vet-ok` |
| Tests | `go test -race -count=1 ./...` with `TEST_DATABASE_URL` (PostgreSQL 17.10) and `REDIS_ADDR=127.0.0.1:6379` | **35 packages `ok`, zero `FAIL`** — `internal/api` 67.7s, `internal/store` 14.6s, `internal/deletion` 5.0s, `internal/seed` 190.7s, `internal/cache` 1.4s |
| Design | `python3 design/generate.py --check` | `generated code up to date (120 tokens, 3 files)` |
| Design rules | `python3 design/test_design.py` | `PASSED: 120 tokens, all Section 12 rules hold` |
| IA | `python3 design/test_ia.py` | `PASSED: 39 screens, 8 entry points, 162 endpoints wired` |
| Dart symbols | `python3 scripts/check_dart_symbols.py` | `PASSED: 133/133` |

`internal/storage`'s `TestContentTypeMapping`, which fails on some images where
`/etc/mime.types` maps `.xyz`, passes on this one — the environmental
observation in `.arena/AUDIT-2026-09-19.md` §4 still holds.

`make lint` was **not** run: the golangci-lint release CDN is unreachable from
this sandbox, exactly as in the ledger 50 and 2026-09-19 records. It is owed to
CI, where the Lint step of the failing run was green.

## 6. Still owed, and not fixable from here

Clearing the finding does not restore the protection that was missing when
PRs #78–#80 merged. **`main` requires the `CI` check to pass before merge**, and
branch protection is a repository-administration setting. Until an owner turns
it on, the next three large pull requests in a day will do this again, and the
gate that would have caught it is only visible after the merge rather than
before it. This is the same item the 2026-09-19 audit carried forward as P1 and
it is now the third ledger to record it.

## 7. A documentation defect found on the way

`docs/PROJECT-STATUS.md` listed **G-10** under "Open gaps carried forward"
while the same file's "Not done" table recorded it as closed in ledger 50. The
list was not updated when the gap was. Corrected in this commit; the gap
ledger, not the sentence, is the authority.
