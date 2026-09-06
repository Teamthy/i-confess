# PHASE 10 — USERS & ACCOUNT SURFACE

**Branch:** `feat/phase10-users` · **Base:** `c366953` (PHASE 09)
**Gaps closed:** G-30 (password policy), G-32 (admin boundary) · **G-31 largely satisfied**
**Gate: PASS WITH CONDITIONS — 8/10**

---

## OBJECTIVE

PHASE 09 made the *door* safe. PHASE 10 makes what is behind it safe: the
account surface users touch every day — passwords, sessions, tokens — and the
privileged surface that sits beside it.

This phase was batched with four open gaps because they touch the same files.
Splitting them would have meant editing `handlers.go` four times and re-reading
the same 1,460 lines four times.

---

## WHAT WAS FOUND AND WHAT WAS DONE

### 1. A fix from PHASE 09 was being silently defeated in three places

PHASE 09 changed the `TokenTTL` default from 720h to 24h. But `Load()` only
applies that default when `TOKEN_TTL` is **unset**. When it was *set but
unparseable* — `"24hr"`, `"24"`, a stray newline from a secret manager — three
separate fallbacks took over:

| Location | Fallback |
|---|---|
| `internal/api/handlers.go:344` | `720 * time.Hour` |
| `internal/api/handlers.go:480` | `720 * time.Hour` |
| `internal/auth/auth.go:61` | `720 * time.Hour` |

A one-character typo in deployment config therefore reverted the whole of
PHASE 09's TTL work, in production, with no log line.

**Fix.** One exported constant, `auth.FallbackTokenTTL = 15 * time.Minute`.

The value matters as much as the deduplication: a runtime fallback has to
**fail short, not long**. A 30-day fallback turns a config typo into a
month-long window on a stolen token. Fifteen minutes is inconvenient and safe.

**And `config.Validate()` now rejects the input outright** — an unparseable
`TOKEN_TTL`, or any value over 24h, refuses to boot. The runtime fallback
should now be unreachable from configuration; it exists only for direct
`Config` construction in tests.

> **Lesson:** grep for the old literal after changing a default. The default
> was not the only place the value lived.

### 2. Password policy was `len < 8`, written three times

Every password entry point carried its own inline length check:

- `register` — `len(req.Password) < 8`
- `changePassword` — `len(req.NewPassword) < 8`
- `resetPassword` — `len(req.NewPassword) < 8`

Three copies of a policy means a fourth entry point gets no policy. It also
means the policy is invisible to anyone looking for it.

**Fix.** `internal/auth/password_policy.go` — one function, one decision point:

```go
func ValidatePassword(pw string) error
const MinPasswordLength = 8
const MaxPasswordLength = 256
```

Design decisions, deliberately:

- **No composition rules.** NIST SP 800-63B §5.1.1.2 advises against them.
  Forcing `Aa1!` does not produce strong passwords; it produces `Password1!`,
  which every attacker dictionary contains. Length is what buys entropy.
- **No forced rotation.** Rotating on a schedule pushes users toward
  `Spring2026!` → `Summer2026!`. Rotate on evidence of compromise instead.
- **`utf8.RuneCountInString`, not `len`.** `len()` counts bytes. A
  three-character CJK password is 9 bytes and sailed through a `>= 8` byte
  check while being 3 characters long.
- **256 upper bound.** Bounds bcrypt's cost against an attacker who submits a
  megabyte, and keeps hashing time predictable.
- **Whitespace-only rejected** — a password of twelve spaces passes a length
  check and protects nothing.
- **A 24-entry blocklist** of the passwords that appear in essentially every
  breach corpus (`password123`, `qwerty123`, …). This is a floor, not a
  defence — see gap G-33.

Wired into **three** call sites. `grep` confirms zero `len(req.*Password) < 8`
remain.

### 3. Dead code that embodied a vulnerability was deleted

`internal/store/users.go` held three functions that stored bearer tokens in
**plaintext** into a column named `token_hash`:

- `CreateRefreshToken` (7 lines) — 1 caller
- `RotateRefreshToken` (22 lines) — **0 callers**
- `RevokeRefreshToken` (5 lines) — **0 callers**

The single caller was a test.

**This is recorded as a correction, not a find.** An earlier reading of this
phase reported these as a live critical vulnerability — a column named
`token_hash` holding a raw token, with a `token_hash = currentToken` lookup,
looks conclusive. It was not traced. **No production caller exists; nothing
returns a `refresh_token` to any client.** There was no live vulnerability.

The live mechanism is `CreateAuthSession` / `RotateSession`. What is stored is
a *session identifier*, wrapped in a signed JWT. The identifier alone grants
nothing. **G-31 is therefore largely satisfied already** — `RotateSession`
stores a successor link, so reuse of a rotated token is detectable.

What was real is the trap: three exported, correctly-named functions that a
future contributor would reasonably wire up, at which point they *would* be a
critical vulnerability. Deleting them removed the trap and left a comment
above `CreateAuthSession` explaining what the column actually holds.

> **Lesson:** dead code that embodies a bug is a deferred bug. And "confirmed"
> requires a live path — trace callers before assigning severity.

### 4. The admin boundary was documentation, not a tested invariant

`Handler.route()` records an `Auth` field for every route — 44 of them carry
`auth: "admin"`. But **enforcement comes only from the `wrap` argument**. The
table is a record of intent. The existing parity tests compare paths, never
privileges.

Every one of the 44 is wrapped correctly today. Nothing kept the 45th that
way.

**Fix.** `internal/api/admin_authorization_test.go` — three tests:

| Test | What it proves |
|---|---|
| `TestEveryAdminRouteRejectsANonAdmin` | Iterates the live route table and asserts **all 44** reject an authenticated non-admin. A new admin route registered without its wrapper fails here. |
| `TestAdminRoutesStillWorkForAnAdmin` | Guards the first test from passing because everything 403s. |
| `TestRoleScopingIsEnforced` | Four cases across the role hierarchy. |

The scoping test was written against a wrong assumption and rewritten. The
generic wrapper is `auth.RequireRoleWithSessions(h.cfg.JWTSecret, sv)` — **no
extra roles named** — so `permitted` is `super_admin` only. Narrower wrappers
name their roles (`voice_manager`, `audio_producer`). That means
`content_admin` is correctly refused `/admin/categories`. The four cases now
assert both directions:

- `content_admin` denied a super-admin route ✓
- `content_admin` denied a voice-manager route ✓
- `voice_manager` **admitted** to a voice-manager route ✓
- `super_admin` admitted everywhere ✓

The gate also reads the role **off the session**, not off the token
(`state.Role` overrides `c.Role`), so the test promotes *then* signs in.
Without that, the tests pass for the wrong reason.

---

## TESTING

- `internal/auth/password_policy_test.go` — **4 tests**: rejects, accepts,
  non-ASCII rune counting, no composition rules.
- `internal/api/admin_authorization_test.go` — **3 tests / 7 assertions**,
  including the 44-route sweep.
- **48 test fixtures** across 10 files used `password123` or `password456` and
  had to be changed. None was a deliberate weak-password assertion — verified
  by grepping for weak/reject/blocklist context before replacing. **The
  breakage was itself evidence the blocklist works.**

**Full suite: 24 packages ok, 0 failures** (PostgreSQL 17).
**`make verify`: all checks passed** — fmt, design tokens, build, vet, lint, test.

---

## SECURITY REVIEW

| Concern | Status |
|---|---|
| Token TTL cannot be silently extended by config | **Fixed** — boot-time validation, 15m fallback |
| One password policy, one location | **Fixed** — 3 call sites, 0 inline checks remain |
| Byte-vs-rune length | **Fixed** — `utf8.RuneCountInString` |
| Plaintext token storage | **Not live**; the inviting dead code is deleted |
| Admin boundary enforced and tested | **Fixed** — 44/44 asserted |
| Role scoping is not decoration | **Fixed** — both directions asserted |
| Error messages | Generic; role gate "does not reveal which roles would suffice" |

---

## EXIT CRITERIA

| Criterion | Result |
|---|---|
| Build clean | ✅ `go build ./...` |
| Vet + lint clean | ✅ 0 issues |
| Full suite green | ✅ 24/24 packages |
| New behaviour covered by tests | ✅ 7 tests |
| Existing tests not weakened | ✅ 48 fixtures updated, 0 assertions removed |
| Documentation | ✅ this file + `PROJECT-STATUS.md` |

**GATE: PASS WITH CONDITIONS — 8/10**

### Conditions on this pass

1. **The 24-entry blocklist is a floor, not a defence.** Real protection needs
   a breach corpus (HIBP k-anonymity API or a shipped top-100k list). Tracked
   as **G-33**.
2. **Existing user passwords were not re-validated.** The policy applies at
   registration, change, and reset only. Accounts created with `password123`
   keep it until they next change it.
3. **`TOKEN_TTL > 24h` now refuses to boot.** Any deployment deliberately
   running longer sessions must change config or the constant. This is
   intentional, but it is a behaviour change.
4. **The route table still does not enforce anything.** The test makes an
   omission loud; it does not make it impossible. Making `Auth` authoritative
   would require reworking `route()` and is deferred.

---

## GAPS TOUCHED

- **G-30 password policy** — **CLOSED**
- **G-31 refresh rotation** — **CLOSED** (successor link already existed; dead
  plaintext code removed)
- **G-32 admin boundary** — **CLOSED** (tested; still not structurally enforced)
- **G-33 blocklist is not a breach corpus** — **NEW, OPEN**
