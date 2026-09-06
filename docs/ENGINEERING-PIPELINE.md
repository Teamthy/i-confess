# THE ENGINEERING PIPELINE

How this product is built, end to end: from the product idea to a merged pull
request, and everything in between.

This is a working document rather than a post-mortem. It exists because the
process has failure modes that are not obvious from the outside — a sandbox that
wipes its toolchain between turns, a generator that can silently replace nothing,
a green build that proves an edit never applied — and because a process that only
lives in someone's head stops working the moment they are not the one running it.

---

## 1. Conception

Everything starts in `docs/00-PRODUCT-SOURCE-OF-TRUTH.md`. The product is a
production-grade Christian confession, Scripture-meditation and prayer **audio
session** platform — comparable in ambition to Spotify, Calm or Headspace, and
explicitly *not* a church CMS, a Bible-quote generator, or an AI chatbot. The
central loop is: choose what to speak, choose a duration, choose a voice, and get
a structured audio session, schedulable.

Three consequences follow from that framing and shape every later phase:

- **The session engine is not the audio player.** Eleven states, centralised
  transitions, snapshots that do not mutate when content changes.
- **Audio never travels through the API.** Object storage plus a CDN, with
  signed, private assets.
- **No unauthorized voice generation.** A hard rights gate before any audio is
  produced: generate → check rights → authorised, or stop.

Nothing is built until the thing it belongs to is written down. A phase that
discovers a contradiction between the brief and what is already in the repository
stops and settles it before writing code — see §4.

---

## 2. The phase journey

The work is split into numbered phases. The numbering is authoritative and comes
from the product directive, not from inference:

```
00 Product source of truth      13 Audio infrastructure
01 Domain model                 14 Voice platform
02 System architecture          15 Session engine
03 Technology decisions         16 Queue / worker
04 Repository bootstrap         17 Session APIs
05 Design system                18 Mobile foundation
06 UX / information architecture 19 Mobile authentication & onboarding
07 Database foundation          20 Mobile home
08 Go backend                   21 Mobile explore
09 Auth                         22 Confession experience
10 Users / profiles             23 Session builder
11 Content engine               24 Audio player
12 Content governance           25+ library, admin…
18 Mobile foundation
19 Mobile authentication
```

Statuses live in `docs/PROJECT-STATUS.md`; the map above is only the ordering.
Phases 20–24 are the mobile content block. 20–21 ship against the read-only
catalogue and session APIs that already exist; 22–24 add playback and are the
first phases that need real audio, so they are deliberately sequenced after the
browsing surfaces have something to browse.

Each phase produces a document in `docs/` carrying its number, and every phase
document has the same eight sections:

**OBJECTIVE · INPUTS · DEPENDENCIES · IMPLEMENTATION · TESTING · SECURITY REVIEW
· DOCUMENTATION · EXIT CRITERIA**

and ends in a verdict: **PASS**, **PASS WITH CONDITIONS**, or **FAIL**. A
condition is a real, named gap with an owner and a phase — not a hedge.

`docs/PROJECT-STATUS.md` is the running index: what each phase delivered, its
verdict, and the open conditions carried forward.

### The gate

Phases are gated. The standing rule was "stop and report after every single
phase", which was chosen deliberately as the slowest safe option.

**It is now grouped.** Interrelated phases run back to back without stopping; one
report with evidence is produced per *group*, and each phase inside the group
still gets its own commit, its own document and its own delivery script. The
first group is the mobile block, PHASE 19–21.

What grouping does not change: a phase that FAILS stops the group immediately. A
condition that would invalidate the next phase stops the group and asks.

---

## 3. Understand before generating

Section 79 of the directive is blunt: never accept "the AI wrote it". Before a
phase writes anything, it audits what already exists.

The audit is not a formality. Every phase so far has found something that
contradicted the documentation:

| Phase | What the audit found |
|---|---|
| 09 | `Validate()` began with `if !c.IsProduction() { return nil }` — configuration validation was skipped entirely outside production |
| 11 | The catalogue launched empty; 78 confessions and 39 categories existed in no table |
| 13 | `main.go` hardcoded `Provider: "local"` with cloud stubs; duration was never measured |
| 14 | The voice-rights gate was unsatisfiable — 12 columns for a ~20-field model — yet `PUT` returned 200 |
| 15 | `AssetsFor` served the *oldest* render; a status-vocabulary mismatch made `/skip` and `/progress` fail silently |
| 16 | The queue was hollow: nothing enqueued, `JobStore` had zero callers, all four handlers logged and returned nil |
| 17 | `DELETE /sessions/{id}` did not exist; `/pause` and `/resume` had no test coverage |
| 18 | `design/generated/tokens.dart` existed but no package could import it |
| 19 | Sign-in never stored its token; the router was rebuilt on every auth change |

A phase that skips the audit builds a second implementation of something that
already exists, or — worse — "fixes" a defect that the documentation described but
the code never had.

The rule that follows: **reuse before you declare**. Read the existing code, then
decide.

---

## 4. Settling contradictions

When the brief and the repository disagree, the phase stops and asks. Three
examples from the mobile work, all resolved before code was written:

- The pasted wireframe said React Native; the recorded decision said Flutter.
  **Flutter** — `clients/dart` and `apps/mobile` already exist.
- The wireframe's primary colour `#A8FF3E` is hue 87°; `design/test_design.py`
  asserts brand hue 100–170° and runs inside `make verify`. **Keep `#2A9D76`** —
  the new colour would have failed the existing guard on day one.
- No Dart toolchain in the sandbox. **Install it**, outside the workspace, so the
  phase could be verified rather than asserted.

Guessing is cheaper for one turn and far more expensive across five.

---

## 5. Building

Conventions that hold across the repository:

- **Modular Go monolith**: API → application services → domain → repositories →
  infrastructure. `server/internal/{api,auth,db,store,workers,…}`.
- **PostgreSQL is the only database**, including in tests. One dialect
  everywhere; there is deliberately no fallback that skips tests when the
  database is missing.
- **Feature-first Flutter** (`apps/mobile/lib/src/features/…`) over a core layer
  (`…/core/{theme,routing,di,error,analytics,persistence,widgets}`).
- **Design tokens are generated.** `design/tokens.json` is the source;
  `design/generate.py` emits CSS, TypeScript and Dart, and mirrors the Dart into
  the app. `make design-check` fails if the mirror drifts. A hand-written hex
  value in a theme is how a design system stops being one.
- **Nothing is hardcoded in the client that the server owns.** 39 categories,
  durations, voices — all backend-configurable.
- **Comments explain why, not what**, and say which section of the directive they
  come from. A comment that turns out to be false is treated as a defect (PHASE
  19 found one).
- **A stub that compiles is worse than no code.** Screens that do not exist yet
  are labelled with the phase that will build them.

---

## 6. Verification

Nothing is reported as done without evidence. The order matters, because each
step catches something the previous one cannot:

1. **Analyse** — `flutter analyze`, `go vet`, `golangci-lint`, `gofmt -l`.
2. **Test** — the project's own runner. `make verify` runs everything CI runs:
   `fmt-check`, `design-check`, `build`, `vet`, `lint`, `test`, `mobile-check`.
3. **Defect injection.** For every guarantee a phase claims, break it on purpose
   and confirm a test fails. Back up, inject, run, restore, re-verify. A test
   suite that has never been seen to fail proves nothing.
4. **Count before claiming.** `grep -c '^func Test'` rather than "about twenty
   tests". This document's numbers were counted, not remembered.

Two traps worth stating because both have been hit:

- **A failed edit script leaves the file unchanged and the build still passes.**
  A green build is not evidence an edit applied. Put `assert old in source`
  before writing, and grep for the new symbol afterwards.
- **An injection that breaks the build proves nothing.** If the package does not
  compile, the test failed for the wrong reason.

---

## 7. Delivery

The sandbox has no push credentials, so work travels as a script the maintainer
runs.

```
local branch ──► commit ──► tools/mkapply.py ──► APPLY_PHASEnn_*.ps1
                                                        │
                       maintainer runs it in their clone │
                                                        ▼
                                        git bundle applied on origin/main
                                                        ▼
                                              branch pushed ──► PR ──► merge
```

1. **Commit** on `feat/phaseNN-<name>`, identity set per session.
2. **Generate** with
   `python3 tools/mkapply.py <branch> <out.ps1> [--pr N] [--desc TEXT]`.
   The script embeds the commit(s) as a base64 **git bundle**, so messages and
   history survive instead of being flattened into a file copy. It applies onto
   `origin/main`, so it works regardless of what the target clone has checked
   out. It never force-pushes.
3. **Run** it from the repository root:
   `powershell -NoProfile -ExecutionPolicy Bypass -File .\APPLY_PHASEnn_*.ps1`
   (`-UpdatePR` also fast-forwards `main`; `-SkipVerify` skips the pre-push
   build and tests).
4. **Open the pull request**, review, merge.
5. **Rebase the next phase on the merged commit.** Before generating a script,
   the local `origin/main` ref is set to the last *merged* upstream commit — the
   GitHub API is the authority, because `git fetch` cannot authenticate from the
   sandbox. A bundle built against the wrong base carries commits that are
   already upstream.

`APPLY_*.ps1` files are gitignored: they contain embedded copies of the tree, and
committing one means every clone carries a stale duplicate.

---

## 8. The environment

The sandbox is ephemeral and the workspace is not, which inverts the usual
assumption about what needs looking after:

- **Everything under `/home/user` persists** between turns, except a list of
  generated directories (`node_modules`, `build`, `.dart_tool`, `.cache`, …).
- **The toolchain does not.** Go, PostgreSQL, golangci-lint, pwsh and the 2.5 GB
  Flutter SDK live in `/home/user/.cache` and are routinely gone on the next
  turn. `ls` before reinstalling; installing a toolchain that is already there
  costs ten minutes for nothing.
- **Environment is a file, not an export line.** `goenv.sh` and `flutterenv.sh`
  are `source`d at the start of every session. A long inline `export A=… B=…` is
  silently dropped.
- **Pinned download URLs rot.** Query the release API rather than trusting a
  remembered version string.
- **There is no root.** Packages are fetched with `apt-get download` and unpacked
  with `dpkg-deb -x` into the cache.

`make mobile-check` prints `SKIPPED` loudly when Flutter is absent rather than
passing silently. A check that quietly does nothing is worse than no check.

A known-good restore order, for the record (verify versions against the release
APIs first — pins rot): Go from `go.dev/dl` into `.cache/go` with
`GOPATH=.cache/gopath`; golangci-lint into `.cache/gopath/bin`; the Flutter
stable tarball from `storage.googleapis.com/flutter_infra_release` into
`.cache/flutter` with `PUB_CACHE=.cache/pub-cache`; PostgreSQL by
`apt-get download postgresql-17 …` + `dpkg-deb -x` into `.cache/pgroot`, then
`initdb`/`pg_ctl` on `127.0.0.1:5432` with a trust user; pwsh from the
PowerShell release assets (`linux-x64`, not `linux-amd64`). Every one of these
has 404'd or 403'd in a remembered form at least once; the GitHub/Go release
JSON endpoints are the reliable way to find a live asset.

---

## 9. Failure modes worth remembering

These are the ones that have actually happened, in the words they cost:

- A generated-file mirror whose existence test keyed on the *destination
  directory* silently skipped, and a missing mirror passed the staleness check.
  Key on the app root.
- `flutter create` regenerates `test/widget_test.dart` and resets the pubspec
  description even with `--platforms=X .` on an existing project.
- `flutter build web` exceeds a 25-minute budget. Import `main.dart` from a test
  instead of compiling a release bundle as a check.
- An `abstract interface class` member with a body is **not** inherited by
  implementers. Use an extension.
- `assert(false)` throws in tests. Telemetry must report through a callback.
- Re-pumping a `ProviderScope` with a different number of overrides trips a debug
  assertion.
- `ORDER BY created_at` alone is not stable at second precision; keyset
  pagination needs `(created_at, id)`.
- `ON CONFLICT (id)` with a freshly generated UUID never fires.
- Adding a column to a unique key makes it looser, and `NULL` in a unique key
  allows duplicates.
- A policy enforced at the handler layer is bypassed by every direct caller.
- `mErr == nil && x.Enabled` fails **open**: a database error skips the second
  factor. `ErrNotFound` means "not enrolled"; every other error must refuse.
- An ownership check on the parent does not cover the ids in the body.
- A queue nobody enqueues to is not a queue, and a handler wired with no producer
  recreates the defect one layer up.

And the meta-rule that produced most of the fixes above:

> **My own documented claims can be false.** A number written from memory, a
> field assumed to exist, a comment describing behaviour the code never had —
> each has happened here. Count it, query it, or run it before writing it down.

---

## 10. What "done" means

A phase is done when:

1. Its document exists with all eight sections and a verdict.
2. `make verify` passes, or the parts that could not run are named with the
   reason.
3. Every guarantee it claims has been broken on purpose and seen to fail a test.
4. Its counts are counted.
5. Its conditions are named, owned and assigned to a phase.
6. Its commit is on a branch, its script parses, and the maintainer can apply it.

Not one of those is "the build is green".


---

## 11. Starting from zero (a fresh session reads this)

A new session — this sandbox, a CI runner, or a laptop — should be able to pick
up the work by reading, in order: this document, `docs/PROJECT-STATUS.md`, then
the numbered phase documents. Nothing else is required to understand *what* to
build. *Where* the work stands is answered by git, not by memory:

1. **Clone and find the truth.** `git log --oneline origin/main` is the merged
   history. The GitHub API (`/repos/Teamthy/i-confess/commits?sha=main`) is the
   authority when the clone's remote refs are stale, because a sandbox cannot
   `git fetch` without credentials.
2. **Pick up at the first phase whose document has no PASS verdict**, or at the
   phase `PROJECT-STATUS.md` names as in progress. Do not start a later phase
   while an earlier one is FAIL.
3. **Restore a toolchain before claiming anything.** See §8 and the recipe in
   §12. `make verify` is the definition of "the project builds"; if it cannot
   run, name exactly which step could not run and why.
4. **Audit before generating** (§3). The phase documents record what each audit
   found; a fresh session repeats the audit rather than trusting the record.

The workspace layout is fixed and small: top level is only `apps/ clients/
contracts/ design/ docs/ server/ tools/` plus the `Makefile`. `apps/mobile` is
the Flutter app, `clients/dart` is the pure-Dart API client it consumes,
`server/` the Go monolith, `contracts/openapi.json` the API contract, and
`design/` the token pipeline. Anything else at the top level is litter.

---

## 12. The maintainer's machine (local, not sandbox)

The maintainer works on a normal machine with push rights; the sandbox does not.
The two meet only through the apply scripts (§7). A few facts about the local
side, because they have caused confusion:

- **Branches:** merged work lands on `main`. `mvp-phase` is an old integration
  branch; a local clone sitting on it and "behind origin" is simply stale —
  `git checkout main && git pull` (or fast-forward) is the fix. Feature work is
  always on `feat/phaseNN-<name>`.
- **The pipeline document lives at `docs/ENGINEERING-PIPELINE.md`.** A copy at
  the repository *root* is a stray produced by an earlier session and should be
  deleted; git shows it as untracked `ENGINEERING-PIPELINE.md`. The tracked,
  canonical file is the one in `docs/`.
- **Applying a script:** from the repository root run
  `powershell -NoProfile -ExecutionPolicy Bypass -File .\APPLY_PHASEnn_*.ps1`.
  It creates the feature branch, applies the bundle, and (with `-UpdatePR`)
  fast-forwards `main` and pushes. Uncommitted local edits (issue templates and
  the like) are untouched by the script but will show in `git status`; commit or
  stash them before switching branches.
- **Helper env files** (`goenv.sh`, `flutterenv.sh`, `tools/mkapply.py`):
  `mkapply.py` is in the repository and travels with it. The two `*env.sh` files
  are sandbox conveniences that set `PATH`/`GOPATH`/`PUB_CACHE`; on a normal
  machine a standard Go/Flutter install makes them unnecessary.
