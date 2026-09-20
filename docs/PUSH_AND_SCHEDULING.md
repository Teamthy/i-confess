# Push delivery & scheduled sessions

**356 tests pass**, `go vet` and `gofmt` clean.

This closes the largest remaining functional gap: schedules were stored and
listed, but nothing ever fired. A user could set a 6 AM routine and it would
silently never arrive.

---

## The bug underneath it

`POST /me/devices` accepted a `push_token` field and **threw it away** — it was
read into the request struct and never persisted. So even with a full push
stack there would have been nowhere to deliver. Tokens now live on the device
row, with their provider and a consecutive-failure count.

---

## Scheduling is a timezone problem

Almost all the difficulty is here, and it is unforgiving: a user's "6:00 AM"
means 6 AM *where they are*, which is a different UTC instant in summer than in
winter. Getting it wrong doesn't round — it wakes someone at 4 AM.

`internal/scheduler` therefore evaluates in the user's IANA zone, never against
a stored offset. Tests cover:

- **6 AM local, not UTC** — a Lagos schedule and a New York schedule with the
  same wall-clock time fire four hours apart.
- **DST transitions** — across the UK spring-forward the alarm stays at 06:00
  local even though the UTC offset changed overnight.
- **Non-existent local times** — 01:30 on a spring-forward day doesn't exist in
  London; it must not panic or fire at a wild hour.
- **Half-hour offsets** — `Asia/Kolkata` is UTC+05:30.
- **ISO vs Go weekdays** — the schema uses 1=Monday, Go uses 0=Sunday.
  Conflating them shifts every schedule by a day, so conversion lives in one
  function.
- **An invalid timezone never fires.** It is skipped rather than defaulting to
  UTC, which would fire at the wrong hour for most of the world.

One test assumption I got wrong and corrected: I'd asserted a Lagos 6 AM
instant shouldn't fire a London schedule. In September London is BST — also
UTC+1 — so they genuinely *should* coincide. The test now uses New York.

## Firing exactly once

The sweep examines a half-open window `(lastSweep, now]`, so two consecutive
ticks can't both claim one moment. But that alone is not enough — a restart or
a second replica can overlap.

The real guarantee is the database: `UNIQUE(schedule_id, occurrence_key)` on
`scheduled_deliveries`, where the key is the **local wall clock**
(`2026-09-02T06:00`). Two sweepers race on the insert; exactly one wins and
sends. Idempotency is a constraint, not application logic a restart could skip.

The key is local rather than UTC deliberately: if a user changes timezone, the
same intended moment keeps the same key instead of becoming a duplicate.

**Verified live** — the schedule fired on its own tick, and after a second
sweep the delivery table still held exactly one row.

## Not flooding people

Two guards, both tested:

- A **cold start** looks back only 15 minutes. A service starting at 10 AM
  must not replay the 6 AM reminder four hours late.
- A **long outage** caps the look-back at two hours. A week of downtime must
  not release a week of stale reminders at once.

A devotional reminder is worthless hours late; APNs `apns-expiration` and FCM
`ttl` are set to two hours so the *provider* drops it too rather than storing
and delivering it whenever the phone reconnects.

## Token lifecycle

The distinction that matters is **dead token vs transient failure**:

| Provider response | Action |
|---|---|
| APNs 410 / `BadDeviceToken` / `Unregistered` | Clear the token — the app is gone |
| FCM 404 / `UNREGISTERED` | Clear the token |
| 429, 5xx | Count a failure, retry later |
| 401/403 | Permanent: an operator must fix the credentials |

`ErrInvalidToken` is a separate error from `ErrPermanent` precisely so a dead
device is *purged* rather than retried forever. Five consecutive failures and
the device stops being a delivery target — continuing to push to dead tokens
wastes quota and damages standing with Apple and Google.

## Payload hygiene

Push payloads traverse Apple's and Google's infrastructure and surface on a
lock screen, so nothing sensitive goes in them. A notification carries the
schedule label, a duration, and a deep link (`iconfess://schedules/{id}/start`)
so tapping opens the session rather than the home screen. A test asserts the
body doesn't leak the user's email; tokens are masked in logs.

Notifications collapse per schedule: a phone offline overnight gets today's
reminder, not five stacked stale ones.

## Respecting the preference

`scheduled_sessions: false` genuinely stops delivery. The occurrence is still
*claimed and marked skipped* rather than left unhandled — otherwise the sweeper
would re-evaluate it every tick forever.

If the preference lookup errors, delivery proceeds. An unwanted notification is
more recoverable than a missed routine someone was relying on.

## The deletion policy test earned its keep

Adding `scheduled_deliveries` broke `TestEveryUserTableHasAPolicy` — the guard
written in the last slice caught a table I'd added with no erasure rule. It's
now explicitly `Erase`: a log of when someone was reminded to pray is
behavioural data about them.

That is the test doing exactly what it was for.

---

## Configuration

| Variable | Purpose |
|---|---|
| `APNS_KEY_PATH`, `APNS_KEY_ID`, `APNS_TEAM_ID` | .p8 token auth — one key, no annual expiry |
| `APNS_TOPIC` | App bundle id |
| `APNS_PRODUCTION` | `true` for the production gateway |
| `FCM_PROJECT_ID`, `FCM_ACCESS_TOKEN` | Android and web |

With nothing configured, reminders are **logged rather than dropped**, so the
schedule path is visible in development. Startup says so plainly.

---

## Delivery goes through the queue (IC-012)

The sweep used to talk to APNs and FCM itself, inside the one-minute tick. That
works until a provider slows down: the sweep then runs longer than its own
interval, and a process that restarts mid-send loses the delivery with nothing
recorded but a row claiming it was "sent".

With a durable queue installed, the sweep's only job is to decide *who* is due.
Each device gets a `notification.send` job, keyed by schedule, occurrence and
device, so a repeated enqueue of the same reminder to the same phone is a no-op;
retries, backoff and dead letters belong to the queue. The delivery row is
recorded as `queued` (migration 0012) because "sent" would be a claim nobody can
support yet — and that row is what someone opens during an incident.

Token lifecycle moved with the send. The queue handler is now the component that
hears "this token is gone", so it is the component that forgets it
(`Services.Devices`), and a provider outage counts against the device only once
it is permanent - otherwise an afternoon of FCM 503s would empty the device
table.

## The client half (IC-012)

The audit's finding was that the server was ready and nothing ever registered:
`postMeDevices` had no caller anywhere in `apps/mobile`, so every reminder ended
as `failed — no registered devices`.

- **Registration.** `apps/mobile/lib/src/core/push/` obtains the FCM/APNs token,
  re-registers on every rotation, and posts
  `{device_id, platform, push_token}` to `POST /me/devices`. The device id is
  generated once and persisted: Android's `ANDROID_ID` changes on a reinstall or
  a signing-key change, and a device that renamed itself would be two rows and
  two notifications.
- **Permission on a user action.** The OS prompt is raised by the "This device"
  tile in Settings → Notifications, never at launch. A refusal is an answer: no
  token is registered.
- **Deep links.** Tapping a reminder does what the reminder says:
  `POST /schedules/{id}/start` builds the session under the entitlement rules in
  force *now*, and the app opens `/player/{session_id}`. If that fails — the plan
  lapsed, the network is down — it falls back to the Activity tab rather than a
  player with nothing in it.
- **No Firebase configuration is not a crash.** `google-services.json` and
  `GoogleService-Info.plist` carry project credentials and are never committed,
  so a checkout without them starts, logs, and runs without reminders.

## Still outstanding

- **Sweeper coordination.** Runs in-process on a one-minute tick. The
  occurrence key makes multiple replicas *safe*, just wasteful.
- **Foreground display on Android.** A message the app is already handling is
  not raised as a notification by the system; showing it needs
  `flutter_local_notifications`, which is not wired yet.
- **Passkeys / WebAuthn**, **offline downloads** (§28), and the
  **Flutter / Next.js clients** remain untouched — still backend-only.
- **Silent/background push** for pre-downloading a session before its
  scheduled time (§28 + §47), which is what would make an offline 6 AM session
  work on a poor connection.

Next I'd do **offline downloads**: it is the remaining piece that makes a
scheduled session reliable for users on intermittent mobile data, which is a
large share of the target market.
