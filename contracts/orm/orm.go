// Package orm defines MGO's persistence integration points — and nothing
// more. MGO does not wrap query APIs: day-to-day data access uses the
// chosen library natively (GORM, ent, sqlc, database/sql). These contracts
// capture only what glue needs from any of them: transaction propagation
// through context, health reporting, and a migration-runner hook.
//
// The transaction pattern: services call Transactor.InTx; repositories
// retrieve the native handle (*sql.Tx, *gorm.DB, *ent.Tx, ...) with the
// adapter's own From(ctx) helper. Deleting MGO leaves working code on the
// underlying library.
package orm

import "context"

// Transactor runs fn inside a database transaction.
//
// Semantics required of implementations:
//
//   - The transaction travels in the context passed to fn, under an
//     adapter-private key; the adapter's From(ctx) helper returns it.
//   - A nested InTx call whose ctx already carries a transaction joins it
//     (no savepoints at the contract level).
//   - fn returning an error — or panicking — rolls back; the panic is
//     re-raised after rollback. Returning nil commits.
type Transactor interface {
	InTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// HealthChecker reports backend connectivity. Adapters implement it so
// health endpoints and readiness probes can aggregate every store without
// knowing what it is.
type HealthChecker interface {
	Health(ctx context.Context) error
}

// Migrator is the migration-runner hook: implementations wrap an engine
// (golang-migrate, goose, atlas, GORM AutoMigrate, ...) so boot-time
// migration and the future `mgo migrate` command can drive any of them.
type Migrator interface {
	Migrate(ctx context.Context) error
}
