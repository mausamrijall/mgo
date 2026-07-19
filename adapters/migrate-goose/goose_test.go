package mgogoose_test

import (
	"context"
	"database/sql"
	"embed"
	"io/fs"
	"testing"

	mgogoose "github.com/mgo-framework/mgo/adapters/migrate-goose"
	_ "modernc.org/sqlite" // database/sql driver "sqlite"
)

//go:embed testdata/migrations/*.sql
var migrations embed.FS

func TestMigrateAppliesAllPendingAndIsIdempotent(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	sub, err := fs.Sub(migrations, "testdata/migrations")
	if err != nil {
		t.Fatal(err)
	}
	m, err := mgogoose.New(db, mgogoose.DialectSQLite3, sub)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if err := m.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	// Both migrations applied: posts table exists with the body column.
	if _, err := db.ExecContext(ctx, `INSERT INTO posts (title, body) VALUES ('t', 'b')`); err != nil {
		t.Fatalf("schema incomplete after Migrate: %v", err)
	}
	// Second run is a no-op, not an error.
	if err := m.Migrate(ctx); err != nil {
		t.Fatalf("Migrate is not idempotent: %v", err)
	}

	// Native goose API stays reachable through the embedded Provider.
	version, err := m.GetDBVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if version != 2 {
		t.Fatalf("db version = %d, want 2", version)
	}
}
