package deletion

import (
	"context"
	"fmt"

	"github.com/Teamthy/i-confess/internal/db"
)

// checkPolicySchema verifies every table and referenced column before an
// erasure executes its first destructive statement. This avoids trying to
// recover from PostgreSQL's aborted-transaction state after a stale policy
// meets an incomplete schema.
func checkPolicySchema(ctx context.Context, tx *db.Tx) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT table_name, column_name
		FROM information_schema.columns
		WHERE table_schema = current_schema()`)
	if err != nil {
		return fmt.Errorf("read current schema columns: %w", err)
	}
	defer rows.Close()

	columns := make(map[string]map[string]struct{})
	for rows.Next() {
		var table, column string
		if err := rows.Scan(&table, &column); err != nil {
			return fmt.Errorf("read current schema columns: %w", err)
		}
		if columns[table] == nil {
			columns[table] = make(map[string]struct{})
		}
		columns[table][column] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read current schema columns: %w", err)
	}

	for _, p := range Policies {
		tableColumns, exists := columns[p.Table]
		if !exists {
			return fmt.Errorf("policy table %q is absent from the current schema", p.Table)
		}
		if _, exists := tableColumns[p.column()]; !exists {
			return fmt.Errorf("policy column %q.%q is absent from the current schema", p.Table, p.column())
		}
	}
	return nil
}
