## Summary

<!-- What does this PR do? One or two sentences. -->

## Motivation & context

<!-- Why is this change needed? Link any issue: Closes #123 -->

## Type of change

- [ ] Bug fix
- [ ] New feature
- [ ] Refactor
- [ ] Documentation
- [ ] Build / tooling / CI
- [ ] Other

## Scope

- [ ] One theme per PR (no unrelated changes bundled together)
- [ ] Under ~400 lines changed (including tests)

## Test plan

<!-- How did you verify this? Commands, manual steps, screenshots. -->

```text
go fmt ./...
go vet ./...
go test ./...
```

## Checklist

- [ ] I have run `go fmt`, `go vet`, and `go test` locally.
- [ ] I have added/updated tests for new behavior.
- [ ] I have updated documentation (`README.md`, `docs/API.md`) where relevant.
- [ ] I have kept the SQLite dev schema and PostgreSQL migration in sync (if schema changed).
- [ ] The change follows the [contributing guide](CONTRIBUTING.md).
- [ ] CI required checks are green on this PR.
- [ ] A written review has been posted on this PR.
- [ ] Handoff doc written to `docs/handoffs/YYYY-MM-DD-<topic>.md` (named after this PR) and committed on this branch.

<!-- Handoff path: docs/handoffs/YYYY-MM-DD-pr-###-<topic>.md -->
