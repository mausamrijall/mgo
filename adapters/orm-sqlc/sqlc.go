// Package mgosqlc glues sqlc-generated code into MGO's transaction flow.
// sqlc's runtime IS database/sql, so lifecycle, health and InTx come from
// adapters/db-sql; the only missing piece is rebinding a generated
// *Queries to the transaction travelling in ctx — one generic function:
//
//	db := dbsql.New(sqlDB)          // orm.Transactor + orm.HealthChecker
//	q := sqlcgen.New(sqlDB)         // sqlc's generated constructor
//
//	err := db.InTx(ctx, func(ctx context.Context) error {
//	    return mgosqlc.From(ctx, q).CreatePost(ctx, "title") // runs on the tx
//	})
//
// Outside InTx, From returns q unchanged. Deleting MGO leaves ordinary
// sqlc + database/sql code.
package mgosqlc

import (
	"context"
	"database/sql"

	dbsql "github.com/mgo-framework/mgo/adapters/db-sql"
)

// WithTxer is satisfied by every sqlc-generated *Queries type.
type WithTxer[Q any] interface {
	WithTx(tx *sql.Tx) Q
}

// From returns q bound to the transaction carried by ctx (inside
// dbsql.InTx), or q unchanged.
func From[Q WithTxer[Q]](ctx context.Context, q Q) Q {
	if tx, ok := dbsql.TxFromContext(ctx); ok {
		return q.WithTx(tx)
	}
	return q
}
