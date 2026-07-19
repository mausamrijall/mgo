package mgosqlc_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"

	dbsql "github.com/mgo-framework/mgo/adapters/db-sql"
	mgosqlc "github.com/mgo-framework/mgo/adapters/orm-sqlc"
)

// queries mimics a sqlc-generated Queries type: New(db) + WithTx(tx).
type queries struct {
	db any // *sql.DB or *sql.Tx, like sqlc's DBTX field
}

func (q *queries) WithTx(tx *sql.Tx) *queries { return &queries{db: tx} }

// ---- minimal driver so InTx can produce a real *sql.Tx ----

type nopDriver struct{}
type nopConn struct{}
type nopTx struct{}

func (nopDriver) Open(string) (driver.Conn, error)  { return nopConn{}, nil }
func (nopConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("no stmts") }
func (nopConn) Close() error                        { return nil }
func (nopConn) Begin() (driver.Tx, error)           { return nopTx{}, nil }
func (nopTx) Commit() error                         { return nil }
func (nopTx) Rollback() error                       { return nil }

func TestFromRebindsInsideTxOnly(t *testing.T) {
	sql.Register("nop", nopDriver{})
	raw, err := sql.Open("nop", "")
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	db := dbsql.New(raw)
	q := &queries{db: raw}

	// Outside a transaction: unchanged.
	if got := mgosqlc.From(context.Background(), q); got != q {
		t.Fatal("From outside InTx must return q unchanged")
	}

	// Inside: rebound to the *sql.Tx carried by ctx.
	err = db.InTx(context.Background(), func(ctx context.Context) error {
		got := mgosqlc.From(ctx, q)
		if got == q {
			t.Fatal("From inside InTx must rebind")
		}
		tx, ok := got.db.(*sql.Tx)
		if !ok || tx == nil {
			t.Fatalf("rebound to %T, want *sql.Tx", got.db)
		}
		wantTx, _ := dbsql.TxFromContext(ctx)
		if tx != wantTx {
			t.Fatal("rebound to a different tx than the ctx one")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
