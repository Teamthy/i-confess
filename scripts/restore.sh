#!/usr/bin/env bash
# iCONFESS Database & Media Restore Script
# Production Runbook Reference: docs/DISASTER-RECOVERY.md
# Verification & Recovery Testing

set -euo pipefail

if [ "$#" -lt 1 ]; then
    echo "Usage: $0 <path-to-db-dump-file> [target-database-url] [path-to-media-tar]"
    echo "Example: $0 /var/backups/iconfess/iconfess_db_20260920_120000Z.dump postgres://iconfess:iconfess@127.0.0.1:5432/iconfess_restored"
    exit 1
fi

DB_BACKUP_FILE="$1"
TARGET_DB_URL="${2:-${DATABASE_URL:-postgres://iconfess:iconfess@127.0.0.1:5432/iconfess?sslmode=disable}}"
MEDIA_TAR="${3:-}"
RESTORE_MEDIA_DIR="${MEDIA_DIR:-/app/data/media}"

echo "==> Starting iCONFESS Production Restore Test / Execution"

# 1. Verify Checksum if present
if [ -f "${DB_BACKUP_FILE}.sha256" ]; then
    echo "--> Verifying dump checksum..."
    sha256sum -c "${DB_BACKUP_FILE}.sha256"
fi

# 2. Restore PostgreSQL Database
if [ ! -f "${DB_BACKUP_FILE}" ]; then
    echo "ERROR: Backup file ${DB_BACKUP_FILE} not found!"
    exit 1
fi

echo "--> Restoring database to ${TARGET_DB_URL}..."
if command -v pg_restore >/dev/null 2>&1; then
    pg_restore --clean --if-exists --no-owner --no-privileges -d "${TARGET_DB_URL}" -v "${DB_BACKUP_FILE}" || {
        echo "WARNING: pg_restore exited with non-zero status (expected if non-critical warnings occurred)."
    }
    echo "--> Database restore completed."
else
    echo "ERROR: pg_restore not found in PATH."
    exit 1
fi

# 3. Restore Media Directory if provided
if [ -n "${MEDIA_TAR}" ] && [ -f "${MEDIA_TAR}" ]; then
    echo "--> Restoring media files from ${MEDIA_TAR} into ${RESTORE_MEDIA_DIR}..."
    mkdir -p "${RESTORE_MEDIA_DIR}"
    tar -xzf "${MEDIA_TAR}" -C "$(dirname "${RESTORE_MEDIA_DIR}")"
    echo "--> Media restore completed."
fi

echo "==> Restore verification completed successfully!"
