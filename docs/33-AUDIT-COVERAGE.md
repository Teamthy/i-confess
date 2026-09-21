# PHASE 33 — Audit coverage for content and moderation (G-41)

**Status:** PASS
**Date:** 2026-09-21
**Depends on:** PHASE 31 (moderation pipeline), PHASE 08 (admin audit sink)
**Closes:** G-41

## OBJECTIVE

Close condition C-2 (G-41) from `docs/31-MODERATION.md`: `audit_logs` did not
cover content and moderation actions. PHASE 31 wrote its trail to
`moderation_cases` and `content_moderation_history` — the tables moderators
query — while the admin-wide `audit_logs` sink was wired only where earlier
phases wired it (rights changes, audio QA transitions). An operator asking
"who touched this content, when, and why" had to union three tables and guess
which workflow each one belonged to.

The condition said: *decide one audit story rather than two.* This phase
decides it the only way the existing code allows: `audit_logs` is the sink for
**admin-visible actions**, every moderation and editorial status action writes
one, and the per-workflow trails keep recording what only they can know
(before/after pairs on the content itself). The trails answer different
questions; the sink makes all of them findable from `GET /admin/audit`.

## IMPLEMENTATION

### Server

- `internal/api/moderation.go` — every successful moderation action records
  one audit line through the existing `Handler.recordAudit` (the one writer,
  best-effort by design, actor from verified claims):
  | Action | entity | entity_id | detail | result |
  |---|---|---|---|---|
  | `report_created` | report | report id | reason + target | `open` |
  | `user_confession_submitted` | user_confession | id | `visibility=…` | `submitted` |
  | `user_confession_approved` | user_confession | id | review note | resulting status (`approved`/`published`) |
  | `user_confession_rejected` | user_confession | id | rejection reason | `rejected` |
  | `report_resolved` / `report_dismissed` | report | id | moderator note | decision |
  | `confession_qa_passed` | confession | id | QA note | `approved` |
  | `confession_qa_failed` | confession | id | the failing check names | `audio_qa` |
- `internal/api/admin.go` — `PATCH /admin/confessions/{id}` records
  `confession_status_{status}`. `ModerationStore.UpdateConfessionStatusAudited`
  now **returns the state it read under its own lock**, so the audit line's
  `result` (`ok` vs `unchanged`) is decided by the same read that wrote the
  history row — the two trails cannot disagree about whether anything moved.
  (A second read from the handler would have raced its own update and always
  reported "unchanged"; the first cut of this phase did exactly that and the
  test caught it.)
- Deliberate non-events: a duplicate report files no second `report_created`
  (the queue holds one case, the trail says so); a refused decision (400/404/
  409) records nothing — a failure that changed no state is not an action.

### Test

- `internal/api/audit_moderation_test.go` — `TestAuditLogsCoverContentAndModeration`:
  walks the full lifecycle against a live database and asserts each action
  above by reading `audit_logs` back: one `report_created` across two filings,
  actor = the acting account (never `admin`, never empty), details carry the
  moderator's note, results carry the state actually reached, the QA gate
  records pass and fail with the failing check names, a no-op PATCH is marked
  `unchanged`, a 404 PATCH leaves no trail at all, and `GET
  /admin/audit?entity=confession&entity_id=…` returns the moderation lines
  alongside the audio ones.

## VERIFICATION

- `go test -run TestAuditLogsCoverContentAndModeration ./internal/api` PASS;
  the PHASE 31 suite (`TestReportingEndToEnd`,
  `TestUserConfessionSubmissionAndReview`, `TestConfessionQAGate`) still PASS —
  no handler behaviour changed except the audit writes and the store's return
  shape (4 call sites updated).
- Full suite, vet, gofmt clean. No route added or changed, so
  `design/routes.json` and `contracts/openapi.json` regenerate byte-identical.

## Verdict — PASS

One audit story, three tables that no longer contradict each other.
