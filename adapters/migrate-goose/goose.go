// Package mgogoose adapts pressly/goose to MGO's orm.Migrator hook, so
// boot-time migration and the future `mgo migrate` command can drive
// goose like any other engine. Migrations stay plain goose SQL files —
// embed them and go:
//
//	//go:embed migrations/*.sql
//	var migrations embed.FS
//
//	sub, _ := fs.Sub(migrations, "migrations")
//	m, err := mgogoose.New(sqlDB, mgogoose.DialectSQLite3, sub)
//	err = m.Migrate(ctx)
package mgogoose

import (
	"context"
	"database/sql"
	"io/fs"

	"github.com/mgo-framework/mgo/contracts/orm"
	"github.com/pressly/goose/v3"
)

// Dialects re-exported so callers don't import goose for one constant.
const (
	DialectPostgres = goose.DialectPostgres
	DialectMySQL    = goose.DialectMySQL
	DialectSQLite3  = goose.DialectSQLite3
)

// Migrator runs goose migrations up. It also exposes the underlying
// goose.Provider for status/down/redo — native API stays reachable.
type Migrator struct {
	*goose.Provider
}

var _ orm.Migrator = (*Migrator)(nil)

// New builds a Migrator over db from the migration files in fsys (rooted
// at the directory containing the .sql files — use fs.Sub on an embed.FS).
func New(db *sql.DB, dialect goose.Dialect, fsys fs.FS) (*Migrator, error) {
	p, err := goose.NewProvider(dialect, db, fsys)
	if err != nil {
		return nil, err
	}
	return &Migrator{Provider: p}, nil
}

// Migrate implements orm.Migrator: apply all pending up migrations.
func (m *Migrator) Migrate(ctx context.Context) error {
	_, err := m.Up(ctx)
	return err
}
