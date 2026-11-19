package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// checkRuntimeRole catches a migration/owner credential accidentally supplied
// to a verify-mode server. Check the actual ledger's schema, and memberships
// as well as inherited privileges (NOINHERIT alone does not prevent SET ROLE).
// This is a startup guard, not a replacement for reviewing production ACLs.
func checkRuntimeRole(ctx context.Context, conn *sql.DB) error {
	var privileged bool
	err := conn.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_catalog.pg_roles r, pg_catalog.pg_class ledger
			WHERE ledger.oid = 'schema_migrations'::regclass
			  AND pg_has_role(session_user, r.oid, 'MEMBER')
			  AND (r.rolsuper OR r.rolcreatedb OR r.rolcreaterole OR r.rolbypassrls OR r.rolreplication
			    OR has_database_privilege(r.oid, current_database(), 'CREATE')
			    OR EXISTS (SELECT 1 FROM pg_catalog.pg_namespace n
			               WHERE (n.oid = ledger.relnamespace OR n.nspname = ANY(current_schemas(false)))
			                 AND (n.nspowner = r.oid OR has_schema_privilege(r.oid, n.oid, 'CREATE')))
			    OR has_table_privilege(r.oid, ledger.oid, 'INSERT, UPDATE, DELETE, TRUNCATE, REFERENCES, TRIGGER')
			    OR has_any_column_privilege(r.oid, ledger.oid, 'INSERT, UPDATE, REFERENCES')
			    OR EXISTS (SELECT 1 FROM pg_catalog.pg_class c
			               WHERE c.relnamespace = ledger.relnamespace AND c.relowner = r.oid)
			    OR EXISTS (SELECT 1 FROM pg_catalog.pg_proc p
			               WHERE p.pronamespace = ledger.relnamespace AND p.proowner = r.oid))
		)`).Scan(&privileged)
	if err != nil {
		return fmt.Errorf("check runtime database role: %w", err)
	}
	if privileged {
		return errors.New("DB_SCHEMA_MODE=verify requires a restricted runtime role: check ownership, role memberships, schema CREATE and migration ledger grants")
	}
	return nil
}
