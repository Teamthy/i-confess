# Security — I CONFESS §79

**Cases (CI `go test -race` must enforce, `server/internal/security/checks.go`):**
- authz 401 (no token), forged premium→402 (client never asserts plan, `billing.Verifier` server-side), IDOR 404/403 (userA cannot read/delete userB session/template), rate-limit 429 (login/register per IP), token reuse 401 (refresh replay→revocation), sqli 200 (tsvector sanitized), xss 200 (body escaped).

**Gates:** `voice_rights` 451 when AI grant without attestation, `entitlements` middleware `CanAccessVoice/Confession`, downloads licenced offline (`OfflineHoursAllowed`, signed URL 1h, `offline.Seal` AES-GCM encrypted metadata, revocation via DELETE), moderation `DRAFT→PUBLISHED` never auto-publish UGC, community `DRAFT→published`.

**Headers:** `next.config.mjs` `nosniff/DENY/strict-origin` `Permissions-Policy` none, `X-Request-Id`/`X-Trace-Id` for audit.

## Reporting a security vulnerability

Never open a public issue or pull request for a security concern — anything public in this tree is public, so do not paste passwords, keys, prayer text, journal content or confession text there.

Use GitHub's "Report a security vulnerability" from this repository's Security tab (private security advisory) instead. Until the repository is made private (plan item A), that private advisory route is the only supported channel.

Include: the endpoint or handler, steps to reproduce, and the commit or version. Maintainers confirm receipt inside the advisory; nothing is disclosed publicly until a fix is merged to `main`.
