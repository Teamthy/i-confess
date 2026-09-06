# PHASE 18 — MOBILE FOUNDATION

## OBJECTIVE

Build the Flutter foundation: architecture, routing, dependency injection, API
client, state management, theme, design tokens, safe areas, error handling,
persistence, analytics.

## DECISIONS CONFIRMED BEFORE WRITING CODE

Three things in the brief contradicted recorded decisions or the environment, so
they were settled first rather than guessed:

| Question | Decision |
|---|---|
| Flutter (recorded decision D-4) vs React Native/Expo (the pasted wireframe) | **Flutter.** The wireframe's RN references are treated as illustrative. |
| `#A8FF3E` primary (wireframe) vs `#2A9D76` (PHASE 05) | **Keep `#2A9D76`.** `#A8FF3E` is hue 87°; `design/test_design.py` asserts brand hue 100–170° and runs in `make verify`, so it would have failed the existing guard on day one. |
| No Dart/Flutter toolchain in the sandbox | **Install Flutter 3.47.2** into `~/.cache`, outside the workspace snapshot, so the phase could be verified rather than asserted. |

## INPUTS

`docs/00`–`17`, the mobile UI/UX wireframe, `design/tokens.json` (120 tokens,
PHASE 05), and `clients/dart` (`iconfess_api`, 2021 lines).

## DEPENDENCIES

PHASE 05 for the token system. `apps/mobile/RETIREMENT.md` named this phase as
the replacement for the retired 749-line shell, so it could not have been
sequenced earlier.

---

## Findings before building

**The API client was already complete and unused.** `clients/dart` covers auth,
profile, preferences, interests, categories, voices, sessions, collections,
schedules and downloads, and already carries the hard parts: a sealed
`Loadable<T>` that forces every screen to handle initial/loading/loaded/failed, a
`TokenStore` interface with a fail-closed keystore wrapper, and a
`WriteResult<T>` that distinguishes an offline failure from a rejection. PHASE 18
wires it; it does not replace it.

**The retired shell's real asset was its event vocabulary.** `analytics.dart` had
the right 17 event names (§§67–68) and the wrong implementation — a singleton
that `debugPrint`ed and carried a `TODO` for production. The names were lifted
into a proper foundation; the implementation was not.

**`design/generated/tokens.dart` existed but nothing could import it.** A Dart
package cannot import across the repository from `design/generated/`, so the
tokens were generated and unreachable.

---

## IMPLEMENTATION

### 1. Tokens reach the app without a second source of truth

`design/generate.py` now mirrors `tokens.dart` into
`apps/mobile/lib/src/core/theme/tokens.dart`, and `--check` verifies the mirror
as well as the original.

The mirror's existence test keys on the **app root**, not the destination
directory. Keying on the destination makes `--check` skip a mirror that is merely
missing, so an app whose tokens were never generated would pass the staleness
check and then fail to compile. Verified: deleting the mirror and running
`design-check` exits 1.

### 2. Theme — two visual modes, not light and dark

`AppMode.discovery` and `AppMode.immersive` are a product distinction, not a
system preference. Browsing wants a warm editorial surface; an active confession
wants almost no interface. Binding them to the system theme would give a listener
who keeps their phone dark an immersive player by day and a dim catalogue at
night. The player opts into immersive explicitly.

Every value comes from `IConfess`. Nothing in the theme chooses a colour, a
radius or a spacing step directly — a hand-written hex in a theme is how a design
system stops being one.

Body copy uses the serif (`Source Serif 4`) and interface text the sans
(`Inter`), per §30: the confessions are read as much as heard.

### 3. Error handling

`ErrorMapper` turns an `ApiException` into a title, an explanation, and up to two
recovery actions. Section 38 forbids "Something went wrong", and the mapping
enforces that: a network failure offers downloads, an expired session offers
sign-in and says the history is safe, a rate limit names the wait, a 400 does
**not** offer a retry that will fail identically, and unpublished audio does not
read as a fault.

Auth failures match the server's stable codes rather than the status alone: a 401
can mean an expired token, a revoked session or a reused refresh token, and those
are three different recoveries.

### 4. Analytics

An interface, a recording debug implementation, and an inert release one. The
release default is `NoopAnalytics` rather than the debug implementation: shipping
a `debugPrint` per event in release is both a cost and a leak surface.

Two guards, both reported through a callback rather than an `assert`:

- **Unknown events are refused.** A typo'd name fragments a metric across two
  names and nobody notices until a dashboard is wrong.
- **Blocked properties are stripped.** §§67–68: no Scripture text, no email, no
  confession body, no free-form query. The list is matched case-insensitively.

An assertion was the obvious choice and was rejected: assertions throw, and
analytics must never be able to break the flow that triggered it. A listener who
completes a session should not get an exception because somebody passed a `query`
property.

### 5. Routing and DI

`go_router` with one redirect rule. Access is decided in the router, not per
screen, so a deep link cannot reach a screen its own guard would have refused.

While auth is unresolved the redirect returns null. Deciding during that window
would flash sign-in at every signed-in listener on every cold start. `main`
resolves the keystore before the first frame, so the splash covers the lookup
rather than the sign-in screen covering the splash.

Riverpod 3 provides DI. Screens never construct a client, cache or store, which
is what makes them testable: one override replaces the dependency chain.

### 6. Safe areas and the shell

`AppScaffold` owns safe-area handling, horizontal margins and scroll behaviour so
they are decided once. It applies the top inset only when there is no app bar —
otherwise the content is pushed down twice.

Five destinations with the confession action in the centre, emphasised as a
filled circle at the same 64pt row height rather than a floating button: a FAB
covers content and reads as a platform default.

---

## TESTING

**44 tests**, all passing. `flutter analyze`: no issues.

| File | Tests | Covers |
|---|---|---|
| `theme_and_errors_test.dart` | 17 | token-driven theme, brand hue in both modes, immersive ≠ dark discovery, 44pt minimums, 4-point spacing, radius set, serif/sans split, and every error mapping |
| `foundation_test.dart` | 16 | analytics vocabulary and blocklist, JSON persistence, corrupt-entry recovery, keystore caching and fail-closed behaviour |
| `navigation_test.dart` | 10 | route table, redirect rules, five destinations, semantic labels, 44pt targets, branch switching |
| `bootstrap_test.dart` | 1 | compiles `main.dart`, which no other test reaches |

`bootstrap_test.dart` exists because no other test imports the entry point, so a
type error in the bootstrap would pass `flutter test` and only surface when the
app was built.

Two defects were found by the tests rather than by reading:

1. **The splash was a dead end.** A signed-out listener landed on `/splash` and
   the redirect never moved them, because splash was only left when signed in.
   Fixed to hand off to `/welcome`.
2. **Analytics threw on misuse.** The first implementation used `assert(false)`,
   which throws in debug and in tests — analytics breaking the flow it was
   recording. Replaced with a reporting callback.

`make verify` now runs `mobile-check` alongside the Go gate, so the app is
analysed and tested in CI. It says `SKIPPED` loudly if Flutter is absent rather
than passing silently.

---

## SECURITY REVIEW

- **The token never reaches plain storage.** `SecureTokenStore` wraps a
  `SecureStorage` the app supplies; the app supplies
  `flutter_secure_storage` with `first_unlock_this_device`, so the token is not
  migrated to a new device in a backup.
- **The keystore fails closed.** A locked keystore after a reboot reports
  "signed out" rather than crashing on launch, and reports the failure upward.
- **Analytics cannot exfiltrate content.** Blocked keys are stripped
  case-insensitively and the removal is reported.
- **No secrets in the widget tree.** Providers resolve them; nothing is passed
  through constructors or `const` maps.
- **The API base URL is compile-time or provider-injected**, never hardcoded in a
  screen.

## DOCUMENTATION

This file, `apps/mobile/README.md`, and `docs/PROJECT-STATUS.md`.
`apps/mobile/RETIREMENT.md` is removed with the shell it described.

---

## EXIT CRITERIA

| Criterion | Status |
|---|---|
| Architecture | PASS — feature-first over a core layer |
| Routing | PASS — one router, one redirect rule, tested |
| Dependency injection | PASS — Riverpod, nothing constructed in widgets |
| API client | PASS — `iconfess_api` wired, not duplicated |
| State management | PASS — notifiers; auth state drives the router |
| Theme | PASS — generated tokens, two modes |
| Design tokens | PASS — mirrored and staleness-checked |
| Safe areas | PASS — owned by `AppScaffold`, tested |
| Error handling | PASS — mapped, with recovery, tested |
| Persistence | PASS — namespaced KV plus keystore for secrets |
| Analytics foundation | PASS — vocabulary, blocklist, no PII |
| `flutter analyze` clean | PASS |
| `make verify` clean | PASS |

## VERDICT

**PASS WITH CONDITIONS — 8/10**

- **C-1** No release binary was compiled. `flutter build web` exceeded the
  sandbox's 25-minute budget and there is no Android SDK, so verification is
  `flutter analyze` (which does check `main.dart`) plus 44 tests. A real
  `flutter build apk` on a developer machine is the remaining check.
- **C-2** Every feature screen is a labelled placeholder naming the phase that
  builds it. Deliberate — a placeholder that pretends to be finished is how a repo
  ends up with screens nobody remembers are empty — but it means PHASE 18 has no
  user-visible product yet.
- **C-3** Analytics has no production transport. Events are recorded in debug and
  discarded in release until one is chosen.
- **C-4** No audio player. The foundation defines no playback abstraction yet;
  that belongs with the player phase rather than being guessed here.
- **C-5** The wireframe's neutrals (`#F7F7F3`, `#111111`, `#6F706B`) were not
  adopted. The established scale is close and is already contrast-checked;
  changing it would mean re-running the whole contrast audit for no visible gain.
- **C-6** The §81 AI-looking design audit has nothing to score yet. It applies
  once PHASE 20/21 produce real screens.
