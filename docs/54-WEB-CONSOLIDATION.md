# Ledger 54 — Two web apps become one

**Date:** 2026-09-28
**Branch:** `arena/01a0e8b0-i-confess`
**Ask:** *"reconcile both web in this repo into just one without losing any major
layer, so we can have just web for user and admin console."*
**Result:** one Next.js application at `web/` serving the public site, the
authenticated app, the Bible platform and the admin console. `apps/web` and
`apps/admin` are deleted.

---

## 1. What the repository actually had

| Surface | Reality before this ledger |
|---|---|
| `web/` | 107 page routes. The restored original design, one stylesheet, static-first content, and the Bible platform from ledger 53. **No real API session, no admin console.** |
| `apps/web/` | 83 page routes, 138 TS files. The PHASE 38/39 rebuild: **the real API layers** — server-component client, authenticated browser client, auth context, consented analytics, `<audio>` player with Media Session — plus one admin page (`/admin/bible`), `/pricing` and `/t/{token}`. |
| `apps/admin/` | 3 files, retired in-tree since PHASE 04. Nothing to keep. |
| `server/internal/webapp`, `server/internal/adminui` | Vanilla-JS SPAs the Go binary embeds and serves. **Kept** — they are the fallback surfaces when no Node process is running. |

Two Next.js projects, neither built in CI, with overlapping routes and two
visual languages. The route comparison is the clearest statement of the
problem: of `apps/web`'s 83 routes, **73 already existed in `web/`**, and of the
ten that did not, four were `[id]`-vs-`[slug]` spellings of the same page.

## 2. The merge

**Direction:** `web/` is the shell — it has the newer design, more routes and
the Bible platform. Everything `apps/web` had that `web/` lacked was moved in.

**Layers moved, not rewritten** (`web/lib/`):

| File | What it is |
|---|---|
| `api.ts` | Server-component client. Server pages call the Go API over the loopback, so the API origin never reaches the browser. |
| `app-api.ts` | Authenticated browser client: typed union results, abort support, machine `code` alongside a human message. |
| `auth-client.ts`, `auth-context.tsx` | The real session. Token in memory, mirrored to `localStorage`, never in a URL; MFA-aware; 401 is reactive, not speculative. |
| `analytics.ts` | Consented event pipeline batching to `POST /analytics/batch`. |
| `player.tsx` | `<audio>` player with **Media Session** — lock-screen, headset and Bluetooth transport. |

**Pages moved:** `/pricing` (live plan table from `GET /subscriptions/plans` —
no price is ever hardcoded) and `/t/{token}` (share links that exist in the
wild keep resolving). `/admin/bible` moved with its logic intact.

**Nothing was lost by deletion.** The four `[id]` routes resolve through
`web/`'s `[slug]` params, `/bible/[...reference]` is superseded by
`/bible/passage/[...reference]`, the journal and legal content models were
already duplicated in `web/lib/data.ts`, and the five marketing photographs
were copied to `web/public/assets/marketing/`.

## 3. The admin console

`/admin` is now a section of the product rather than a second application:

| Route | What it does |
|---|---|
| `/admin` | Totals, content lifecycle counts, background queue, Bible platform summary |
| `/admin/content` | Categories, and confessions with lifecycle transitions |
| `/admin/bible` | Catalog review, per-use rights decisions, plans, verse-of-day, cross references, audio queue, health and sanitized metrics |
| `/admin/moderation` | UGC and editorial queue with recorded decisions |
| `/admin/users` | Admin roles: grant and revoke |
| `/admin/audit` | Security counters and recent privileged actions |

Three rules hold it together:

1. **The server decides.** No panel is gated by a client-side role check the
   API does not repeat. The sign-in gate is a courtesy; a non-admin who reaches
   a panel gets 403 from the API and sees that fact.
2. **No second design system.** The old console shipped an indigo/violet
   palette the design system had already retired — the audit called it "two
   visual languages in one repository". Every panel here uses the iCONFESS
   tokens. In the ported Bible console this was three constants, so its logic
   was not touched to restyle it.
3. **Four honest states.** Loading, empty, forbidden and unreachable are four
   different sentences, and none of them is a status code.

`robots.ts` excludes `/admin`, `/app`, `/t/` and the reader's private library.

## 4. Two findings worth recording

**`strict: false` was hiding real type errors.** The ported files failed to
compile in `web/` because discriminated-union narrowing on a boolean `ok`
discriminant does not work with `strictNullChecks` off. Turning **`strict: true`**
on produced **zero** errors across the merged app — every existing file already
satisfied it. The config was the bug, not the code.

**The web app was never built in CI.** With two projects and no authoritative
one, neither was compiled. There is one now, and `ci.yml` gained a `web` job
running `npm ci`, `npm run typecheck` and `npm run build` on every push.

## 5. One session, one analytics seam

`lib/store.ts`'s local `track()` now forwards to the consented server pipeline
when a real token exists, so there is one event name-space instead of two. The
Bible study store's sync used a token key (`iconfess:token`) that nothing ever
wrote; it reads the real one (`ic_token`) now, which is what made
`/api/v1/me/bible/*` mirroring actually reachable.

Sign-in calls the real API first. A wrong password is a wrong password — the
server says so. Only when the API cannot be reached **at all** does the form
open a clearly-labelled local session, because the same app has to stay
browsable as a marketing site with no backend in front of it. It never presents
a failed sign-in as a success.

## 6. Verified

- `npm run typecheck` (strict) — clean. `npm run build` — succeeds; 121 routes
  including all six admin pages, `/pricing`, `/t/[token]`, `robots.txt` and
  `sitemap.xml`.
- Every admin route, `/pricing`, `/t/{token}` and `/robots.txt` served 200
  against the dev fixture; the admin API answers 401 without a token and real
  corpus figures with one (39 categories, 78 confessions, 3 voices).
- Go suite untouched by this ledger and still green.
- `scripts/dev-api.mjs` gained the admin endpoints the console reads, mirroring
  the Go handler shapes, so the console is demonstrable without a Go toolchain.

## 7. Not done

- The embedded Go SPAs (`internal/webapp`, `internal/adminui`) still exist and
  still serve `/` and `/admin/` from the binary. They are the no-Node fallback;
  consolidating *those* is a separate decision with an availability trade-off.
- The admin console covers the six areas above. Of the API's 90 admin routes,
  the audio pipeline (`/admin/audio/*`), plan pricing editor (`/admin/plans`)
  and queue requeue are not yet surfaced — the endpoints exist and are
  documented, the panels are not built.
- `/app/**` still reads local state rather than the API for most panels; the
  session layer it needs is now present, but porting each panel is its own
  piece of work.
- Historical ledgers (03, 44, 45, 46) still describe `apps/web` as it was. They
  are history and were left alone; this ledger is the current record.
