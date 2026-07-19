// Package mgoent adapts a generated ent client to MGO's orm contract.
//
// ent generates a typed client per project, so this adapter cannot import
// it — instead it is generic over three functions every generated package
// already provides. Wiring is three lines, all of them ent's own API:
//
//	tx := mgoent.New(
//	    client.Tx,        // begin
//	    ent.NewTxContext, // carry the tx in ctx (ent's generated helper)
//	    func(ctx context.Context) (*ent.Tx, bool) {
//	        t := ent.TxFromContext(ctx)
//	        return t, t != nil
//	    },
//	)
//
// Repositories keep using ent natively; inside InTx they pick the
// transactional client with ent.TxFromContext(ctx). The adapter module
// depends only on contracts — no ent import, nothing to version-match.
package mgoent

import (
	"context"

	"github.com/mgo-framework/mgo/contracts/orm"
)

// Tx is the subset of a generated *ent.Tx the adapter needs.
type Tx interface {
	Commit() error
	Rollback() error
}

// Transactor implements orm.Transactor over a generated ent client.
type Transactor[T Tx] struct {
	begin func(ctx context.Context) (T, error)
	wrap  func(ctx context.Context, tx T) context.Context
	from  func(ctx context.Context) (T, bool)
}

var _ orm.Transactor = (*Transactor[Tx])(nil)

// New builds a Transactor from a generated client's Tx method and the
// generated NewTxContext/TxFromContext helpers.
func New[T Tx](
	begin func(ctx context.Context) (T, error),
	wrap func(ctx context.Context, tx T) context.Context,
	from func(ctx context.Context) (T, bool),
) *Transactor[T] {
	return &Transactor[T]{begin: begin, wrap: wrap, from: from}
}

// InTx implements orm.Transactor: begin, run fn with the tx in ctx,
// commit on nil, roll back on error or panic (re-raised). A ctx already
// carrying a transaction joins it.
func (t *Transactor[T]) InTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := t.from(ctx); ok {
		return fn(ctx) // join the enclosing transaction
	}
	tx, err := t.begin(ctx)
	if err != nil {
		return err
	}
	done := false
	defer func() {
		if !done {
			tx.Rollback() // error or panic path; commit never ran
		}
	}()
	if err := fn(t.wrap(ctx, tx)); err != nil {
		return err // deferred rollback fires
	}
	done = true
	return tx.Commit()
}
