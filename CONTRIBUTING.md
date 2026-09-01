# Contributing to i-confess

Thanks for contributing. i-confess is a scheduled spoken biblical-confession app
(backend in Go, mobile + admin console). This guide covers the workflow everyone
should follow so the repo stays clean and reviewable.

## Table of contents

1. [Workflow at a glance](#workflow-at-a-glance)
2. [Branching model](#branching-model)
3. [Conventional commits](#conventional-commits)
4. [Pull requests](#pull-requests)
5. [Local development](#local-development)
6. [Code style](#code-style)

## Workflow at a glance

```text
main (protected)
   â”‚
   â”œâ”€â”€ create feature branch
   â”‚       â”‚
   â”‚       â”œâ”€â”€ commit small, well-scoped changes
   â”‚       â”‚
   â”‚       â”œâ”€â”€ push branch
   â”‚       â”‚
   â”‚       â”œâ”€â”€ open Pull Request â†’ review â†’ merge
   â”‚       â”‚
   â”‚       â””â”€â”€ delete branch
   â””â”€â”€ repeat
```

**Never commit directly to `main`.** All changes land via a feature branch and a
pull request. This keeps `main` always deployable.

## Branching model

Branch names follow `type/scope`:

| Prefix   | Use for                                  | Example                    |
|----------|------------------------------------------|----------------------------|
| `feat/`  | new features                             | `feat/session-engine`      |
| `fix/`   | bug fixes                                | `fix/premium-fallback`     |
| `docs/`  | documentation                            | `docs/api-contract`        |
| `chore/` | build, tooling, non-code changes         | `chore/ci-lint`            |
| `refactor/` | restructuring without behavior change | `refactor/store-package`   |

```powershell
# 1. Start from a fresh main
git checkout main
git pull

# 2. Create and switch to a feature branch
git checkout -b feat/session-engine

# 3. ... make changes ...

# 4. Stage and commit (see conventional commits below)
git add -A
git commit -m "feat(sessions): cycle categories round-robin to fill duration"

# 5. Push and open a PR
git push -u origin feat/session-engine
```

## Conventional commits

Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/):

```text
<type>(<scope>): <short summary>

[optional body]
[optional footer]
```

Types: `feat`, `fix`, `docs`, `chore`, `refactor`, `test`, `style`, `perf`, `ci`.

Examples:

```text
feat(engine): deterministic session composition with voice fallback
fix(auth): reject tokens signed with an unexpected algorithm
docs(api): document schedule and favorite endpoints
chore(db): add canonical postgres migration for v1
test(engine): cover duration-fill and no-content paths
```

Rules:

- First line â‰¤ 72 characters, imperative mood ("add", not "added").
- One logical change per commit; don't bundle unrelated edits.
- Reference issues in the footer when applicable: `Closes #12`.

## Pull requests

1. Open a PR from your feature branch into `main`.
2. Fill the PR template (title, summary, test plan).
3. Request at least one review; address feedback.
4. Keep PRs small and focused â€” easier to review, fewer conflicts.
5. CI must pass (build + vet + tests) before merge.
6. Squash-merge when the branch contains WIP commits.

## Local development

See `README.md`. Quick start:

```powershell
go run ./cmd/server        # seeds a dev DB on first boot, listens on :8080
go build ./...             # compile check
go vet ./...               # static analysis
go test ./...              # unit tests
```

Pre-push checklist:

```powershell
go fmt ./...
go vet ./...
go test ./...
```

## Code style

- `gofmt` / `goimports` on every file (no manual alignment fights).
- Package layout follows the existing `internal/` structure:
  `api/`, `auth/`, `config/`, `db/`, `engine/`, `httpx/`, `models/`,
  `store/`, `media/`, `adminui/`, `seed/`.
- New handlers go in `internal/api/`; data access in `internal/store/`.
- Keep the SQLite (dev) and PostgreSQL (prod) schemas in sync â€” both live in
  `internal/db/schema.sql` and `migrations/postgres/` respectively.
- Write a test for any new non-trivial logic (see `internal/engine/engine_test.go`).

Questions? Open an issue or start a discussion.
