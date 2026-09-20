#!/usr/bin/env bash
# Bootstrap the development sandbox: Go, PostgreSQL, and a working module graph.
#
# This script exists because the interactive sandbox starts empty and is
# discarded between sessions, while work in it is lost the moment it is not
# committed. Twice now a session's commits were unrecoverable for exactly that
# reason, and both times the toolchain had to be rediscovered from scratch -
# which is an hour spent on the environment rather than on the product. The
# recipe belongs in the repository.
#
# It is NOT used by CI. GitHub Actions installs Go and PostgreSQL itself; this
# script is only for a bare sandbox where the network reaches PyPI, npm and
# github.com but not proxy.golang.org or go.googlesource.com.
#
# Usage:
#   scripts/sandbox-bootstrap.sh          # toolchain + a running PostgreSQL
#   source /tmp/toolchain/env.sh          # put go/psql on PATH for this shell
#   cd server && go test -modfile=/tmp/local.mod ./...
#
# Nothing here is committed to the module graph: the alternate modfile lives in
# /tmp and the real go.mod/go.sum stay exactly as CI sees them.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TOOLCHAIN=/tmp/toolchain
GO_VERSION=1.27.1
PG_VERSION=17.10.0-beta.17

mkdir -p "$TOOLCHAIN"

# ---------------------------------------------------------------------------
# Go
# ---------------------------------------------------------------------------
if [ ! -x "$TOOLCHAIN/go/bin/go" ]; then
  echo "bootstrap: fetching Go $GO_VERSION"
  mkdir -p "$TOOLCHAIN/download"
  pip3 download "go-bin==$GO_VERSION" --no-deps -d "$TOOLCHAIN/download" >/dev/null
  python3 - "$TOOLCHAIN" "$TOOLCHAIN/download" <<'PY'
import sys, zipfile, glob, os
dest, dl = sys.argv[1], sys.argv[2]
wheel = glob.glob(os.path.join(dl, "go_bin-*.whl"))[0]
zipfile.ZipFile(wheel).extractall(dest)
PY
  chmod +x "$TOOLCHAIN"/go/bin/*
  chmod -R +x "$TOOLCHAIN"/go/pkg/tool/linux_amd64/ 2>/dev/null || true
fi

# ---------------------------------------------------------------------------
# PostgreSQL (embedded build; ships without psql, hence `postgres --single`)
# ---------------------------------------------------------------------------
if [ ! -x "$TOOLCHAIN/pg/native/bin/postgres" ]; then
  echo "bootstrap: fetching PostgreSQL $PG_VERSION"
  mkdir -p "$TOOLCHAIN/pgdownload"
  (cd "$TOOLCHAIN/pgdownload" && npm pack "@embedded-postgres/linux-x64@$PG_VERSION" >/dev/null)
  mkdir -p "$TOOLCHAIN/pg"
  tar xzf "$TOOLCHAIN"/pgdownload/embedded-postgres-*.tgz -C "$TOOLCHAIN/pg" --strip-components=1
  # The package ships the libraries without their SONAME links, so the binaries
  # cannot be started as unpacked.
  python3 - "$TOOLCHAIN/pg" <<'PY'
import json, os, sys
root = sys.argv[1]
links = json.load(open(os.path.join(root, "native/pg-symlinks.json")))
for link in links:
    src = os.path.join(root, link["source"])
    dst = os.path.join(root, link["target"])
    if not os.path.exists(src):
        continue
    if os.path.islink(dst) or os.path.exists(dst):
        os.remove(dst)
    os.symlink(os.path.basename(src), dst)
PY
fi

# ---------------------------------------------------------------------------
# Environment
# ---------------------------------------------------------------------------
cat > "$TOOLCHAIN/env.sh" <<EOF
export PATH=$TOOLCHAIN/go/bin:\$PATH
export LD_LIBRARY_PATH=$TOOLCHAIN/pg/native/lib\${LD_LIBRARY_PATH:+:\$LD_LIBRARY_PATH}
export PGBIN=$TOOLCHAIN/pg/native/bin
export PGDATA=/tmp/pgdata
export GOMODCACHE=/tmp/gomodcache
export GOFLAGS=-mod=mod
export GOPROXY=direct
export GOSUMDB=off
export GOPRIVATE='*'
export GONOSUMDB='*'
export GOTOOLCHAIN=local
EOF
# shellcheck disable=SC1090
source "$TOOLCHAIN/env.sh"

# ---------------------------------------------------------------------------
# Module graph
# ---------------------------------------------------------------------------
# golang.org/x/* live on go.googlesource.com, which the sandbox cannot reach.
# github.com/golang/* are the same repositories, so the alternate modfile
# substitutes them. Every other dependency is github-hosted and resolves
# directly. The real go.mod and go.sum are never touched.
if [ ! -f /tmp/local.mod ]; then
  echo "bootstrap: writing /tmp/local.mod (sandbox module substitutions)"
  cp "$ROOT/server/go.mod" /tmp/local.mod
  cat >> /tmp/local.mod <<'EOF'

// Sandbox-only: go.googlesource.com (golang.org/x/*) is unreachable from the
// dev sandbox while github.com is not, so the mirrored repositories are
// substituted. Never committed; CI resolves the real modules.
replace golang.org/x/crypto => github.com/golang/crypto v0.55.0

replace golang.org/x/net => github.com/golang/net v0.57.0

replace golang.org/x/sys => github.com/golang/sys v0.47.0

replace golang.org/x/term => github.com/golang/term v0.45.0

replace golang.org/x/text => github.com/golang/text v0.41.0
EOF
fi

# ---------------------------------------------------------------------------
# Database
# ---------------------------------------------------------------------------
if [ ! -s "${PGDATA}/PG_VERSION" ]; then
  echo "bootstrap: initialising PostgreSQL"
  rm -rf "$PGDATA"
  "$PGBIN/initdb" -D "$PGDATA" -U iconfess --auth-local=trust --auth-host=trust -E UTF8 >/tmp/pg-initdb.log 2>&1
fi
if ! "$PGBIN/pg_ctl" -D "$PGDATA" status >/dev/null 2>&1; then
  echo "bootstrap: starting PostgreSQL"
  "$PGBIN/pg_ctl" -D "$PGDATA" -l /tmp/pg.log \
    -o "-p 5432 -k /tmp -c listen_addresses=127.0.0.1" -w start >/dev/null
fi

echo
echo "ready. Next:"
echo "  source $TOOLCHAIN/env.sh"
echo "  cd $ROOT/server && TEST_DATABASE_URL=\"host=127.0.0.1 port=5432 user=iconfess dbname=postgres sslmode=disable\" \\"
echo "      go test -modfile=/tmp/local.mod -count=1 ./..."
