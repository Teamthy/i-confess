# Check direct private R2 playback in staging

Use this for **R2 S3-compatible presigned GETs**, not for the [S3 + CloudFront deployment](MEDIA-EDGE-VERIFICATION.md). It exercises the Go storage adapter and presigner with a one-second generated tone. It does not need an API login, PostgreSQL, a worker, a real recording or a listener's object key.

**Status:** the checker has been exercised against a local TLS fixture. No live bucket result is recorded here. `make test`, CI and a passing CloudFront configuration check are **not** evidence of live R2 delivery.

## Before running

1. Obtain approval for the specific **non-production** account. Pre-create a dedicated, disposable, default-jurisdiction bucket named `icf-media-check-staging`. Keep all listener data out, and keep other writers off the fixture keys. Do not repurpose an application bucket or point production credentials at this test.
2. Independently verify in the Cloudflare control plane that `r2.dev` public access is disabled, no public custom domain or Worker serves the bucket, and no other route bypasses playback authorization. An unsigned denial at the **S3 API endpoint** does not test those other routes. Ensure object retention permits cleanup.
3. Use a short-lived R2 S3 access-key pair scoped to object read/write in **only that bucket** (the check uses PUT, HEAD, GET and DELETE; it never lists). Inject `ICF_R2_CHECK_ACCESS_KEY_ID` and `ICF_R2_CHECK_SECRET_ACCESS_KEY` from your staging secret manager. Set `ICF_R2_CHECK_ACCOUNT_ID` to the approved 32-character lowercase hex account ID. Do not put secrets on the command line, in Git, shell tracing, CI logs or release tickets.

## Run

From the repo root on an approved runner with a synchronized clock, Go and direct TLS access to R2:

```sh
ICF_R2_CHECK_CONFIRM_BUCKET=icf-media-check-staging make r2-staging-check
```

The target enables the `r2staging` test tag and disables Go test caching. It derives `https://<account-id>.r2.cloudflarestorage.com`, uses region `auto` and accepts **no** endpoint, input file or object key argument. It doesn't load an AWS profile, use ambient `S3_*` credentials, or follow redirects. It does not create buckets or change public access, policies, domains or lifecycle rules. Missing dedicated config is a failure, not a skip.

## What qualifies as a pass

| Check on the one generated key | Required observation |
| --- | --- |
| Legacy fixture | Create-only PUT (`If-None-Match: *`) under `audio/voices/_delivery-check/`. Authenticated HEAD confirms this run's metadata, WAV type and length and the deliberately cacheable `public, max-age=3600, immutable` header. This is object metadata, **not** public access. |
| Valid playback | Signed GET returns `200` and the exact WAV. A GET for bytes `44-59` returns `206`, the expected `Content-Range` and that run's 16 PCM bytes. Both response headers override the legacy value with `private, no-store`. HEAD above uses the storage credentials: a SigV4 URL signed for GET is not a signed HEAD. |
| Signature and expiry | Unsigned and changed-signature GETs on the known-existing key return `401`/`403` without the fixture audio; the original link still works. After the 20-second link expires, **new** full and range GETs with that same link are denied. A fresh signed GET must still return the same bytes. Redirects, `404`s and network failures do not count as denials. |
| Normal app upload | `S3Storage.Upload` updates the probe's own synthetic bytes; authenticated HEAD sees `private, no-store` on the object itself. Viewer playback must return the updated bytes rather than a stale response. |
| Cleanup | Inspect this run's marker, delete only its fixture and confirm the current key is absent. A conflict on creation never triggers deletion. A failed check still attempts cleanup, even if the request context expired. Unconfirmed ownership or deletion fails the check. |

Require `PASS: TestPrivateR2StagingDelivery` **and** the final verification/cleanup message; a skipped or unselected test is not a result. Record the tested revision, UTC time, approved environment reference, fixture ID and outcome. Do not capture URLs, query strings, credentials, bodies or raw SDK traces.

If the run is interrupted or a write acknowledgement or cleanup is uncertain, use the logged fixture ID to inspect **only** `audio/voices/_delivery-check/<fixture-id>.wav` after in-flight operations settle. Verify its `icf-smoke-run` metadata equals that ID before removing it. Never sweep the prefix. Retire the bucket/credential through the approved process. If any read, header, denial or cleanup check fails, record **not verified** and keep delivery gated; do not expose a public domain or remove the response override to make a test pass.

A live pass covers this fixture on this S3-compatible endpoint at the time of the run. It does not certify other buckets, CDN caches, API ownership/rights checks, browser or device playback, or recall previously issued links. [VE-007](OPEN-WORK-RECONCILIATION.md) remains open pending approved live evidence.
