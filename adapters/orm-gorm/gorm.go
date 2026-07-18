// Package mgogorm adapts GORM to MGO's orm contract. The *gorm.DB is
// embedded — GORM's native API is the API. The adapter adds transaction
// propagation (InTx + From), health, and an AutoMigrate hook; nothing
// else. Deleting MGO leaves ordinary GORM code.
//
//	db := mgogorm.New(gormDB)
//	err := db.InTx(ctx, func(ctx context.Context) error {
//	    return mgogorm.From(ctx, db).Create(&post).Error   // tx inside InTx
//	})
package mgogorm

import (
	"context"

	"github.com/mgo-framework/mgo/contracts/orm"
	"gorm.io/gorm"
)

// DB wraps *gorm.DB with the orm contract.
type DB struct {
	*gorm.DB
}

var (
	_ orm.Transactor    = (*DB)(nil)
	_ orm.HealthChecker = (*DB)(nil)
)

// New wraps an opened *gorm.DB.
func New(db *gorm.DB) *DB { return &DB{DB: db} }

type ctxKey struct{}

// From returns the transaction handle carried by ctx (inside InTx), or
// the base handle bound to ctx. Either way the result is a *gorm.DB —
// repositories don't care which.
func From(ctx context.Context, db *DB) *gorm.DB {
	if tx, ok := ctx.Value(ctxKey{}).(*gorm.DB); ok {
		return tx
	}
	return db.WithContext(ctx)
}

// InTx implements orm.Transactor via gorm's Transaction: commit on nil,
// rollback on error or panic (GORM re-raises panics after rollback). A
// ctx already carrying a transaction joins it.
func (d *DB) InTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := ctx.Value(ctxKey{}).(*gorm.DB); ok {
		return fn(ctx) // join the enclosing transaction
	}
	return d.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(context.WithValue(ctx, ctxKey{}, tx))
	})
}

// Health implements orm.HealthChecker by pinging the underlying sql.DB.
func (d *DB) Health(ctx context.Context) error {
	sqlDB, err := d.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

// AutoMigrator adapts GORM's AutoMigrate to the orm.Migrator hook. (Named
// to avoid shadowing gorm.DB's own Migrator method, which stays reachable.)
func (d *DB) AutoMigrator(models ...any) orm.Migrator {
	return migrator{db: d, models: models}
}

type migrator struct {
	db     *DB
	models []any
}

func (m migrator) Migrate(ctx context.Context) error {
	return m.db.WithContext(ctx).AutoMigrate(m.models...)
}
