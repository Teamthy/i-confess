# iCONFESS web

One Next.js application, three audiences:

| Surface | Routes | Data |
|---|---|---|
| Public site | `/`, `/explore`, `/categories/*`, `/journal/*`, `/premium`, `/pricing`, legal pages | Bundled content (`lib/data.ts`) plus live plan pricing from the API |
| Bible | `/bible/**` — reader, search, topics, parallel reading, plans, private library | Canon from `lib/canon.json` (generated); text from the rights-gated API |
| Authenticated app | `/app/**` | Local session state plus the API when signed in |
| Admin console | `/admin/**` — overview, content, Bible operations, moderation, people, audit | The API's admin routes; every request re-authorised server-side |

There is no second web project. `apps/web` and `apps/admin` were folded into
this app (see `docs/54-WEB-CONSOLIDATION.md`); nothing else builds a browser
surface except the Go binary's embedded fallback SPAs
(`server/internal/webapp`, `server/internal/adminui`).

## Local development

```bash
npm ci
npm run dev            # http://localhost:3000 (binds 0.0.0.0 for sandbox previews)
npm run typecheck      # tsc --noEmit, strict
npm run build          # production build; CI runs both
```

The app talks to the API only through Next's same-origin `/api/*` proxy
(`next.config.mjs`), so there is no API origin in browser code and no CORS to
configure.

- `IC_API_URL` — where the proxy forwards. Defaults to the local Go server
  (`http://127.0.0.1:8080`).

## Previewing without the Go server

`server/` needs a Go toolchain some sandboxes cannot reach.
`scripts/dev-api.mjs` is a dependency-free fixture that mirrors the real
handler shapes, plan caps and error codes:

```bash
npm run dev:api                                  # :8081, from the canonical seed corpus
npm run bible:corpus                             # optional: public-domain KJV + WEB into /tmp
IC_API_URL=http://127.0.0.1:8081 npm run dev
```

Any valid email plus any password signs in against the fixture. With no API at
all, the site still renders: sign-in opens a clearly-labelled local session and
the Bible reader says plainly that Scripture text is served by the API.

## Conventions

- **One stylesheet.** `app/globals.css` holds the design system; there is no
  CSS framework and no second palette. The admin console uses the same tokens.
- **Structure may ship, text may not.** `lib/canon.json` is generated from
  `server/internal/bible` and contains no scripture text; verse text is always
  fetched under the per-translation rights the API enforces.
- **The server decides permission.** No surface is gated by a client-side role
  or plan check that the API does not repeat.
- **Strict TypeScript.** `tsc --noEmit` is clean and CI keeps it that way.
