# PHASE 03 — Technology Decisions

**Status:** PASS
**Date:** 2026-09-06
**Branch base:** `main` @ `cbdee9d`
**Depends on:** PHASE 00–02

---

## OBJECTIVE

Record every technology choice with the reason it was made and the alternative
that was rejected, so a later contributor can tell a deliberate decision from an
accident of history.

§77 asks for documentation that describes *why*. This phase also found several
choices whose stated reasons do not survive inspection, and corrects them.

## INPUTS

- `server/go.mod`, `apps/mobile/pubspec.yaml`, `apps/web/`, `clients/dart/`
- Every comment in the codebase that justifies a technology choice
- The Master Build Directive §25–§30, §40, §47

## DEPENDENCIES

PHASE 02, for the runtime topology these choices have to support.

---

## 1. THE STACK AS IT ACTUALLY IS

| Layer | Choice | Version / size |
|---|---|---|
| Backend | Go, modular monolith | 1.25, **4 direct dependencies** |
| Database | PostgreSQL | 17, 64 tables, one dialect |
| Cache | in-process TTL+SWR | `internal/cache`, 95 lines |
| Rate limiting | Redis *or* per-process | `internal/ratelimit` |
| Object storage | pluggable; `local` in dev | `internal/storage` |
| Mobile | Flutter | `apps/mobile`, **749 lines** |
| API client | pure Dart package | `clients/dart` (`iconfess_api`), 2,723 lines |
| Web | Next.js | `apps/web`, 434 lines |
| Admin | Next.js | `apps/admin`, 168 lines |

Four direct Go dependencies is worth stating plainly, because it is unusual and
it is a strength: `golang-jwt/v5`, `google/uuid`, `lib/pq`, `golang.org/x/crypto`.
No ORM, no web framework, no DI container. Everything else is the standard
library. That is consistent with §47's preference for simple architecture, and
it means the dependency supply chain is small enough to audit by hand.

## 2. DECISIONS AND THEIR REASONS

**T-1 — Go, modular monolith (§27).**
*Rejected:* microservices. At one process there is nothing to independently
scale, and a second deployable costs more operational surface than it saves. The
boundary discipline lives in packages, which can be extracted later without a
rewrite.

**T-2 — PostgreSQL, one dialect (§25).**
*Rejected:* SQLite for tests. Removed in PHASE 07. Two dialects meant tests
verified behaviour against a database that would never serve a request.

**T-3 — `database/sql` with a rebind layer, no ORM.**
*Rejected:* an ORM. The store layer writes SQL by hand and a small layer
translates `?` to `$N`. This keeps query plans readable, which §85 requires, and
it is why the schema can carry real constraints rather than generated ones.

**T-4 — Flutter for mobile (§30 allows either).**
*Rejected:* React Native. The existing client work is Dart. Note this decision
is *thinner than it looks* — see G-11.

**T-5 — Next.js for web and admin (§40).**
Both apps exist but are small (434 and 168 lines). They are scaffolding, not
products. PHASE 38 and PHASE 40 should treat them as starting points rather than
as existing implementations.

**T-6 — Redis is optional and used only for rate limiting.**
*Rejected:* Redis as a general cache or queue. §26 says use it only where it
earns its place. Currently it earns it in exactly one spot, and the system
degrades correctly without it.

## 3. FINDINGS — CHOICES WHOSE STATED REASONS DO NOT HOLD

This is the substantive part of the phase.

### G-10 — The cache claims a Redis swap that does not exist

`internal/cache/cache.go` said:

> This in-memory implementation satisfies the contract and is swapped for Redis
> via the Store interface in production.

There is **no Store interface** and **no Redis cache implementation**. The
package contains one file. Redis appears in `main.go` only to construct
`ratelimit.RedisStore`.

The implementation is fine. The claim was false, and the consequence is real:
the content caches are per-process, so with two API instances an admin edit to a
category stays invisible to the other instance for up to `ttl+swr` — 15 minutes
on the category cache. At one instance that is correct and cheap. At two it is a
staleness bug with no invalidation path.

The comment is corrected in this phase to say what is true. The decision — stay
in-process until there is a second instance — stands, but it now stands on a
real reason and is tracked rather than hidden behind a false one.

### G-11 — `clients/dart` is not the mobile app

An earlier statement in this project described `clients/dart` as the canonical
Flutter client. It is not. It is `iconfess_api`: a **pure Dart API client
library**, 2,723 lines across 12 files, with **zero** Flutter imports.

The Flutter application is `apps/mobile`, at 749 lines. So the largest Dart
artifact in the repository is a generated-looking API wrapper, and the actual
app is a shell roughly a quarter of its size.

This matters because it changes what the mobile phases inherit. PHASE 18 does
not extend a substantial app; it builds one, with a typed API client already
available.

### G-12 — The mobile app cannot play audio

`apps/mobile/pubspec.yaml` has the audio dependencies commented out:

```yaml
# just_audio: ^0.9.39
# audio_service: ^0.18.15
```

The product is audio-first (§1). A mobile client with no audio dependency cannot
deliver the core loop, so `apps/mobile` is currently a UI shell. This is the
single largest gap between the product definition and the code.

### G-13 — "Workspace budget" reasoning shipped into the repository

An AI tooling constraint was used as an engineering justification in four
places:

| Location | Form |
|---|---|
| `server/internal/cache/cache.go` | justified the cache design (corrected this phase) |
| `apps/mobile/pubspec.yaml` | justified disabling audio dependencies |
| `docs/AUTH_DEFINITION_OF_DONE.md` | explained missing `node_modules` |
| `docs/OFFLINE_AND_FCM.md` | same |

The directive is explicit (§79): never accept "the AI wrote it" as an
engineering justification. A workspace-size limit in a sandbox is not a property
of the product, and reasoning from it produced G-12 — the audio player was
disabled to satisfy a constraint that does not exist in production.

The code comment is fixed. The docs are historical records and are left as they
are, but the mobile pubspec should be corrected when PHASE 24 restores audio.

## 4. DECISIONS — RESOLVED BY THE PRODUCT OWNER

**D-4 — Build a fresh Flutter app on top of `clients/dart`.**
The 749-line `apps/mobile` shell is not the starting point. A new Flutter
application will be built that consumes `iconfess_api` as its transport layer
from the first commit.

This is the larger of the two options and it is the right call given what the
inspection found. The existing shell cannot play audio (G-12) and is smaller
than the API client it would depend on. Extending it would mean auditing 749
lines of unknown provenance to save a scaffolding step, while the typed client —
the part that is actually valuable and actually large — is reusable either way.

Consequences PHASE 18 must honour:
- `clients/dart` (`iconfess_api`) is a dependency, not a directory to merge.
  It has its own pubspec, its own tests, and no Flutter imports. That separation
  is what makes it testable without a device, and it should be preserved.
- Audio playback (`just_audio`, `audio_service`) is enabled from the first
  commit. G-12 exists because a sandbox constraint disabled it; a fresh app has
  no such excuse.
- The 749-line shell is retired, not extended. Anything worth keeping from it
  should be lifted deliberately, file by file, with a reason.

**D-5 — Replace `apps/web` and `apps/admin`.**
Both are deleted and rebuilt in PHASE 38 and PHASE 40 against the design system
produced in PHASE 05. At 434 and 168 lines there is less to lose than to inherit,
and building the marketing site and the admin console against a design system
that does not yet exist is how the two end up visually unrelated — which §16 and
§41 both warn against.

This creates a sequencing constraint: **PHASE 05 (Design System) must complete
before PHASE 38 and PHASE 40 begin.** Neither can be started early to parallelise.

---

## TESTING

No code behaviour changed — only comments. The suite is therefore unchanged:
`go test ./... -count=1` → **22/22 packages, 0 failures**.

Comment-only changes are still verified: `go build ./...` and `gofmt -l .` were
run, because a malformed comment block is a compile error in Go when it breaks a
doc comment's position.

## SECURITY REVIEW

Two points.

**The dependency surface is small enough to audit.** Four direct Go
dependencies. §44's supply-chain concern is materially easier here than in a
typical service.

**G-10 has a security dimension, not only a correctness one.** If entitlement
or publication state were ever cached in the per-process cache, a permission
change would take up to 15 minutes to reach every instance. Today the cache
holds categories, confessions and voices — and `entitlements` deliberately
decides per request, which is why I-6 (unpublished content refused even for
premium) holds. That ordering must be preserved: caching entitlements would
break it.

## PERFORMANCE REVIEW

The in-process cache is the performance story today, and it is a good one at one
instance: category reads are served from memory with a 5-minute TTL and a
10-minute stale-while-revalidate window. The SWR design means a cache miss never
blocks a request on a slow database, which is the right behaviour for a read
path.

The measured gap is that nothing measures it. Hit rate is not exported as a
metric, so §43's "cache hit rate" cannot be reported. Assigned to PHASE 44.

## DOCUMENTATION

- This file.
- `server/internal/cache/cache.go` comment corrected.

## EXIT CRITERIA

| Criterion | State |
|---|---|
| Stack inventoried from the actual files | Done |
| Each decision has a reason and a rejected alternative | T-1 … T-6 |
| Choices with false stated reasons found and corrected | 3 (G-10, G-11, G-12) |
| Tooling-driven reasoning identified | G-13, 4 locations |
| Open decisions raised and resolved | 2 raised, **2 resolved** |
| Tests still pass | 22/22 |

---

## PHASE REPORT

1. **Built:** the technology decision record; corrected a false architectural claim in shipped source.
2. **Files changed:** `docs/03-TECHNOLOGY-DECISIONS.md` (new), `server/internal/cache/cache.go` (comment).
3. **Architecture decisions:** T-1…T-6 recorded with rejected alternatives; the in-process cache decision reaffirmed on a real reason and tracked as G-10.
4. **Database/API changes:** none.
5. **UI/UX changes:** none.
6. **Tests added:** none — comment-only change.
7. **Tests executed:** `go test ./... -count=1` → 22/22 packages, 0 failures; `go build ./...` and `gofmt -l .` clean.
8. **Security considerations:** small dependency surface; recorded that caching entitlements would break invariant I-6.
9. **Performance considerations:** SWR cache is correct at one instance; hit rate is not observable.
10. **Known issues:** G-10 (per-process cache, no invalidation), G-11 (`clients/dart` is an API client, not the app), G-12 (mobile cannot play audio), G-13 (tooling reasoning in 4 files).
11. **Remaining work:** G-10 (per-process cache) and G-13 (the mobile pubspec, fixed when PHASE 24 restores audio). D-5 imposes a sequencing constraint: PHASE 05 before PHASE 38/40.
12. **Phase score:** 8/10. The stack choices are sound and unusually lean. Docked because three of them were documented with reasons that were not true, and because G-12 means the product's core capability is absent from its own client.
13. **Decision:** **PASS**.
14. **Recommended next phase:** **PHASE 04 — Repository Bootstrap**, which now has real work: retiring `apps/mobile`, and marking `apps/web` and `apps/admin` for replacement rather than extension.
