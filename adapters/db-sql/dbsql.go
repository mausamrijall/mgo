// Package dbsql adapts database/sql to MGO's orm contract. The *sql.DB is
// embedded — its native API is the API. The adapter adds only transaction
// propagation (InTx + From) and health, per the glue philosophy.
//
//	db := dbsql.New(sqlDB)
//	err := db.InTx(ctx, func(ctx context.Context) error {
//	    q := dbsql.From(ctx, db)           // *sql.Tx inside InTx, *sql.DB outside
//	    _, err := q.ExecContext(ctx, "INSERT ...")
//	    return err
//	})
package dbsql

import (
	"context"
	"database/sql"

	"github.com/mgo-framework/mgo/contracts/orm"
)

// Querier is the query surface shared by *sql.DB and *sql.Tx — write
// repositories against it and they work inside and outside transactions.
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)
}

var (
	_ Querier = (*sql.DB)(nil)
	_ Querier = (*sql.Tx)(nil)
)

// DB wraps *sql.DB with the orm contract. The embedded DB keeps the full
// stdlib API available.
type DB struct {
	*sql.DB
}

var (
	_ orm.Transactor    = (*DB)(nil)
	_ orm.HealthChecker = (*DB)(nil)
)

// New wraps an opened *sql.DB.
func New(db *sql.DB) *DB { return &DB{DB: db} }

type ctxKey struct{}

// From returns the transaction carried by ctx (inside InTx) or db itself.
func From(ctx context.Context, db *DB) Querier {
	if tx, ok := ctx.Value(ctxKey{}).(*sql.Tx); ok {
		return tx
	}
	return db.DB
}

// InTx implements orm.Transactor: begin, run fn with the tx in ctx,
// commit on nil, roll back on error or panic (re-raised). A ctx already
// carrying a transaction joins it.
func (d *DB) InTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := ctx.Value(ctxKey{}).(*sql.Tx); ok {
		return fn(ctx) // join the enclosing transaction
	}
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	done := false
	defer func() {
		if !done {
			tx.Rollback() // error or panic path; commit never ran
		}
	}()
	if err := fn(context.WithValue(ctx, ctxKey{}, tx)); err != nil {
		return err // deferred rollback fires
	}
	done = true
	return tx.Commit()
}

// Health implements orm.HealthChecker.
func (d *DB) Health(ctx context.Context) error { return d.PingContext(ctx) }
