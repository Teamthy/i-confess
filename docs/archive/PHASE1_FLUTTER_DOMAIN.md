# Phase 1 — Flutter domain and data layers

**404 Go tests + 35 Dart tests passing.** Everything in this phase is verified:
it is pure Dart with no Flutter import, so `dart test` runs it for real.

---

## What was built

| File | Purpose |
|---|---|
| `secure_token_store.dart` | Keystore-backed token storage (§24) |
| `models.dart` | Domain models with defensive parsing |
| `result.dart` | `Loadable` / `WriteResult` state machines (§58) |
| `cache.dart` | Offline cache with explicit staleness (§102) |
| `repository.dart` | Auth, profile, content, library repositories (§57) |

The layering is deliberate: **repositories know about caching and offline
policy, models know about parsing, and neither knows about widgets.** That is
what §57 asks for, and here it also means the majority of the client is
testable without a simulator.

---

## Decisions worth recording

### The read policy is one function, not one per repository

`Repository.cachedRead` implements: fetch → cache on success → fall back on
failure. Writing that separately in each repository is how an app ends up
showing week-old data on some screens and a blank page on others.

The fallback is **conditional on the kind of failure**, which matters:

| Failure | Behaviour | Why |
|---|---|---|
| Network unreachable | Serve cache, flag `fromCache` | The user is on a train, not unauthorised |
| 5xx | Serve cache, flag stale | Our outage should not blank their screen |
| **403** | **Fail — no cache** | Serving cache here shows content they are no longer entitled to |
| Corrupt cache | Treat as a miss | Interrupted writes and version skew happen; neither should crash launch |

Each of those is a test.

### TTL controls honesty, not availability

A four-day-old library still beats a blank screen on a plane. So the cache is
always served when offline — the TTL decides whether it is *labelled* stale.
`LoadLoaded` carries both `fromCache` and `stale` so the UI can say "showing
saved data" rather than presenting it as live.

Entitlement-adjacent data gets a short TTL (1 hour for bootstrap) because
showing a lapsed subscriber as premium is worse than showing a spinner.
Reference data like categories is cached for a day.

### Writes are never optimistically cached

§102 is explicit that the client must not claim a server change succeeded
before it did. A test asserts that a failed profile update leaves nothing in
the cache — otherwise a user believes an edit saved when it did not, which is
worst for exactly the settings that matter (privacy, personalization).

### Offline is a distinct type, not a status code

`NetworkException` is a sibling of `ApiError`, not a subclass. The single most
common client bug is showing "invalid password" when someone is simply
offline — it makes people change a password that was never wrong. `SignInFailed`
exposes `isOffline` so the UI cannot conflate them.

### Locked items are kept, not filtered out

A free user's session comes back with premium items present but `locked: true`
and no URL. The model keeps them and exposes `playable` separately, so the
player shows an upgrade prompt where the gap is rather than silently skipping.
`voiceDowngraded` is surfaced for the same reason: a silently substituted voice
looks like a bug, a declared one is a paywall the user can act on.

### Models degrade instead of throwing

Every field goes through a typed accessor with a fallback. An older app talking
to a newer server — and the reverse — must not crash on an unexpected shape. A
test feeds deliberately wrong types through.

One case is load-bearing: `Entitlements.fromJson({})` yields **free**, never
premium. A parse failure must not grant access.

### Token storage caches, and fails closed

Keychain reads cost milliseconds and happen on every request, so the value is
cached in memory. If the keystore is unavailable — which happens on Android
before first unlock after reboot — the user is reported signed out rather than
the app crashing on launch.

### Interests stay split by provenance

`Interests` has separate `explicit` and `inferred` lists rather than one merged
collection. Merging them in the model would make it structurally impossible for
the UI to honour §15, which forbids presenting an inference as something the
user said.

---

## Still deliberately abstract

`SecureStorage` and `LocalCache` are interfaces with no shipped production
implementation. The Flutter app supplies them (`flutter_secure_storage`, a file
or sqlite cache). Keeping them abstract means:

- this package has **zero dependencies**, which matters for something handling
  credentials
- it stays testable without platform channels
- the security requirement is explicit — whatever is plugged into
  `SecureStorage` must be hardware-backed, and the doc comment says so

---

## Phase 2 — Flutter presentation (next)

Widgets, navigation, the audio player, and the platform implementations of the
two interfaces above.

**I cannot verify Phase 2 here.** No Flutter SDK (~1.5 GB) and no simulator. I
will keep widgets thin so the unverified surface stays small, and I will mark
the handover honestly rather than implying it has been run.

## Running Phase 1

```bash
cd clients/dart
dart pub get && dart analyze && dart test
```
