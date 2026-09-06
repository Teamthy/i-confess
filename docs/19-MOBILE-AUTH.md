# PHASE 19 — MOBILE AUTHENTICATION & ONBOARDING

## OBJECTIVE

Build Splash, Welcome, Onboarding, Sign In, Sign Up, Forgot Password,
Verification, Reset and Completion, ensuring validation, loading states, error
handling, keyboard behaviour and accessibility.

## INPUTS

`docs/09` (auth), `docs/18` (mobile foundation), the live server auth handlers in
`server/internal/api/handlers.go`, `server/internal/auth/password_policy.go`,
`server/internal/ratelimit/ratelimit.go`, and `clients/dart` (`iconfess_api`).

## DEPENDENCIES

PHASE 18 for the router, theme, token mirror, DI and error mapper. PHASE 09 for
the endpoints themselves — nothing in this phase invents an API.

---

## Findings before building

Four things were wrong before this phase wrote a line, and each is provable by a
test that now fails if it is put back.

**1. Sign-in never stored the token.** `ApiClient` writes to the `TokenStore` in
exactly one place — inside `_doRefresh` (`api_client.dart:102`). `AuthRepository.signIn`
returned the token to the caller and nothing persisted it. Meanwhile
`auth_controller.dart` carried the comment *"The token itself is persisted by the
client's transport, not here."* The comment was false. The consequence: a
listener who signed in was signed in for the lifetime of the process and signed
out on the next launch, with every request in between sent with no
`Authorization` header. `dart test` in `clients/dart` covers it now; removing the
one added line fails the test.

**2. The router was rebuilt on every auth transition.** `createRouter` began with
`ref.watch(authControllerProvider)`, so `routerProvider` recomputed and
`MaterialApp.router` received a *new* `GoRouter` each time the session changed —
discarding the navigation stack and restarting at `/splash`. Sign-in survived
this by luck, because the redirect sends a signed-in listener to `/home` anyway.
Registration did not: it was on its way to the confirmation screen and arrived at
Home instead. The router is now built once and told about auth changes through a
`refreshListenable`.

**3. A failed sign-in was reported as an ended session.** The auth handlers write
`{"error": ...}` with no `code` (`httpx.WriteError`), so `ErrorCodes.invalidCredentials`
never arrived — even though `contracts/openapi.json` documents the envelope as
`{error, code}` with *"Do not parse; use code"*. The generic mapper's
`401 || 403` branch therefore answered a mistyped password with *"Your session has ended. Sign in again to continue."* — a
claim about the listener's account the server never made. `ErrorMapper.describeAuth`
now maps the unauthenticated surface separately, and the generic mapping is
untouched for authenticated calls. This was condition C-2, closed the same day.

**4. `AuthRepository` could not verify, resend or reset.** It exposed `signIn`,
`register`, `signOut`, `signOutEverywhere`, `sessions`, `revokeSession` and
`requestPasswordReset`. The three endpoints the Verification and Reset screens
need — `POST /auth/verify-email`, `POST /auth/resend-verification`,
`POST /auth/reset-password` — had typed endpoint methods but no repository
method, so no screen could reach them.

---

## IMPLEMENTATION

### 1. The client learns the rest of the auth surface

`ApiClient` gains `attachSession(token)`, `clearSession()` and `hasSession()`.
Persisting is explicit rather than inferred from the response body: several
endpoints return a field called a token that is *not* a session token (the MFA
enrolment secret, a one-time verification token), and auto-persisting anything
under that key would overwrite a working session with a value that cannot
authenticate.

`AuthRepository` gains `verifyEmail`, `resendVerification` and `resetPassword`,
and persists the session on a successful sign-in and registration.

`register` now returns a sealed `RegisterOutcome` — `AccountCreated(token, userId)`,
`CheckYourEmail(message)` or `RegisterFailed(error)` — instead of
`WriteResult<String>`. The previous encoding used an empty string to mean
"address already exists", which made every caller carry a convention in its head.
`SignedIn` also carries `userId`, read from the `user` object the server returns,
so analytics attribution does not depend on a later profile fetch.

### 2. Validation mirrors the server rather than inventing rules

`features/auth/validators.dart` is a pure Dart mirror of
`server/internal/auth/password_policy.go`: 8 characters floor counted as
*characters*, 256 *bytes* ceiling (Go's `len()`), whitespace-only refused, and
the server's 24-entry breach blocklist. No composition rules, because the server
has none and NIST SP 800-63B says requiring a symbol makes people append "1!" to
the word they were already using.

The mirror is guarded by a test that reads the Go source and compares the
constants and the blocklist, so a change on the server fails a Flutter test
rather than diverging silently. It fails loudly rather than skipping if the Go
file cannot be found.

One deliberate asymmetry: **sign-in does not apply the password policy.** An
account that predates the policy may hold a six-character password the server
will happily accept, and refusing it on the device would lock someone out of an
account they are entitled to. Only "is it empty" is checked.

### 3. Keyboard behaviour is specified, not left to defaults

`AuthTextField` and `AuthPasswordField` take a `nextFocus`; the last field's
`textInputAction` is `done` and submitting it calls the same handler as the
button. Autocomplete is real: `AutofillGroup` wraps each form, with
`AutofillHints.email` / `password` on sign-in and `newPassword` on sign-up and
reset, so a password manager can save the pair. The second-factor field takes
`AutofillHints.oneTimeCode` and a text keyboard, because recovery codes are of
the form `ABCD-EFGH-JKMN` and a numeric keypad would lock out exactly the person
who lost their authenticator.

`AuthScaffold` owns the rest: content scrolls, tapping outside a field dismisses
the keyboard, and dragging dismisses it too. A fixed-height form is unusable on a
small phone with the keyboard up, which is most of this flow.

### 4. Loading, errors and the form's own state

`AuthFormState` is a mixin holding busy/submitted/error, so the four things that
go wrong in forms are solved once: a second tap while the first is in flight,
`setState` after the screen was popped, an exception escaping into the framework,
and the same failure worded differently on five screens. Field-level errors are
withheld until the first submission attempt; a form that complains the moment it
appears feels like it is already annoyed.

A failed submission keeps what the listener typed. `PrimaryActionButton` shows
its busy state in the button and disables itself, rather than putting a modal
spinner over the form — which is how people end up submitting twice.

### 5. Errors are mapped by which endpoint answered

`ErrorMapper.describeAuth(error, surface:)` takes an `AuthSurface`
(`credentials` or `token`), because 401 means two different things: bad
credentials on `/auth/login`, and an invalid or expired one-time token on
`/auth/verify-email` and `/auth/reset-password`. Without the distinction, a
listener who pasted a stale link was told their password was wrong.

A 400 passes the server's own message through, capitalised: "password must be at
least 8 characters" is actionable, where "that request was incomplete" is not. A
429 names the wait from `Retry-After` and says the account is not locked, because
the throttle never locks one (S20) and a listener who believes theirs is locked
will not try again.

### 6. The flows

| Screen | Behaviour |
|---|---|
| Splash | Brand only. No spinner: `main` resolves the keystore before `runApp`, so in release it is on screen for a frame, and if that read ever stalled an animation running forever would keep the GPU busy while the listener waited for nothing. It does not watch auth itself — the router does, and rebuilding the router is what moves the listener off it. |
| Welcome | Three ways forward: create an account, sign in, or browse. The third is a real button because Home and Explore are deliberately open (`AppRoutes.requiresAuth`). |
| Onboarding | Three slides, each answering a question a new listener has before handing over an address. Skippable, and skipping is recorded as completion — three slides you can only escape by finishing are a toll booth, not onboarding. |
| Sign in | Email, password, forgot-password link, and the MFA step in place. The second factor lives on this screen because the API takes the code in the same call; a separate screen would have to carry the password across a navigation boundary. A wrong code is a *field* error, not a page-level failure. |
| Sign up | Name, email, password, confirm. Timezone is sent so scheduled sessions mean the listener's evening. |
| Forgot password | Prefilled from sign-in. The confirmation carries the server's own hedge verbatim. |
| Verification | Resend with a 30-second local floor (the server allows four per fifteen minutes), and a field to enter the token by hand for someone whose link will not open. |
| Reset | Token, new password, confirm. Reads `?token=` so the day a deep link is wired the field is prefilled and the screen does not change shape. |
| Completion | Its own route, so pressing back cannot resubmit the token that produced it. Explains that a reset signs every device out. |

### 7. Two routes left the auth-flow guard

`verification` and `completion` are not in `isAuthFlow`. Registration issues a
session *before* the address is confirmed, so a listener who has just created an
account is signed in and mid-flow at the same time; putting verification in the
list throws them to Home at exactly the wrong moment. Neither route grants
anything, so both are harmless to reach at any time.

---

## SECURITY REVIEW

- **No account-enumeration oracle was introduced.** `/auth/register` answers a
  duplicate address exactly as it answers a fresh signup, minus the session
  (S65). Both non-failure outcomes land on the same verification screen with the
  same copy, and the test asserts no error banner and no session.
  `/auth/request-password-reset` is neutral server-side and the screen keeps it
  that way.
- **A wrong password is not reported as an ended session**, and a 401 on the
  credential surface offers no retry of the same input.
- **A half-finished sign-in stores nothing.** An MFA challenge leaves no token on
  the device; the second factor is what gates the session (S41).
- **A successful reset clears the local token.** The server revokes every session
  (S34); keeping this one would defeat it.
- **No secret travels through a route argument.** The MFA code stays in the
  screen's controller; only the one-time reset/verification tokens appear in a
  query string, and those are single-use by design.
- **Analytics carries nothing it should not.** Sign-in and sign-up track an event
  name and an opaque user id. The blocklist from PHASE 18 still applies.
- **The password is never logged, printed or persisted.** It lives in a
  `TextEditingController` and dies with the screen.

---

## TESTING

114 tests in `apps/mobile` (44 before this phase), 42 in `clients/dart` (35
before), and 3 Go tests / 13 subtests in `internal/api` covering the error
codes the handlers now emit. The last of the 114 is a golden for the welcome
screen: it renders against the real theme with Roboto committed under the
family names the tokens request, and it fails when a brand token changes —
verified by injecting one.

| Area | Coverage |
|---|---|
| Validation | 20 tests: grapheme vs byte counting, the breach list, no composition rules, and a cross-language guard that reads `password_policy.go` |
| Errors | A 401 on login is not a 401 on an authenticated call; 403 stays vague; a 400 carries the server's words; offline is not a credential problem; 429 names the wait; 503 is the server's fault |
| Sign in | happy path (token stored, analytics, routed to Home), wrong password, offline, empty fields send nothing, a short legacy password is accepted, MFA challenge, wrong MFA code |
| Keyboard | `next` vs `done`, autofill hints per surface, `AutofillGroup` present, the reveal toggle flips and is announced |
| Sign up | new account, duplicate address, breached password, mismatched confirmation, a server rejection keeps the form filled |
| Reset | valid token (password changed, local session cleared), expired token |
| Verification | confirm by hand, resend cooldown, expired token still offers a resend |
| Onboarding | three slides, the pager's spoken position, skip persists |
| Accessibility | a failure is a live region, 44pt targets, the wordmark is excluded from semantics |
| Routing | a signed-in listener is not bounced off verification, cannot re-enter sign-in, completion is reachable signed out, a malformed reason does not crash |

**Six defect injections, each failing its own test:**

| Injection | Test that caught it |
|---|---|
| `createRouter` watches auth again | registration lands on Home, not verification |
| `describeAuth` delegates to the generic mapper | "That didn't match" disappears |
| the token-surface branch removed | "That code has expired" disappears |
| sign-up submits without validating | a breached password reaches the API |
| the resend cooldown removed | no local rate limit |
| onboarding completion not persisted | the tour would be shown again |

Each was restored and re-verified: `flutter analyze` clean, all 96 passing.

---

## EXIT CRITERIA

| Requirement | Result |
|---|---|
| Splash | PASS — hands off, never dead-ends |
| Welcome | PASS — create, sign in, browse |
| Onboarding | PASS — three slides, skippable, remembered |
| Sign In | PASS — including MFA in place |
| Sign Up | PASS — validation, neutral duplicate handling |
| Forgot Password | PASS — neutral confirmation |
| Verification | PASS — resend and manual entry |
| Reset | PASS — clears the local session |
| Completion | PASS — its own route, explains the sign-out |
| Validation | PASS — mirrors the server, guarded against drift |
| Loading | PASS — in-button, no double submit |
| Errors | PASS — mapped per surface, with recovery |
| Keyboard behaviour | PASS — actions, autofill, dismissal |
| Accessibility | PASS — live regions, labels, targets |
| `flutter analyze` clean | PASS |
| `make verify` clean | see verdict |

## VERDICT

**PASS WITH CONDITIONS — 9/10**

- **C-1** No release binary. As in PHASE 18: `flutter build apk` needs an Android
  SDK the sandbox does not have. Verification is `flutter analyze` plus 96 tests.
- ~~**C-2** The auth handlers emit no stable `code`.~~ **CLOSED.** All six public
  auth handlers now use the `writeCode` helper `deletion.go` and `mfa.go` already
  used, emitting `AUTH_INVALID_CREDENTIALS`, `AUTH_TOKEN_INVALID`,
  `AUTH_VALIDATION_FAILED`, `AUTH_UNAVAILABLE` and `AUTH_ACCOUNT_UNAVAILABLE` —
  the last a new code, deliberately *not* `AUTH_ACCOUNT_SUSPENDED`, because the
  message withholds the reason and a code that named it would disclose what the
  message refuses to (S80). `ErrorMapper.describeAuth` reads the code first and
  keeps the status branch for a server that has not been upgraded. 5xx responses
  stay uncoded: no client branches on why the server failed.
  Pinned by `internal/api/auth_error_codes_test.go` (13 subtests) and six Dart
  tests covering both the code path and the fallback.
- ~~**C-3** No deep links.~~ **CLOSED on the app side.** The router now accepts a
  cold-start location, serves the exact path the email sends
  (`/verify-email?token=…`) on top of the in-app `/verification`, and the token
  fields extract the token from a pasted link — pasting the whole URL is the
  obvious move and used to fail with a wrong diagnosis. All four are pinned by
  widget tests, including one that boots the router straight from the link. What
  remains is a manifest change — Android intent filters and iOS associated
  domains — and that needs a real domain, so it lands with the release work, not
  this phase.
- **C-4** Email verification is not enforced by the client. Registration issues a
  session before the address is confirmed and the app lets the listener continue,
  which matches the server. If the product later requires confirmation before
  first use, that is a server-side entitlement decision, not a mobile one.
- **C-5** Social sign-in is not built. `/auth/social/{provider}` exists on the
  server and has no screen here; the brief for this phase does not ask for it.
- **C-6** The §81 AI-looking design audit still has nothing to score. It applies
  when PHASE 20/21 produce the content screens this flow leads into.
