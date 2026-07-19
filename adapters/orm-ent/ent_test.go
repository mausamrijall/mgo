package mgoent_test

// The adapter is generic over plain functions, so its transaction
// semantics are fully testable with a fake tx — no ent, no database.

import (
	"context"
	"errors"
	"testing"

	mgoent "github.com/mgo-framework/mgo/adapters/orm-ent"
)

type fakeTx struct {
	committed  int
	rolledBack int
}

func (t *fakeTx) Commit() error   { t.committed++; return nil }
func (t *fakeTx) Rollback() error { t.rolledBack++; return nil }

type ctxKey struct{}

func harness() (*mgoent.Transactor[*fakeTx], *[]*fakeTx) {
	var began []*fakeTx
	tr := mgoent.New(
		func(ctx context.Context) (*fakeTx, error) {
			tx := &fakeTx{}
			began = append(began, tx)
			return tx, nil
		},
		func(ctx context.Context, tx *fakeTx) context.Context {
			return context.WithValue(ctx, ctxKey{}, tx)
		},
		func(ctx context.Context) (*fakeTx, bool) {
			tx, ok := ctx.Value(ctxKey{}).(*fakeTx)
			return tx, ok
		},
	)
	return tr, &began
}

func TestInTxCommits(t *testing.T) {
	tr, began := harness()
	err := tr.InTx(context.Background(), func(ctx context.Context) error {
		if _, ok := ctx.Value(ctxKey{}).(*fakeTx); !ok {
			t.Fatal("tx not in ctx inside InTx")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	tx := (*began)[0]
	if tx.committed != 1 || tx.rolledBack != 0 {
		t.Fatalf("committed=%d rolledBack=%d, want 1/0", tx.committed, tx.rolledBack)
	}
}

func TestInTxRollsBackOnError(t *testing.T) {
	tr, began := harness()
	boom := errors.New("boom")
	if err := tr.InTx(context.Background(), func(context.Context) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	tx := (*began)[0]
	if tx.committed != 0 || tx.rolledBack != 1 {
		t.Fatalf("committed=%d rolledBack=%d, want 0/1", tx.committed, tx.rolledBack)
	}
}

func TestInTxRollsBackAndRepanicsOnPanic(t *testing.T) {
	tr, began := harness()
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("panic was swallowed")
			}
		}()
		tr.InTx(context.Background(), func(context.Context) error { panic("kaboom") })
	}()
	if tx := (*began)[0]; tx.rolledBack != 1 {
		t.Fatalf("rolledBack=%d, want 1", tx.rolledBack)
	}
}

func TestNestedInTxJoins(t *testing.T) {
	tr, began := harness()
	err := tr.InTx(context.Background(), func(ctx context.Context) error {
		return tr.InTx(ctx, func(context.Context) error { return nil })
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(*began) != 1 {
		t.Fatalf("began %d transactions, want 1 (nested joins)", len(*began))
	}
	if tx := (*began)[0]; tx.committed != 1 {
		t.Fatalf("committed=%d, want 1", tx.committed)
	}
}
