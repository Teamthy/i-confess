# iCONFESS web app

## Local development

```bash
npm ci
npm run dev            # http://localhost:3000 (binds 0.0.0.0 for sandbox previews)
```

The app talks to the API exclusively through Next's `/api/*` proxy
(`next.config.ts`), so there are no `NEXT_PUBLIC_*` origins to configure.

- `IC_API_URL` — where the proxy forwards to. Defaults to the local Go
  server; any port is allowed while `NODE_ENV !== "production"` (production
  requires HTTPS or an explicit `IC_ALLOW_INSECURE_ORIGIN=1`).
- `IC_ADMIN_API_URL` — the admin proxy target (set by the dev script).

## Previewing without the Go server

`server/` needs a Go toolchain and module downloads that some sandboxes
cannot reach. `scripts/dev-api.mjs` is a dependency-free fixture for the
authenticated pages:

```bash
npm run dev:api                                  # serves :8081 off the canonical seed corpus
IC_API_URL=http://127.0.0.1:8081 npm run dev     # point the web app at it
```

Any valid email + any 12-character password signs in. The fixture mirrors the
real handler shapes, plan caps and error codes; its header comment lists every
deviation. It is a dev tool, not a test harness — CI's Go suite stays the
contract's source of truth.
