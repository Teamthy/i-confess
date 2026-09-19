# PHASE 23 — MOBILE SESSION BUILDER

## OBJECTIVE

Complete the core loop's building half (§12): from a category — or from a
single confession via "Build a session with this" — the listener chooses how
long, chooses a voice, reviews what the engine would build, and creates a real
session. The three steps the IA names `confess/duration`, `confess/voice` and
`confess/create` stop being placeholders, and the create step becomes the only
place in the app a session is born (§5.4).

This closes the two conditions PHASE 22 deferred with an owner and a phase:

- **22-C-2** — "Build a session with this" routed to a hint. Now it carries
  the confession into the builder, which pre-selects its category and says so.
- **22-C-3** — the voices screen was a placeholder. Now it lists the licensed
  catalogue with duration already behind it.

## INPUTS

- `docs/06-UX-INFORMATION-ARCHITECTURE.md` — the builder walk
  `confess → confess/duration → confess/voice → confess/create → player`,
  with endpoints per screen (`POST /sessions/preview` on duration,
  `GET /voices` on voice, `POST /sessions` and `POST /templates` on create)
  and the rule that a voice that cannot be licensed must not be selectable.
- `server/internal/engine/planner.go` — the duration ladder (10m–180m,
  custom), the five strategies with BALANCED as default, and the wording each
  strategy's behaviour is paraphrased from.
- `server/internal/api/handlers.go`, `handlers_sessions_preview.go`,
  `handlers_templates.go` — request/response shapes, the 60 s–3 h bounds, the
  402 plan cap and the 422 exact-fit refusal.
- `clients/dart` — `createSession` existed since PHASE 20's groundwork;
  preview and templates did not.
- Audit findings (§3 of the pipeline, run before writing):
  - `ListVoices` returns **every** row regardless of `status`, including
    retired voices. The IA's selectability rule is therefore the client's to
    enforce; a comment and a selector now state that.
  - The checked-in `contracts/openapi.json` had not been regenerated since the
    phase-0 route table: 70 paths against a server now serving 216. The file's
    own generator comment promises it "can never disagree with the server". It
    was regenerated from the live route table in this phase.
  - The `player` placeholder still read "PHASE 23"; the player is PHASE 24.

## DEPENDENCIES

- PHASE 22 (category selection, confession detail) — merged.
- PHASE 17 session APIs (`POST /sessions/preview`, `POST /sessions`) — live
  and contract-tested.
- No new server code, no new packages, no new permissions.

## IMPLEMENTATION

**`clients/dart`** (the contract the app ships on):

- `endpoints.dart` — `postSessionsPreview`, `postTemplates`.
- `models.dart` — `SessionPreview` (target/actual seconds, total items, the
  three-item peek, voice downgrade, strategy, the server's `display` line,
  kept verbatim because the server is the single author of what a plan is
  called); `SessionTemplate` (accepts both the wrapped create response and the
  bare row); `Voice` gains `description`, `gender`, `status` and
  `isSelectable`.
- `repository.dart` — `previewSession`, which returns `WriteResult` rather
  than `Loadable` because the interesting failures are rejections (402 plan
  cap, 422 no exact fit) the UI must word differently from an outage; and
  `createTemplate`. Neither is cached, for the same reason `createSession`
  is not: the answer depends on entitlement at this instant.

**`apps/mobile`** — one new pure-Dart file and three screens, wired through
the existing `confess` branch so the tab bar stays:

- `session_builder.dart` — the ladder, the bounds (60 s–3 h), the strategy
  vocabulary and its wording, and `formatSeconds`. No Flutter import: it is a
  mirror of the engine, and mirrors are only honest if they are guarded, so
  `builder_test.dart` reads `planner.go` and `handlers.go` and fails if the
  two ever disagree (the pattern PHASE 19 built for the password policy).
- `duration_screen.dart` — the ladder as chips, a custom-minutes field that
  enforces the server's own bounds client-side, the strategy selector
  defaulting to BALANCED with each option paraphrasing the engine's doc
  comment, and the live `POST /sessions/preview` card: the server's
  `display` line, the asked-versus-filled gap ("complete confessions are
  never cut short"), any voice-downgrade notice, and the first three items
  with an honest "and N more".
- `voice_screen.dart` — "No preference" as a first-class choice, the active
  catalogue filtered by one `selectableVoicesProvider` selector so the picker
  and the summary cannot disagree, premium marked, retired voices absent.
- `review_screen.dart` — every choice restated; Create calls `POST /sessions`;
  a 402 words itself and offers "Choose a shorter length"; the created
  session is shown with its composition, locked items marked as locked, and
  — deliberately — **no play button**: playback is PHASE 24, and a button
  that cannot play would be a lie with an icon. After success the shape is
  offered as a template (§5.4 puts that offer here, not in the player), with
  the share link surfaced once saved. `Done` resets the builder and returns
  home, so back never walks the listener through consumed steps.
- `confess_screen.dart` — reads `?confession=<id>` from the route (a deep
  link carries the same parameter), folds the confession's category into the
  selection once on arrival via `ref.listen`, and shows a dismissible banner
  naming where the builder came from. Continue now routes to
  `/confess/duration`.
- Router — `duration`, `voice`, `create` live under the confess branch; the
  `/voices` placeholder is gone; `requiresAuth` covers the steps by the
  existing `/confess` prefix; the player placeholder now says PHASE 24.
- Analytics — no new vocabulary. The two events this phase emits
  (`session_created`, `template_created`) already existed; properties are
  structural (`item_count`, `duration_seconds`, `has_voice`), never content.

## TESTING

**What was executed in this sandbox** (Go 1.27.1 via PyPI `go-bin`;
PostgreSQL 17.10 via npm `@embedded-postgres/linux-x64`; **no Dart/Flutter SDK
is obtainable here** — pub.dev, the Google storage hosts and GitHub release
assets are all unreachable, so the Dart suites were written to the repo's
standards but could not be executed in this session):

| Claim | Evidence |
|---|---|
| Server builds | `go build ./...` — 0 errors |
| Server vets | `go vet ./...` — 0 findings |
| 27 of 28 packages pass against PostgreSQL 17.10 | `go test -modfile=go.local.mod ./... -count=1` |
| The builder's endpoints answer with the exact shapes the Dart models decode | live server + seeded database: `GET /v1/voices`, `POST /v1/sessions/preview` (200), `POST /v1/sessions` (201), `POST /v1/templates` (201, `{template, share_url, deeplink}`) all key-checked against `Voice.fromJson`, `SessionPreview.fromJson`, `ListeningSession.fromJson`, `SessionTemplate.fromJson` |
| 402 plan cap, 400 short custom length | live: 3 h preview → 402; 30 s preview → 400 |
| `internal/storage` content-type failure is environmental, pre-existing | this image's `/etc/mime.types` maps `.xyz` → `chemical/x-xyz`; the test expects the no-entry fallback. Fails identically on `main` before this phase's changes; passes on CI hosts without that mapping |
| The checked-in OpenAPI spec matches the server again | `go run ./cmd/genspec` — 216 paths, 280 unique operation ids, `/sessions/preview` and `/templates` present |
| Drift-guard logic proven against the Go source | the guard's own extraction rules (preset regex incl. `10 * 60` arithmetic, strategy constants, handler bounds) replayed against `planner.go`/`handlers.go` — all assertions pass |
| Dart sources are structurally sound | every new/modified file bracket-balanced and hand-audited; `flutter analyze` still owed by CI |

**Written this phase, to run in CI / the next Flutter-capable session:**

- `clients/dart/test/repository_test.dart` — preview decodes and sends the
  create-shaped body; 402 is a typed `WriteFailure` with
  `requiresSubscription`; template create sends shape fields and decodes the
  wrapped response; a retired voice is `isSelectable == false`; a missing
  template name is a 400 the UI can show.
- `apps/mobile/test/builder_test.dart` — the full walk (category → length →
  voice → create) asserting the exact `POST /sessions` body, the preview
  display line, the BALANCED default, the custom-length refusal at 500
  minutes, the retired voice's absence, the created surface's no-player
  wording, `session_created` analytics; the 402 create path; re-preview on
  duration change; the confession handoff pre-selecting `cat-heal`; and the
  drift guard reading the Go source.

## SECURITY REVIEW

- **Authorization.** `POST /sessions/preview`, `POST /sessions` and
  `POST /templates` are registered `authed`; PHASE 22's route test already
  asserts every authenticated route rejects an anonymous caller. The builder
  sends nothing identity-shaped: the user id comes from the token server-side.
- **No content in analytics.** Event properties are counts and durations.
  Category names, session titles and confession text never leave the device —
  the blocklist is defence in depth, but the first gate is not sending them.
- **The review screen shows signed URLs' presence, not their contents.** The
  preview's `audio_url` values are rendered nowhere; only titles, lengths and
  counts are shown. Signed links remain short-lived and server-minted.
- **The paywall is still not the only path.** A 402 offers "Choose a shorter
  length" that returns to the duration step: a free listener can always reach
  a created session without passing through any purchase surface (§6's
  reachability rule, upheld in code rather than asserted in prose).
- **The template offer cannot hijack the session.** A failed template save is
  worded as such ("the session itself is safe"); the session and the template
  are separate writes, and only the template's failure blocks nothing.
- **Deep-link handoff.** `/confess?confession=<id>` reuses the detail fetch;
  the id is passed to the API client, which URL-encodes it. No HTML rendering
  is introduced anywhere in the flow.

## DOCUMENTATION

- This file.
- `docs/PROJECT-STATUS.md` — phase 23 entry, PHASE 22's C-2/C-3 closed,
  verified line updated.
- `contracts/openapi.json` — regenerated from the live route table (audit
  finding; see INPUTS).
- Doc comments on the new client types state the server contract they mirror
  and why (`isSelectable`, `previewSession`, `SessionPreview.display`).

## EXIT CRITERIA

- Duration step offers the engine's ladder plus custom, refuses out-of-bounds
  values before the server does, defaults strategy to BALANCED, and previews
  live. ✔ (code + live-shape evidence; widget run owed to CI)
- Voice step offers no-preference plus the licensed catalogue, with retired
  voices filtered. ✔
- Review creates a real session with the exact body the handlers define, and
  shows the created composition honestly (locked items, no play button). ✔
- "Build a session with this" lands in the builder with the category
  pre-selected and announced. ✔
- Template save-offer lives on the create step per §5.4. ✔
- Route table, navigation test list and the IA's walk agree. ✔
- Dart test suites written; execution owed to a Flutter-capable environment —
  recorded as the first condition below. ✔ (with condition)

## VERDICT — PASS WITH CONDITIONS

- **C-1** The Dart/Flutter toolchains were unreachable from this sandbox
  (pub.dev and SDK hosts blocked). Every Dart assertion this phase adds is
  written and its logic proven where possible without the SDK, but
  `dart test`, `flutter test` and `flutter analyze` have not run on this
  code. Owner: CI on this pull request, or the next session with SDK access.
  Until then the phase's client claims rest on the live-shape verification
  above, which is evidence, not a substitute.
- **C-2** Created sessions cannot play yet — the player is PHASE 24. The
  review surface says so in words rather than shipping a dead button.
- **C-3** The free plan's length cap is server-owned and surfaced only as a
  402 message. A first-class paywall surface is PHASE 30; until then the
  builder's wording stays plan-agnostic.
- **C-4 (carried from 22)** Favourite status is derived from the list; the
  dedicated `GET /me/favorites/{id}` endpoint remains a server-side idea.
