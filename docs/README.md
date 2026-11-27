# Documentation

Where to look, by what you need. File names are kept stable because code comments and tests cite
them; this page is the map.

## Start here

| Need | Read |
|---|---|
| What the product is and must do | [00 Product source of truth](00-PRODUCT-SOURCE-OF-TRUTH.md), [01 Domain model](01-DOMAIN-MODEL.md) |
| How the system fits together | [02 System architecture](02-SYSTEM-ARCHITECTURE.md), [Backend architecture](BACKEND-ARCHITECTURE.md), [03 Technology decisions](03-TECHNOLOGY-DECISIONS.md) |
| What is outstanding | [Open work](OPEN-WORK-RECONCILIATION.md), [Project status](PROJECT-STATUS.md) |
| Where the last session stopped | [Handoffs](handoffs/) — newest file first |

## Specification series (00–67)

The numbered documents are the build record, one per area, written as each was built. They are
cited from code by number.

- **Foundations** — 00–08: product, domain, architecture, stack, repository, design system, IA, database, Go backend
- **Accounts** — 09 Auth, 10 Users, 46 Super-admin RBAC
- **Content** — 11 Content engine, 12 Governance, 37 Lifecycle, 39 Canonical audio, 40 Theological review
- **Audio and voice** — 13 Audio infrastructure, 14 Voice platform, 16 Queue worker
- **Sessions** — 15 Session engine, 17 Session APIs, 23 Session builder, 26 Templates
- **Mobile** — 18–22, 24–25, 27–29, 34, 61, 63
- **Premium and trial** — 30 Premium, 36 Billing and trial, 41 Trial engagement, 65 Trial expiry sweep
- **Community and safety** — 31 Moderation, 32 Community UGC, 42 Blocking and appeals, 62 Report a person
- **Bible** — 51 Bible, 52 Bible platform architecture, 53 Structure and web reader
- **Web** — 44 Platform audit, 45 Marketing site, 47–49 Reference IA and build reports, 54 Consolidation, 59 Token parity
- **Reliability** — 33 Audit coverage, 38 Retention and versioning, 43 Personalisation signals, 50 Cache invalidation, 55 CI gate repair, 58 Health checks, 60 Handler composition, 64 Recommendation reasons, 66 Dependency outages, 67 Deletion attribution

## Operations

[Deployment](DEPLOYMENT.md) · [Database role separation](MIGRATION-ROLE-SEPARATION.md) ·
[Disaster recovery](DISASTER-RECOVERY.md) · [Observability](OBSERVABILITY.md) ·
[Security](SECURITY.md) · [Media edge verification](MEDIA-EDGE-VERIFICATION.md) ·
[R2 staging playback](R2-STAGING-PLAYBACK.md) · [Engineering pipeline](ENGINEERING-PIPELINE.md)

## Topic references

[API](API.md) · [Content API](CONTENT-API.md) · [Authentication](AUTHENTICATION.md) ·
[Auth and profile](AUTH_AND_PROFILE.md) · [Auth definition of done](AUTH_DEFINITION_OF_DONE.md) ·
[Deletion and MFA](DELETION_AND_MFA.md) · [Billing](BILLING.md) ·
[Entitlement and audio](ENTITLEMENT_AND_AUDIO.md) · [Audio infrastructure](AUDIO-INFRASTRUCTURE.md) ·
[Offline and FCM](OFFLINE_AND_FCM.md) · [Push and scheduling](PUSH_AND_SCHEDULING.md) ·
[Platform services](PLATFORM_SERVICES.md)

## Licensing

[Model licences](MODEL_LICENSES.md) (machine-readable register: [model_licenses.json](model_licenses.json)) ·
[VoiceStudio licence](VOICESTUDIO_LICENSE.md) · [Privacy policy](legal/PRIVACY.md) · [Terms](legal/TERMS.md)

## Records

- [`audits/`](audits/) — dated, point-in-time reviews. Later work may have fixed what they found.
- [`archive/`](archive/) — superseded phase and status reports, kept for history. Their "complete"
  claims are not release approval; the open-work ledger is.
