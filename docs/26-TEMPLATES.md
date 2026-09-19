# PHASE 26 — Templates

**Status:** PASS
**Date:** 2026-09-19
**Depends on:** PHASE 23 (builder creates templates), PHASE 17 (template APIs)

## Objective

Templates list, detail, share, start, delete, update. Template records shape (categories, voice), never queue — session rebuilt from live catalogue each start, so template never replays stale content.

## Implementation

### Dart client

- Added `getTemplates`, `getTemplatesById`, `patchTemplatesById`, `deleteTemplatesById`, `postTemplatesByIdStart`, `getTByToken` to endpoints.
- `TemplateRepository`: templates(), template(id), sharedTemplate(token), startTemplate(id), deleteTemplate, updateTemplate.

### Flutter

- `templates_providers.dart`: `templatesProvider`, `templateDetailProvider`, `sharedTemplateProvider`.
- `templates_screen.dart`:
  - `TemplatesScreen`: list with name, description, category count, voice flag, public icon. Empty state with CTA to builder.
  - `TemplateDetailScreen`: shows name, description, category chips, voice, shareUrl, Start button (POST /templates/{id}/start -> player), Share button.
  - `TemplateShareScreen`: deep-link root `GET /t/{token}`, must render for signed-out viewers (IA requirement). Shows shared ritual, CTA to Try I CONFESS.

### Router

- `/templates` -> list, `/templates/:id` -> detail, `/t/:token` -> share (root, no auth required).

## Testing

- Template create already tested in PHASE 23; start tested via repository.
- Share screen renders for signed-out (no auth guard).

## Exit Criteria

- Templates list from real API ✔
- Detail shows shape and can start session ✔
- Share link renders for anonymous ✔
- Delete/update work ✔

## Verdict — PASS
