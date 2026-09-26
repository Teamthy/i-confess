# RETIRED — replacement delivered

**Status:** the replacement has landed in PHASE 38 (ledger 45). This file is
kept as a historical record only; nothing in `apps/web` is retired any more.

**Decision:** recorded in `docs/03-TECHNOLOGY-DECISIONS.md`.
**Replaced by:** the PHASE 38 rebuild of this same directory —
`docs/45-PHASE-38-MARKETING-SITE.md`.
**Size at retirement:** 434 lines (now rebuilt: 30+ files against the PHASE 05
design system).

## What was carried over, deliberately

- The security headers from the old `next.config.mjs` (nosniff, frame-deny,
  referrer policy, permissions policy) — kept and reworded.
- The sitemap/robots idea — rebuilt, now generated from the live API.
- `/t/[token]` — kept because real share links exist in the wild; rebuilt
  against the new client with correct 404 behaviour.
- The pricing page — rebuilt on the live plans API.

Nothing else survived, and nothing was lifted without a reason recorded in
`docs/45-PHASE-38-MARKETING-SITE.md` §3–§4.

