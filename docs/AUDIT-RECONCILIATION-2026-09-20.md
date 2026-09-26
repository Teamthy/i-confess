# Audit reconciliation — 2026-09-20

This checklist reconciles the production certification findings with executable evidence. “Implemented” is not treated as synonymous with production-verified.

## Implemented and verified

- PR B billing boundary: store-signed webhook verification, notification-id idempotency, event-time ordering, and server-resolved entitlements remain covered by `internal/api/webhooks_test.go` and billing tests. Clients submit receipts; they do not assert Premium.
- Audit IC-001–IC-010, IC-014–IC-017, IC-019–IC-028 and IC-034: retained implementations and PostgreSQL/race coverage described in the certification remediation record.
- IC-011 implementation: native CloudFront signed URLs, private S3/OAC template, fail-closed production config, and unit tests.
- IC-018 implementation: non-root runtime, aligned Kubernetes policy, PR deployment checks, gated publishing, and approval-protected digest deployment.
- Flutter/Dart source checks: GitHub CI on merged PR #46 passed both client jobs. This branch must pass those jobs again before merge.

## Implemented, awaiting external verification

- Docker runtime identity and read-only filesystem: CI builds and inspects the image; this sandbox has no Docker daemon.
- CloudFront/S3: template and signer are testable locally; a real distribution, OAC denial of direct S3 reads, signed expiry/tamper behavior, cache behavior, and egress require approved AWS credentials and deployment.
- Kubernetes: manifest validation is automated; rollout, probes, network policy, secret mounts, autoscaling and disruption behavior require an approved cluster.
- APNs/FCM: signed app builds, provider credentials and physical devices are required for token registration, foreground/background/cold-start taps, and scheduled delivery.
- App Store/Play: signed builds, configured products and provider sandboxes are required for purchase, restore, renew, refund/revoke and out-of-order live notification exercises.
- Publishing/deployment workflows: deliberately not invoked; no tag, package, cloud resource or cluster was changed.

## Still missing or blocked

- Production content/audio certification: canonical text theological/licensing review and licensed generated audio are organizational/provider work, not proven by seed tests.
- Legal approval and explicit production consent copy remain owner/counsel gates.
- Live backup restore, monitoring/alerting, load, penetration and accessibility certification require approved environments and test plans.
- Store/provider credentials, Apple/Google signing identities, physical devices, AWS role/account, EKS cluster, DNS/TLS and GitHub environment reviewers are not available in this session.

The audit is therefore **not complete**. Automated implementation gaps are closed in reviewable code; the external release gates above remain explicit blockers.
