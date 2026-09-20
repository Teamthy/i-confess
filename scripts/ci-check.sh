#!/usr/bin/env bash
# Keep the command's exit status and expose failure context through the Checks
# API as well as the log archive (the archive may be unreachable in a sandbox).
set -uo pipefail
log=$(mktemp)
trap 'rm -f "$log"' EXIT
"$@" 2>&1 | tee "$log"
status=${PIPESTATUS[0]}
if [ "$status" -ne 0 ]; then
  python3 - "$log" <<'PY'
import sys
lines = open(sys.argv[1]).read().splitlines()
# Analyzer diagnostics are more useful than its final count of lint infos.
errors = [line for line in lines if 'error' in line.lower() or 'warning' in line.lower()]
# A step keeps at most ten error annotations. One grouped diagnostic preserves
# all findings rather than silently dropping everything after the tenth.
text = '\n'.join((errors or lines[-60:])[:150])
text = text.replace('%', '%25').replace('\r', '%0D').replace('\n', '%0A')
print(f'::error title=Check failure::{text}')
PY
fi
exit "$status"
