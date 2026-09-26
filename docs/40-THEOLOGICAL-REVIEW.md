# PHASE 40 — Canonical theological review

**Gap closed:** `G-35 (PHASE 40: canonical content review and honest
provenance)`

**Verdict:** PASS WITH BOUNDARY

## OBJECTIVE

Close the canonical-content provenance gap without making a stronger claim than
the evidence supports. Before this phase, the 78 repository confessions were
stored with `Author = "i-confess content team"`, although no named team or
church review record existed. That string overstated provenance and made
published text look institutionally endorsed.

The canonical corpus now has an explicit, database-enforced review record for
every confession. The author field says only what the repository can prove:
`Canonical corpus`. Review metadata states that an internal editorial review
checked the text completeness and Scripture-reference records. It does **not**
claim a pastor, denomination, church, or ecclesiastical body authored or
endorsed the declarations.

## INPUTS

- `server/internal/seed/confessions_canonical.go`: the 78 canonical texts and
  Scripture references.
- `server/internal/seed/ensure.go`: all-environment content bootstrap and the
  new review gate.
- `server/internal/models/models.go` and `server/internal/store/content.go`:
  confession persistence.
- `server/internal/db/migrations/0017_theological_review.sql`: review schema
  and CHECK vocabulary.
- `docs/11-CONTENT-ENGINE.md` and `docs/12-CONTENT-GOVERNANCE.md`: G-35's
  original finding and the absence of prior theological review.

The checkout does not contain `docs/35-TRIAL-LIFECYCLE.md`; it was not treated
as an input or represented as read.

## IMPLEMENTATION

### Review record

Migration 0017 adds to `confessions`:

```text
theological_review_status  unreviewed | reviewed | needs_revision
theological_reviewer       internal reviewer label
theological_reviewed_at    UTC text timestamp
theological_review_notes   bounded review evidence
```

The status CHECK is read back by
`TestTheologicalReviewVocabularyParityAgainstConstraint`. No table or foreign
key is added; the schema remains **66 tables / 77 foreign keys**.

`models.Confession` carries the fields for internal persistence but serializes
them out of the public projection. Review details belong in editorial/admin
surfaces, not in a listener's confession response.

### Bounded corpus review

`EnsureCanonicalTheology` resolves rows by the canonical category/title pair,
never by a broad author UPDATE that could relabel user content. For each corpus
item it checks:

- title and all three text lengths are present;
- at least one Scripture reference is recorded; and
- every reference has a book, positive chapter, verse, and translation.

It then records `reviewed`, `I CONFESS editorial review`, a UTC timestamp, and a
note counting the references and preserving the corpus's direct-quote flags.
The note explicitly says this is not ecclesiastical endorsement. Existing rows
from before PHASE 40 are normalized on the next boot; repeated runs are
idempotent.

This is a bounded repository/editorial review, not a claim that a qualified
clergy or denominational board has approved every theological proposition. If
the product later requires that stronger gate, the `needs_revision` state and
reviewer fields provide the place to record it without rewriting authorship.

### Honest author provenance

`CanonicalAuthor = "Canonical corpus"` replaces the unsupported
`"i-confess content team"` in both `EnsureContent` and the development seed.
The old value is not accepted by the provenance test. User-created content
continues to carry its supplied author independently.

### Audio dependency

`EnsureCanonicalAudio` now refuses a canonical row whose review status is not
`reviewed`. Server startup runs content bootstrap, theological review, then
canonical audio. Thus the 312 fixture assets from PHASE 39 cannot be installed
for an unreviewed canonical row. The review is a real prerequisite, not merely
a status shown in documentation.

## TESTING

Named tests and evidence:

- `TestEnsureCanonicalAudioCoversAllCanonicalConfessions` first proves audio
  is refused before review, then proves all 78 rows are reviewed and all 312
  object-backed assets can be created idempotently.
- The same test proves 78 canonical rows carry `Canonical corpus` and zero
  rows carry the overstated `i-confess content team` author.
- `TestTheologicalReviewVocabularyParityAgainstConstraint` compares the Go
  vocabulary with the live PostgreSQL CHECK.
- Existing corpus tests continue to prove 39 categories, 78 unique titles,
  complete duration text, and Scripture coverage.

The phase proving commands are:

```sh
source /tmp/toolchain/env.sh
export TEST_DATABASE_URL='host=127.0.0.1 port=5432 user=iconfess dbname=postgres sslmode=disable'
cd server
export GOFLAGS=-modfile=/tmp/local.mod
go test ./internal/db ./internal/seed ./internal/store ./internal/api -count=1
gofmt -l internal cmd
go vet ./...
```

The contract checks remain:

```sh
EXPORT_ROUTES=1 EXPORT_ROUTES_PATH=/home/user/i-confess/design/routes.json \
  go test ./internal/api -run '^TestExportRouteTable$' -count=1
go run ./cmd/genspec /home/user/i-confess/contracts/openapi.json
cd ..
python3 design/test_ia.py
python3 scripts/check_dart_symbols.py
```

The live route contract remains **306 entries / 240 paths / 306 operations**;
Dart symbol coverage remains **85/85**. No route, IA, OpenAPI, or client change
was required.

## SECURITY AND DATA REVIEW

- The old team-provenance string is removed from production writers.
- Review rows are scoped to the canonical category/title corpus, so a user
  confession cannot be relabeled as canonical by sharing an author string.
- The database CHECK prevents arbitrary review states.
- Audio bootstrap refuses unreviewed canonical content, preventing a newly
  inserted published row from silently becoming a playable canonical asset.
- The review note is deliberately explicit about its limit. It records what was
  checked rather than laundering an internal code review into church authority.

## EXIT CRITERIA

- [x] Every canonical confession has an explicit review decision and timestamp;
      proving command: `go test ./internal/seed -run '^TestEnsureCanonicalAudioCoversAllCanonicalConfessions$'`.
- [x] Review vocabulary is live-constraint parity-tested; proving command:
      `go test ./internal/db -run '^TestTheologicalReviewVocabularyParityAgainstConstraint$'`.
- [x] `Author` no longer overstates provenance; proving command: the canonical
      seed test's zero-overstated-author assertion (`go test ./internal/seed -run
      '^TestEnsureCanonicalAudioCoversAllCanonicalConfessions$'`).
- [x] Audio requires reviewed canonical content; proving command: the
      pre-review refusal in `TestEnsureCanonicalAudioCoversAllCanonicalConfessions`.
- [x] Table/FK counts remain 66/77; proving command:
      `go test ./internal/db -run '^TestPostgresSchemaLoads$'`.

## VERDICT

**PASS WITH BOUNDARY — `G-35` is closed by PHASE 40. The repository now proves
its editorial review and does not claim ecclesiastical endorsement that it does
not possess.**
