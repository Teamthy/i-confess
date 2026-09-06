package db

import (
	"context"
	"database/sql"
)

// DB is a *sql.DB that rebinds `?` placeholders to PostgreSQL's `$N` form on
// every query.
//
// It exists so the store layer keeps the SQL it was written with. Every method
// here has the same name and signature as its database/sql counterpart, so
// swapping a store's field from *sql.DB to *db.DB changes no call site — the
// rebinding is invisible to the code that issues queries, which is the point.
// A choke point that callers had to remember to use would not be a choke point.
//
// Methods that do not carry SQL (Ping, Close, SetMaxOpenConns, Stats) are
// promoted from the embedded *sql.DB unchanged.
type DB struct {
	*sql.DB
}

// NewDB wraps an existing connection pool.
func NewDB(sqlDB *sql.DB) *DB { return &DB{sqlDB} }

func (d *DB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return d.DB.ExecContext(ctx, Rebind(query), args...)
}

func (d *DB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return d.DB.QueryContext(ctx, Rebind(query), args...)
}

func (d *DB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return d.DB.QueryRowContext(ctx, Rebind(query), args...)
}

func (d *DB) Exec(query string, args ...any) (sql.Result, error) {
	return d.DB.Exec(Rebind(query), args...)
}

func (d *DB) Query(query string, args ...any) (*sql.Rows, error) {
	return d.DB.Query(Rebind(query), args...)
}

func (d *DB) QueryRow(query string, args ...any) *sql.Row {
	return d.DB.QueryRow(Rebind(query), args...)
}

func (d *DB) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return d.DB.PrepareContext(ctx, Rebind(query))
}

func (d *DB) Prepare(query string) (*sql.Stmt, error) {
	return d.DB.Prepare(Rebind(query))
}

// BeginTx returns a Tx that rebinds too. Without this a transaction would
// silently run unrebound SQL and fail on its first parameter, which is easy to
// miss because the non-transactional paths all work.
func (d *DB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*Tx, error) {
	tx, err := d.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &Tx{tx}, nil
}

func (d *DB) Begin() (*Tx, error) {
	tx, err := d.DB.Begin()
	if err != nil {
		return nil, err
	}
	return &Tx{tx}, nil
}

// Tx is the transaction counterpart of DB. Commit and Rollback are promoted
// from the embedded *sql.Tx and need no rebinding.
type Tx struct {
	*sql.Tx
}

func (t *Tx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return t.Tx.ExecContext(ctx, Rebind(query), args...)
}

func (t *Tx) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return t.Tx.QueryContext(ctx, Rebind(query), args...)
}

func (t *Tx) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return t.Tx.QueryRowContext(ctx, Rebind(query), args...)
}

func (t *Tx) Exec(query string, args ...any) (sql.Result, error) {
	return t.Tx.Exec(Rebind(query), args...)
}

func (t *Tx) Query(query string, args ...any) (*sql.Rows, error) {
	return t.Tx.Query(Rebind(query), args...)
}

func (t *Tx) QueryRow(query string, args ...any) *sql.Row {
	return t.Tx.QueryRow(Rebind(query), args...)
}

func (t *Tx) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return t.Tx.PrepareContext(ctx, Rebind(query))
}

func (t *Tx) Prepare(query string) (*sql.Stmt, error) {
	return t.Tx.Prepare(Rebind(query))
}
