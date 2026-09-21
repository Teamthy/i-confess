# PHASE 39 — Canonical audio coverage

**Gap closed:** `G-34 (PHASE 39: audio exists for all 78 canonical
confessions)` and `G-36 (PHASE 39: canonical bootstrap failure is fatal)`

**Verdict:** PASS

## OBJECTIVE

Remove the production gap between canonical text and playable catalogue
assets. `EnsureContent` already installs the 39 categories and 78 confessions
in every environment, but its old contract created no audio. The development
`Seed` path was the only place that wrote placeholder audio, so production
could boot with a catalogue that could not build a playable session.

This phase adds a separate, idempotent audio-completeness step. It uploads a
real object for every canonical confession variant, records its content
snapshot and provenance, and refuses startup if storage or the catalogue is
incomplete. The deterministic fixtures are intentionally labeled; they are
not represented as human recordings or provider-generated speech.

## INPUTS

- `server/internal/seed/ensure.go`: the all-environment canonical text
  bootstrap.
- `server/internal/seed/confessions_canonical.go`: the 78-row corpus and its
  four-variant duration ladder.
- `server/internal/storage`: the object-storage interface and key contract.
- `server/internal/store/audio.go`: audio upsert and served-asset queries.
- `server/cmd/server/main.go`: production boot ordering and failure handling.
- `docs/11-CONTENT-ENGINE.md`: G-34 and the production-empty-catalogue finding.

## IMPLEMENTATION

### `EnsureCanonicalAudio`

`seed.EnsureCanonicalAudio` runs after `EnsureContent` and requires the same
object-storage provider the handler will use. It:

1. resolves the active canonical Grace voice and all canonical categories;
2. finds each of the 78 corpus confessions by category and title;
3. calls `ContentStore.EnsureVersion` so every render is attributable to the
   exact text snapshot;
4. checks for an existing row and object before doing work;
5. uploads one deterministic WAV fixture for each `30s`, `1m`, `3m`, and `5m`
   variant; and
6. upserts a ready `audio_assets` row with its content-version foreign key,
   storage key, size, and `audio_source=bootstrap_fixture`.

The upload precedes the row write. An interrupted upload cannot leave a ready
row pointing at an object that never reached storage. A repeated boot finds the
same object and creates zero new rows.

The fixture uses the existing `media.ToneBytes` generator because this phase
has no speech-provider credentials and must not fabricate a claim that a voice
read the confession. It is a playable coverage fixture, not a theological or
narration approval. The normal rights-gated generation pipeline can replace it
using the same content-version and variant identity.

### Provenance migration

`0016_audio_provenance.sql` adds `audio_assets.audio_source` with the closed
vocabulary `generated`, `human_recorded`, and `bootstrap_fixture`, and installs
the live CHECK constraint. `AudioAsset` and `AudioStore.UpsertAsset` preserve
that provenance and clear a tombstone when a replacement render is written.
Seeded development fixtures now carry the same explicit label.

### Startup correctness

The server now calls `EnsureCanonicalAudio` after content bootstrap and before
constructing the handler. A failure in either content or audio bootstrap is a
fatal startup error. This closes G-36: an empty/partial catalogue cannot sit
behind a process that reports healthy merely because the database connection
worked.

No table or foreign key was added. The schema remains **66 tables / 77 foreign
keys**.

## TESTING

`TestEnsureCanonicalAudioCoversAllCanonicalConfessions` proves against a real
PostgreSQL database and local object storage that:

- 78 distinct canonical confession IDs have audio;
- all four duration variants are represented, for 312 ready assets;
- each database key resolves to an existing object;
- every asset is linked to a content version and labeled `bootstrap_fixture`;
- a second ensure creates zero rows.

The existing content, audio-generation, and storage suites remain green. The
phase proving commands are:

```sh
source /tmp/toolchain/env.sh
export TEST_DATABASE_URL='host=127.0.0.1 port=5432 user=iconfess dbname=postgres sslmode=disable'
cd server
export GOFLAGS=-modfile=/tmp/local.mod
go test ./internal/seed ./internal/store ./internal/db ./internal/api -count=1
gofmt -l internal cmd
go vet ./...
```

The repository contract checks remain:

```sh
EXPORT_ROUTES=1 EXPORT_ROUTES_PATH=/home/user/i-confess/design/routes.json \
  go test ./internal/api -run '^TestExportRouteTable$' -count=1
go run ./cmd/genspec /home/user/i-confess/contracts/openapi.json
cd ..
python3 design/test_ia.py
python3 scripts/check_dart_symbols.py
```

The live contract remains **306 route entries / 240 OpenAPI paths / 306
operations**, with **85/85 Dart symbols**. No client route or IA entry changed.

## SECURITY AND DATA REVIEW

- Canonical assets still pass through the existing signed-URL and entitlement
  gates; existence in object storage does not bypass Premium checks.
- The bootstrap fixture is not mislabeled as human-recorded or provider-
  generated audio. Provenance is database-enforced.
- Content version links prevent a later text edit from being mistaken for the
  words used to create an asset.
- Storage failures fail closed at startup instead of presenting a catalogue
  whose play buttons all fail later.
- The object key is deterministic and scoped by confession, variant, voice,
  language, and version, so replacement cannot silently change the meaning of
  an immutable key.

## EXIT CRITERIA

- [x] All 78 canonical confessions have four object-backed assets; proving
      command: `go test ./internal/seed -run '^TestEnsureCanonicalAudioCoversAllCanonicalConfessions$'`.
- [x] Assets are linked to content versions and carry explicit provenance;
      proving command: the same seed test plus `go test ./internal/store ./internal/db`.
- [x] The operation is idempotent; proving command: the second-pass assertion
      in `TestEnsureCanonicalAudioCoversAllCanonicalConfessions`.
- [x] A failed canonical bootstrap cannot leave a false healthy process;
      proving command: `go test ./internal/api ./internal/seed -count=1` and
      inspection of the fatal boot branch in `cmd/server/main.go`.
- [x] Table/FK counts remain 66/77; proving command:
      `go test ./internal/db -run '^TestPostgresSchemaLoads$'`.

## VERDICT

**PASS — `G-34` and `G-36` are closed by PHASE 39.**
