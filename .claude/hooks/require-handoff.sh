#!/usr/bin/env bash
# Stop hook: refuse to finish a session whose branch has committed work that is
# newer than its latest handoff in docs/handoffs/. See CLAUDE.md "Session handoff".
#
# Passes (exit 0, no output) when:
#   - the hook already blocked once this stop (stop_hook_active), so it never loops
#   - the branch has no commits beyond main that touch anything but docs/handoffs/
#   - a handoff was committed at or after the latest such commit
#   - a handoff file has uncommitted changes (written, about to be committed)
set -u

input=$(cat)
if [ "$(printf '%s' "$input" | jq -r '.stop_hook_active // false' 2>/dev/null)" = "true" ]; then
  exit 0
fi

cd "${CLAUDE_PROJECT_DIR:-.}" 2>/dev/null || exit 0
git rev-parse --git-dir >/dev/null 2>&1 || exit 0

base=$(git merge-base HEAD origin/main 2>/dev/null) || exit 0
[ -n "$base" ] || exit 0

last_work=$(git log -1 --format=%ct "$base..HEAD" -- . ':(exclude)docs/handoffs' 2>/dev/null)
[ -n "$last_work" ] || exit 0

last_handoff=$(git log -1 --format=%ct "$base..HEAD" -- docs/handoffs 2>/dev/null)
if [ -n "$last_handoff" ] && [ "$last_handoff" -ge "$last_work" ]; then
  exit 0
fi

if [ -n "$(git status --porcelain -- docs/handoffs 2>/dev/null)" ]; then
  exit 0
fi

jq -n '{
  decision: "block",
  reason: "This branch has committed work with no session handoff after it. Before finishing, write docs/handoffs/YYYY-MM-DD-<topic>.md from docs/handoffs/TEMPLATE.md (what changed, how it was verified, what is left open, how to pick it up), commit it on this branch and push."
}'
