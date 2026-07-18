package dbsql_test

// The fake driver below keeps this module dependency-free: transaction
// semantics (commit/rollback/join/panic) are observable without a real
// database engine.

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"sync/atomic"
	"testing"

	dbsql "github.com/mgo-framework/mgo/adapters/db-sql"
)

// ---- minimal database/sql/driver stub ----

type fakeDriver struct{ state *txState }

type txState struct {
	begins    atomic.Int32
	commits   atomic.Int32
	rollbacks atomic.Int32
	execs     atomic.Int32
}

type fakeConn struct{ state *txState }
type fakeTx struct{ state *txState }
type fakeStmt struct{ state *txState }
type fakeResult struct{}

func (d *fakeDriver) Open(string) (driver.Conn, error) { return &fakeConn{d.state}, nil }

func (c *fakeConn) Prepare(string) (driver.Stmt, error) { return &fakeStmt{c.state}, nil }
func (c *fakeConn) Close() error                        { return nil }
func (c *fakeConn) Begin() (driver.Tx, error) {
	c.state.begins.Add(1)
	return &fakeTx{c.state}, nil
}

func (t *fakeTx) Commit() error   { t.state.commits.Add(1); return nil }
func (t *fakeTx) Rollback() error { t.state.rollbacks.Add(1); return nil }

func (s *fakeStmt) Close() error  { return nil }
func (s *fakeStmt) NumInput() int { return -1 }
func (s *fakeStmt) Exec([]driver.Value) (driver.Result, error) {
	s.state.execs.Add(1)
	return fakeResult{}, nil
}
func (s *fakeStmt) Query([]driver.Value) (driver.Rows, error) {
	return nil, errors.New("not implemented")
}

func (fakeResult) LastInsertId() (int64, error) { return 0, nil }
func (fakeResult) RowsAffected() (int64, error) { return 1, nil }

func open(t *testing.T) (*dbsql.DB, *txState) {
	t.Helper()
	state := &txState{}
	name := "fake_" + t.Name()
	sql.Register(name, &fakeDriver{state})
	raw, err := sql.Open(name, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { raw.Close() })
	return dbsql.New(raw), state
}

// ---- tests ----

func TestInTxCommitsOnNil(t *testing.T) {
	db, state := open(t)
	err := db.InTx(context.Background(), func(ctx context.Context) error {
		q := dbsql.From(ctx, db)
		if _, ok := q.(*sql.Tx); !ok {
			t.Fatalf("From inside InTx = %T, want *sql.Tx", q)
		}
		_, err := q.ExecContext(ctx, "INSERT")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if state.commits.Load() != 1 || state.rollbacks.Load() != 0 {
		t.Fatalf("commits=%d rollbacks=%d, want 1/0", state.commits.Load(), state.rollbacks.Load())
	}
}

func TestInTxRollsBackOnError(t *testing.T) {
	db, state := open(t)
	boom := errors.New("boom")
	err := db.InTx(context.Background(), func(ctx context.Context) error { return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if state.commits.Load() != 0 || state.rollbacks.Load() != 1 {
		t.Fatalf("commits=%d rollbacks=%d, want 0/1", state.commits.Load(), state.rollbacks.Load())
	}
}

func TestInTxRollsBackAndRepanicsOnPanic(t *testing.T) {
	db, state := open(t)
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("panic was swallowed")
			}
		}()
		db.InTx(context.Background(), func(ctx context.Context) error { panic("kaboom") })
	}()
	if state.rollbacks.Load() != 1 {
		t.Fatalf("rollbacks=%d, want 1", state.rollbacks.Load())
	}
}

func TestNestedInTxJoins(t *testing.T) {
	db, state := open(t)
	err := db.InTx(context.Background(), func(ctx context.Context) error {
		outer := dbsql.From(ctx, db)
		return db.InTx(ctx, func(ctx context.Context) error {
			if inner := dbsql.From(ctx, db); inner != outer {
				t.Fatal("nested InTx must join the enclosing transaction")
			}
			return nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	if state.begins.Load() != 1 || state.commits.Load() != 1 {
		t.Fatalf("begins=%d commits=%d, want 1/1", state.begins.Load(), state.commits.Load())
	}
}

func TestFromOutsideTxReturnsDB(t *testing.T) {
	db, _ := open(t)
	if q := dbsql.From(context.Background(), db); q != db.DB {
		t.Fatalf("From outside tx = %T, want the *sql.DB", q)
	}
}

func TestHealth(t *testing.T) {
	db, _ := open(t)
	if err := db.Health(context.Background()); err != nil {
		t.Fatal(err)
	}
}
