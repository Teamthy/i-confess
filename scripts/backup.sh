#!/usr/bin/env bash
# iCONFESS Database & Media Backup Script
# Production Runbook Reference: docs/DISASTER-RECOVERY.md
# RPO Target: < 1 hour | RTO Target: < 30 minutes

set -euo pipefail

BACKUP_DIR="${BACKUP_DIR:-/var/backups/iconfess}"
DATABASE_URL="${DATABASE_URL:-postgres://iconfess:iconfess@127.0.0.1:5432/iconfess?sslmode=disable}"
MEDIA_DIR="${MEDIA_DIR:-/app/data/media}"
TIMESTAMP=$(date -u +"%Y%m%d_%H%M%SZ")
RETENTION_DAYS="${RETENTION_DAYS:-30}"

mkdir -p "${BACKUP_DIR}"

echo "==> [${TIMESTAMP}] Starting iCONFESS Production Backup"

# 1. Database Dump (Custom binary format with compression)
DB_BACKUP_FILE="${BACKUP_DIR}/iconfess_db_${TIMESTAMP}.dump"
echo "--> Exporting PostgreSQL database..."
if command -v pg_dump >/dev/null 2>&1; then
    pg_dump -d "${DATABASE_URL}" -F c -b -v -f "${DB_BACKUP_FILE}"
    sha256sum "${DB_BACKUP_FILE}" > "${DB_BACKUP_FILE}.sha256"
    echo "--> DB Backup created: ${DB_BACKUP_FILE} ($(du -h "${DB_BACKUP_FILE}" | cut -f1))"
else
    echo "WARNING: pg_dump not found in PATH. Skipping DB dump."
fi

# 2. Media Metadata & Audio Manifest Backup (if local directory exists)
if [ -d "${MEDIA_DIR}" ]; then
    MEDIA_BACKUP_FILE="${BACKUP_DIR}/iconfess_media_${TIMESTAMP}.tar.gz"
    echo "--> Packaging local media directory..."
    tar -czf "${MEDIA_BACKUP_FILE}" -C "$(dirname "${MEDIA_DIR}")" "$(basename "${MEDIA_DIR}")"
    sha256sum "${MEDIA_BACKUP_FILE}" > "${MEDIA_BACKUP_FILE}.sha256"
    echo "--> Media Backup created: ${MEDIA_BACKUP_FILE}"
fi

# 3. Retention Cleanup (Prune backups older than RETENTION_DAYS)
echo "--> Cleaning up backups older than ${RETENTION_DAYS} days..."
find "${BACKUP_DIR}" -type f \( -name "*.dump" -o -name "*.tar.gz" -o -name "*.sha256" \) -mtime "+${RETENTION_DAYS}" -delete

echo "==> Backup completed successfully at $(date -u +"%Y%m%d_%H%M%SZ")"
