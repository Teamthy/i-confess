# Entitlement-gated audio delivery

Covers PRD §11 (audio architecture), §25/§26 (plans), §27 (subscription
architecture) and §55 (never trust the client). This closes the shipping
blocker flagged in the previous slice: signed audio URLs were being minted for
any authenticated session owner, with no entitlement check.

## The rule

> The backend is the sole authority on what a listener may hear.

Nothing in this path reads an `isPremium`, `role` or `plan` value sent by a
client. The plan is loaded from the database for the authenticated user on
every request, and if that load fails the user is treated as **free** — a
database hiccup must never hand away the paid catalogue.

## Where the gate lives

```
GET /sessions/:id
      │
      ├─ load session, verify ownership          (403 if not yours)
      ├─ load plan from DB → Entitlements
      └─ for each item:
             CanPlayAudio{voice premium?, content premium?, asset status}
                 ├─ allowed → GenerateSignedURL(key, plan-specific TTL)
                 └─ denied  → audio_url = "", locked = true, lock_reason = …
```

Entitlement is evaluated **at URL-minting time, on every read** — not once when
the session is created. A plan can lapse between building a session and playing
it, so a create-time check would leave a cancelled subscriber with working
links until their session expired. `TestDowngradeRevokesAudioOnReread` pins
this: a session created as premium returns locked, URL-less items the moment
the subscription drops to free.

### Why blank the URL instead of erroring

A free listener whose session contains one premium item should still hear the
rest. Returning `locked: true` plus a `lock_reason` lets the player skip the
item and show an upgrade affordance, rather than failing the whole session or
leaving a silent unexplained gap.

## Signed-link lifetimes

| Plan | Playback | Download |
|---|---|---|
| Free | 1 hour | not permitted |
| Premium | 6 hours | 24 hours |

Free links are deliberately shorter-lived: a leaked URL has a smaller blast
radius, and lapsed entitlement takes effect sooner. Download links outlive
playback links because the file is meant to survive offline (§28) — but never
indefinitely, since an expiring link is what eventually makes a cancelled
subscription stop working.

## Session length as a plan capability

Free tops out at 15 minutes, premium at 3 hours. Enforced server-side in
`createSession`, returning **402 Payment Required** with a structured body:

```json
{ "error": "session length exceeds your plan limit",
  "reason": "session_duration_exceeds_plan_limit",
  "max_seconds": 900, "plan": "free" }
```

Free remains genuinely useful (§25) — 15 minutes is a real morning devotional,
not a teaser. A test asserts free is at least 10 minutes so a future tweak
cannot quietly gut it.

## Defects found and fixed this slice

**1. The dev media origin served every audio file unguarded.** The router
mounted `http.FileServer` over the media directory, so anyone who could guess a
path could stream any audio — bypassing entitlement entirely. Replaced with the
signed origin, which enforces the same signature and expiry rules as the
production CDN. Verified live: signed `200`, unsigned `403`, tampered `403`.

**2. The seed wrote public URLs into `audio_assets.url`.** Assets now store
storage **keys**; URLs are minted per request.

**3. `RoleBasedMiddleware` locked out SUPER_ADMIN.** It matched only the
explicitly-listed roles, so a super admin was refused on every role-scoped
route — exactly the account you need during an incident. Now admitted
everywhere.

**4. Malformed format string in `PrefixForVoice`.** `fmt.Sprintf("audio/%/voice/%s/", voiceID)`
produced a corrupt prefix and failed `go vet`, which is why the storage package
would not build. Fixed.

**5. Dead self-assignment `job.ID = job.ID`** in the job queue, also a vet
failure.

**6. Silent voice downgrade.** The session engine substitutes a free voice when
a free user requests a premium one — correct, but it did so invisibly, so the
listener saw a different voice with no explanation and the product looked
broken rather than gated. The session now reports `voice_downgraded` and
`voice_downgrade_reason`.

This last one is worth noting: my first version of the headline test asserted
the wrong behaviour (that premium items would come back locked). The test
failed, and investigating *why* revealed the substitution was happening
silently. The test was wrong; the finding was real.

## Merge notes

The remote had restructured into `server/` and added substantial schema and
models, but its storage providers were TODO stubs — `GenerateSignedURL`
returned nothing on all four, so audio delivery did not work. Rather than
replace that abstraction, I kept the remote's `ObjectStorage` interface and
implemented `LocalStorage` against it properly, porting the tested HMAC signing
across.

The remote also has its own `models.VoiceRights` with two overlapping
generations of fields (`Status`/`LicenseStatus`, `ExpiryDate`/`ExpirationDate`).
`rights/adapter.go` normalises that into the tested evaluator rather than
duplicating rules, and resolves ambiguity **against the platform**: if the two
status fields disagree, the more restrictive reading wins; if the two expiry
dates disagree, the sooner one governs. An `AllowedUse` of `recording` also
forces `AIGenerationAllowed` off — permission to play captured audio is not
permission to run a TTS model.

## Repo hygiene

- `server/bin/iconfess-server` — a **15.7 MB compiled binary** was committed.
  Untracked and gitignored; it bloats every clone and goes stale against source.
- `.history/` — 137 editor-backup files. Untracked and gitignored.
- Cleared 73 MB of regenerable placeholder audio from the workspace.

## Still open

- **Payment provider integration.** Entitlement is enforced, but nothing yet
  *sets* `premium` except an admin call. App Store / Play / web billing
  verification and webhook handling are the next slice (§27).
- **The generation job worker.** `generation_jobs` exists with idempotency,
  backoff and dead-letter columns; generation still runs inline in the admin
  request.
- **S3/GCS/Azure providers** remain stubs. `LocalStorage` is the only working
  one; the interface seam is in place for R2/S3.
- **Content-level premium flags** are modelled (`ContentPremium`) but not yet
  populated from category/confession data — only voice premium-ness is wired.
