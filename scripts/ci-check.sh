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
import sys
lines = open(sys.argv[1]).read().splitlines()
# Analyzer diagnostics are more useful than its final count of lint infos.
errors = [line for line in lines if 'error' in line.lower() or 'warning' in line.lower()]
# GitHub caps each annotation near 4 KiB and keeps ten per step.
selected = (errors or lines[-120:]) if 'analyze' in sys.argv[2] else lines[-220:]
text = '\n'.join(selected)[-30000:]
for offset in range(0, min(len(text), 30000), 3000):
    chunk = text[offset:offset + 3000]
    chunk = chunk.replace('%', '%25').replace('\r', '%0D').replace('\n', '%0A')
    print(f'::error title=Check failure::{chunk}')
PY
fi
exit "$status"
