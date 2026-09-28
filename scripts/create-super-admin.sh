#!/usr/bin/env bash
# Create a super admin user for local development.
# Usage: ./scripts/create-super-admin.sh [email] [password]
# Requires: psql with DATABASE_URL or TEST_DATABASE_URL, or running Go server with admin endpoint.

set -euo pipefail

EMAIL="${1:-superadmin@iconfess.local}"
PASSWORD="${2:-SuperAdmin123!}"
DB_URL="${DATABASE_URL:-${TEST_DATABASE_URL:-}}"

if [[ -z "$DB_URL" ]]; then
  echo "Set DATABASE_URL or TEST_DATABASE_URL to your Postgres connection string"
  echo "Example: DATABASE_URL='host=127.0.0.1 port=5432 user=iconfess password=iconfess dbname=iconfess sslmode=disable' $0"
  exit 1
fi

echo "Creating super admin: $EMAIL"

# Hash password using Go helper if available, else use placeholder that will be re-hashed on login? 
# We will insert via SQL using the same bcrypt that Go uses - we need to generate hash.
# For simplicity, we will use a small Go snippet if go is available, otherwise insert and rely on auth to hash on next step.

if command -v go >/dev/null 2>&1; then
  HASH=$(cd server && go run -exec "echo" ./cmd/hash-password 2>/dev/null || echo "")
  # Fallback: use python bcrypt
  if [[ -z "$HASH" ]]; then
    HASH=$(python3 -c "import bcrypt, sys; print(bcrypt.hashpw(sys.argv[1].encode(), bcrypt.gensalt()).decode())" "$PASSWORD" 2>/dev/null || echo "")
  fi
else
  HASH=$(python3 -c "import bcrypt, sys; print(bcrypt.hashpw(sys.argv[1].encode(), bcrypt.gensalt()).decode())" "$PASSWORD" 2>/dev/null || echo "")
fi

if [[ -z "$HASH" ]]; then
  echo "Could not generate bcrypt hash. Install python3 bcrypt or go."
  echo "You can also register via API and then promote:"
  echo "  curl -X POST http://localhost:8080/auth/register -d '{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\",\"display_name\":\"Super Admin\"}' -H 'Content-Type: application/json'"
  echo "  Then: psql \$DATABASE_URL -c \"INSERT INTO admin_users (id, user_id, role, created_at) SELECT gen_random_uuid(), id, 'super_admin', now() FROM users WHERE email='$EMAIL' ON CONFLICT(user_id) DO UPDATE SET role='super_admin';\""
  exit 1
fi

USER_ID=$(uuidgen 2>/dev/null || cat /proc/sys/kernel/random/uuid 2>/dev/null || python3 -c "import uuid; print(uuid.uuid4())")
NOW=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

psql "$DB_URL" <<SQL
INSERT INTO users (id, email, password_hash, display_name, timezone, status, email_verified, created_at, updated_at)
VALUES ('$USER_ID', '$EMAIL', '$HASH', 'Super Admin', 'UTC', 'active', 1, '$NOW', '$NOW')
ON CONFLICT(email) DO UPDATE SET password_hash=EXCLUDED.password_hash, status='active', email_verified=1, updated_at=EXCLUDED.updated_at
RETURNING id;

INSERT INTO admin_users (id, user_id, role, created_at)
SELECT gen_random_uuid(), id, 'super_admin', '$NOW' FROM users WHERE email='$EMAIL'
ON CONFLICT(user_id) DO UPDATE SET role='super_admin';

-- Also insert into rbac_user_roles if table exists
INSERT INTO rbac_user_roles (id, user_id, role_id, created_at)
SELECT gen_random_uuid(), u.id, r.id, '$NOW' FROM users u, rbac_roles r WHERE u.email='$EMAIL' AND r.name='super_admin'
ON CONFLICT(user_id, role_id) DO NOTHING;
SQL

echo "Super admin $EMAIL ready. Password: $PASSWORD"
echo "Login: POST /auth/login {\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}"
