# iCONFESS Privacy Policy & Special Category Data Notice

**Last Updated:** November 22, 2026

## 1. Introduction
iCONFESS ("we", "our", or "us") provides a daily spiritual and prayer declaration platform. Because spiritual practices, prayer requests, and biblical confessions reveal religious and philosophical beliefs, this data constitutes **Special Category Data** under Article 9 of the EU/UK General Data Protection Regulation (GDPR) and equivalent international privacy standards.

## 2. Explicit Consent (GDPR Article 9(2)(a))
By creating an account, selecting confession categories, saving personal prayers, and using audio sessions, you explicitly consent to the collection, processing, and storage of your spiritual and faith-based declarations. You may withdraw this consent at any time by requesting account deletion or adjusting your profile preferences.

## 3. Data We Collect
- **Account Information:** Email address, display name, password hash (bcrypt), timezone.
- **Spiritual & Content Data:** Selected categories, favourite confessions, session history, scheduled declaration times, personal confessions.
- **Device & Security Data:** Device tokens for scheduled reminders, session identifiers, security telemetry (anonymised upon account erasure).
- **Billing & Subscriptions:** Store receipts (Apple App Store / Google Play) validated server-side. Payment card details are processed directly by Apple/Google and never touch our servers.
- **Session-builder requests:** When you describe what you need in your own words to build a session, that text is used to choose categories and length. See *AI assistance* below.
- **Newsletter (no account needed):** The email address you enter, where it came from (the website), when it subscribed and, if it did, when it unsubscribed. Every newsletter email has an unsubscribe link; one click stops further mail.
- **Contact form (no account needed):** Your name, email address and message, kept so we can reply. When email delivery is configured, the message is also forwarded to our support inbox.

### AI assistance
If the deployment enables it, the free-text description you give the session builder is sent to Anthropic's Claude API to interpret it. Only that description is sent: not your name, email, account identifier, saved confessions or history. If the service is unavailable or not enabled, a keyword matcher on our own servers is used instead. Anthropic processes the text under its commercial API terms.

## 4. Anonymity & Confidentiality
- **Community Declarations:** Confessions shared to the community feed are stripped of author identifiers before broadcast.
- **Audio Delivery:** Audio streams are generated and served via time-limited, cryptographically signed URLs. Audio binaries are never stored in databases.

## 5. Data Retention & Account Deletion (Right to Erasure)
- **7-Day Grace Period:** When deletion is requested (`POST /me/deletion`), all active sessions and tokens are immediately revoked. The account is permanently erased following a 7-day grace period.
- **Complete Erasure:** All personal profiles, preferences, listening histories, audio references, device tokens, and security telemetry records are irreversibly purged across all tables.
- **Newsletter and contact data:** These are not tied to an account, so account deletion does not remove them. Unsubscribing stops all mail at once. To have a newsletter address or contact message erased, write to the address in section 7.
- **Retained Records:** Only minimal compliance records (financial subscription receipts and consent withdrawal audit tombstones) are retained in detached, non-identifying form to satisfy legal obligations.

## 6. Sub-processors
We work with reputable infrastructure providers:
- **Cloud Infrastructure & Storage:** Cloudflare / AWS S3 (encrypted at rest and in transit).
- **Voice Synthesis:** ElevenLabs (processed via rights-governed server pipeline).
- **Transactional Email:** Postmark (deliverability without tracking).
- **AI Text Interpretation:** Anthropic (only the session-builder description, and only where enabled; see *AI assistance*).

## 7. Contact & Data Protection Officer
For data protection inquiries or to exercise your rights:
- **Email:** `privacy@iconfess.dev`
