#!/usr/bin/env bash
# Keep the command's exit status and expose failure context through the Checks
# API as well as the log archive (the archive may be unreachable in a sandbox).
set -uo pipefail
log=$(mktemp)
trap 'rm -f "$log"' EXIT
"$@" 2>&1 | tee "$log"
status=${PIPESTATUS[0]}
if [ "$status" -ne 0 ]; then
  python3 - "$log" "$*" <<'PY'
import re
import sys

lines = open(sys.argv[1], errors='replace').read().splitlines()
command = sys.argv[2]

if 'analyze' in command:
    # Analyzer diagnostics are more useful than its final count of lint infos.
    selected = [ln for ln in lines
                if 'error' in ln.lower() or 'warning' in ln.lower()] or lines[-120:]

elif ' test' in command:
    # `--reporter expanded` prints a line per test, so the last N lines are
    # almost entirely passing progress and the assertion that actually failed
    # has already scrolled past. That is not a theoretical loss: a run whose
    # trailing 220 lines were all successes reported four failing tests by
    # name and no reason for any of them, and the log archive it told you to
    # consult is not always reachable.
    #
    # So select by relevance, not position: each failure marker plus the block
    # under it, which is where Expected/Actual/the stack live.
    keep = set()
    for i, line in enumerate(lines):
        if re.search(r'\[E\]$|^Failing tests:|Test failed\.', line):
            # A little before for the test's own name, and enough after to
            # carry the matcher output.
            for j in range(max(0, i - 2), min(len(lines), i + 26)):
                keep.add(j)
    for i, line in enumerate(lines):
        if re.match(r'\s*(Expected|Actual|Which|Failing tests):', line):
            for j in range(max(0, i - 6), min(len(lines), i + 8)):
                keep.add(j)

    selected = [lines[i] for i in sorted(keep)] or lines[-220:]
    # The one-line summary is worth having whatever else was chosen.
    tail = [ln for ln in lines[-6:] if 'Some tests failed' in ln or ln.startswith('0')]
    selected += [ln for ln in tail if ln not in selected]

else:
    selected = lines[-220:]

# GitHub caps each annotation near 4 KiB and keeps ten per step, so the budget
# is what fits in ten chunks.
#
# For a test run keep the START of the selection: the selection is already
# nothing but failures, and the first ones are the ones with room for their
# Expected/Actual. Tailing here would drop exactly what was just gathered —
# a failure early in a long suite is the normal case, not the exception.
text = '\n'.join(selected)
text = text[:30000] if ' test' in command else text[-30000:]
for offset in range(0, min(len(text), 30000), 3000):
    chunk = text[offset:offset + 3000]
    chunk = chunk.replace('%', '%25').replace('\r', '%0D').replace('\n', '%0A')
    print(f'::error title=Check failure::{chunk}')
PY
fi
exit "$status"
