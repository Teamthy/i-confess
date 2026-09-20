# PR D handoff — deployment and native CDN delivery

The prior local-only commit `2f70252` was unavailable in this checkout and on every remote ref. Its base `cc7ec64` is contained in merged PR #46. PR D was therefore reconstructed against current `main` without replaying already-merged work.

## Delivered

- Numeric non-root image user `10001:10001`; owned writable paths; Kubernetes identity, read-only root filesystem, seccomp, and live/ready probes aligned.
- Private/versioned/encrypted S3 bucket, CloudFront Origin Access Control, trusted key group, and bucket policy scoped to the distribution.
- Native CloudFront canned-policy URL signing. The former host rewrite of an S3 presigned URL was removed because CloudFront cannot validate an S3 query signature after the host changes.
- Pull-request container/Kubernetes/CloudFormation/security checks, tag-gated immutable image publishing, and approval-environment manual deployment by digest.
- No image was published and no infrastructure was deployed.

## Verification boundaries

Local Go formatting, vet, PostgreSQL suites, race suites, design checks, and signing tests are reproducible. GitHub-hosted container, CloudFormation lint, Flutter/Dart, and security jobs are authoritative where this sandbox lacks Docker/Flutter. Live CloudFront/S3 and Kubernetes verification requires an approved AWS environment and remains external.
