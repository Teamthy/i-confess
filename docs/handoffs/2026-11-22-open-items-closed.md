# Handoff — closing the open items: photos, trial UI, Bible release step, legacy pages

**Date:** 2026-11-22 · **Branch:** `claude/dreamy-faraday-7th0my` · **Commits:** `98d00bb`, `7f72c4f`, `4e2b9a2`, `55fd6fe` (+ this) · **PR:** [Teamthy/i-confess#97](https://github.com/Teamthy/i-confess/pull/97), merged through #98 with review fixes (this line said #96 until 2026-11-27)

## Goal

Close every open item from `2026-11-22-brand-ui-trial-repo.md`, using the best-fitting images where
photos were missing.

## What changed

**Photos (`98d00bb`)**
- The six categories whose watermarked previews were removed now have CC0 photos, found through
  Openverse with the search filtered to CC0 and public-domain licences:

  | Category | Photo |
  |---|---|
  | Breakthrough | Light through an archway |
  | Healing | Open hand in rainbow light |
  | Hope | Sunrise over the sea |
  | Peace | Still lake at dawn |
  | Prayer | Folded hands on a Bible |
  | Wisdom | Old book and lantern |

- Provenance is in `web/ASSET-SOURCES.md`, and `UNLICENSED_CAT_IMAGES` is empty.
- The "blank" image areas noted earlier were lazy-loading artefacts of fast-scrolling screenshots. A
  slow scroll confirms every image on the main pages loads.

**Trial UI (`7f72c4f`)**
- **Web:** `TrialCard` on Home and Subscription. It shows "Day N of 7" with progress. Its button
  calls `POST /subscriptions/trial/today`, then:
  - one session: opens the player, which waits for "Start listening"
  - Day 2: offers Morning and Evening
  - Day 6: opens the builder prefilled (the API now returns `category_slugs`)
- **Subscription page:** "Start 7-day trial" calls `POST /subscriptions/trial`. The fake
  renewal/cancel/restore demo is removed; plan cards point to the app, where purchases are verified
  store receipts.
- **Mobile:** a `TrialTodayCard` on Home, and `TrialOverview` / `TrialTodayResult` with
  `trialOverview()` and `startTrialToday()` in the Dart client.
- **Welcome golden:** re-rendered (the placeholder icon is now the real mark).
- **`pubspec.lock`:** completed. It was missing 40 packages `pubspec.yaml` needs; three
  SDK-pinned transitive packages moved to the versions current Flutter pins.

**Bible (`4e2b9a2`)**
- The image ships `iconfess-bible-import`.
- `server/k8s/bible-load-job.yaml` is a hardened Job that fetches, checksum-verifies and loads the
  registered translations with the migration Secret.
- The Deploy workflow and `deploy_backend.py` have a new `load-bible` operation, documented in
  `docs/DEPLOYMENT.md`.

**Legacy pages (`55fd6fe`)**
- `internal/webapp` and `internal/adminui` are deleted.
- `GET /` returns JSON service info, and any other unknown GET returns a JSON 404. Before, the
  catch-all served the old HTML app with 200 for every unknown path.
- Real `/admin/...` API routes are unaffected.

## Verification

**Run here:**
- **Go:** the race suite passes and golangci-lint v2.13 on Go 1.26.11 is clean. New tests cover:
  - the root and 404 behaviour
  - Day 6 `category_slugs`
  - the `load-bible` release driver (23 deploy tests)
- **Web:** typecheck passes. The trial flow ran end to end against the real API: start trial, then
  Day 1, then the player.
- **Flutter and Dart** (installed this session; pub.dev is now reachable):

  | Package | Result |
  |---|---|
  | `apps/mobile` analyze | No errors or warnings |
  | `apps/mobile` tests | 251 pass, including the new `TrialTodayCard` and `CategoryCard` tests, which had never run before |
  | `clients/dart` analyze and tests | 85 pass |

**Not run here:**
- **The Docker image build and `kubeconform`:** neither tool is available here; CI's deployment job
  runs them.
- **A real Bible load:** this sandbox can't reach `api.github.com/repos/seven1m/open-bibles`. Run the
  Deploy workflow with `load-bible`.

## Open items

- **Bible:** run Deploy → `load-bible` once in staging and production. The cluster needs outbound
  HTTPS to `api.github.com`.
- **Mobile Day 6:** the mobile builder opens at its first step without the prefill. Web prefills.
- **Carried over:**
  - the `PRIVACY.md` wording for the Claude parser
  - G-33 (breached passwords)
  - minister-voice licensing
  - a live staging request with `AI_PROVIDER=anthropic`

## Picking it up

Run `load-bible` in staging and confirm `/bible/translations` lists KJV and WEB. Then decide the
G-33 approach.
