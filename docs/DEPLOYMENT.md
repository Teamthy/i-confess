# Deployment — I CONFESS

## Production release path

Pull requests must pass backend/PostgreSQL/race, Flutter/Dart, design, container,
Kubernetes, CloudFormation and filesystem vulnerability checks. A `v*` tag is
the only image publishing trigger. Deployments are manual, require a full
`sha256:` image digest, assume AWS through GitHub OIDC, and target a protected
GitHub `staging` or `production` environment whose required reviewers provide
the approval gate. Never replace the digest with `latest`.

The Deploy workflow targets `iconfess-staging` or `iconfess-prod` and runs a
one-off migration Job before the backend rollout. Read the [database cutover
runbook](MIGRATION-ROLE-SEPARATION.md) before enabling `DB_ROLE_SPLIT_READY`:
it covers Secret references, grants, first install, migration-only operation,
legacy replicas, failure handling and safe rollback. Do not directly apply the
backend manifest as a substitute for the migration gate.

For AWS media, the stack in `infrastructure/media-cloudfront.yaml` keeps S3 private and
CloudFront alone reads it through OAC. Keep the matching private RSA key in the
runtime secret store (never CloudFormation or Git); mount it read-only and set:

```bash
MEDIA_BASE_URL=https://<distribution-or-media-domain>
CLOUDFRONT_KEY_PAIR_ID=<CloudFront-public-key-id>
CLOUDFRONT_PRIVATE_KEY_PATH=/run/secrets/cloudfront-private-key.pem
S3_BUCKET=<stack BucketName output>
S3_REGION=<region>
STORAGE_PROVIDER=s3
```

`MEDIA_BASE_URL` for CloudFront is a bare HTTPS origin (an optional trailing
`/` is fine), not an application path such as `/media`. A path prefix can route
an otherwise valid signature through the wrong cache behavior and is rejected.
The workflows do not create or update the CloudFront stack automatically.
Follow the [private media staging check](MEDIA-EDGE-VERIFICATION.md) to verify
the **deployed** policies, private legacy objects, range reads, warm-cache
signature enforcement, expiry and direct S3 denial before enabling delivery.
For the separate direct-R2 mode, follow the [private R2 playback check](R2-STAGING-PLAYBACK.md).
Neither check is a production deployment step.

## Other release checks

- PostgreSQL is the only backend database. The migrations shipped with the Go binary live in `server/internal/db/migrations/`; `DATABASE_URL` belongs to the runtime, `MIGRATION_DATABASE_URL` to the Job only.
- Stage the required API provider configuration (see `server/.env.example` and `server/internal/config/config.go`). Do not use a development billing verifier, local media storage or a log email sender in production. Test the installed providers; database readiness alone does not exercise them.
- The web application lives in [`web/`](../web/README.md) and proxies `/api/*` to the Go server via `IC_API_URL`. The Kubernetes backend workflow does not deploy the web or mobile clients.
- For product-specific release blockers, including voice rights, real audio and live storage verification, see [open work](OPEN-WORK-RECONCILIATION.md).
