package mgogorm_test

import (
	"context"
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	mgogorm "github.com/mgo-framework/mgo/adapters/orm-gorm"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Post struct {
	ID    uint `gorm:"primarykey"`
	Title string
}

func open(t *testing.T) *mgogorm.DB {
	t.Helper()
	g, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	db := mgogorm.New(g)
	if err := db.AutoMigrator(&Post{}).Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return db
}

func count(t *testing.T, db *mgogorm.DB) int64 {
	t.Helper()
	var n int64
	if err := db.Model(&Post{}).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

func TestInTxCommits(t *testing.T) {
	db := open(t)
	err := db.InTx(context.Background(), func(ctx context.Context) error {
		return mgogorm.From(ctx, db).Create(&Post{Title: "hello"}).Error
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := count(t, db); n != 1 {
		t.Fatalf("count = %d, want 1", n)
	}
}

func TestInTxRollsBackOnError(t *testing.T) {
	db := open(t)
	boom := errors.New("boom")
	err := db.InTx(context.Background(), func(ctx context.Context) error {
		if err := mgogorm.From(ctx, db).Create(&Post{Title: "doomed"}).Error; err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if n := count(t, db); n != 0 {
		t.Fatalf("count = %d, want 0 after rollback", n)
	}
}

func TestInTxRollsBackOnPanic(t *testing.T) {
	db := open(t)
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("panic was swallowed")
			}
		}()
		db.InTx(context.Background(), func(ctx context.Context) error {
			if err := mgogorm.From(ctx, db).Create(&Post{Title: "doomed"}).Error; err != nil {
				return err
			}
			panic("kaboom")
		})
	}()
	if n := count(t, db); n != 0 {
		t.Fatalf("count = %d, want 0 after panic rollback", n)
	}
}

func TestNestedInTxJoinsAndRollsBackTogether(t *testing.T) {
	db := open(t)
	boom := errors.New("boom")
	err := db.InTx(context.Background(), func(ctx context.Context) error {
		if err := mgogorm.From(ctx, db).Create(&Post{Title: "outer"}).Error; err != nil {
			return err
		}
		return db.InTx(ctx, func(ctx context.Context) error {
			if err := mgogorm.From(ctx, db).Create(&Post{Title: "inner"}).Error; err != nil {
				return err
			}
			return boom // fails the joined transaction as a whole
		})
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if n := count(t, db); n != 0 {
		t.Fatalf("count = %d, want 0 — joined tx must roll back everything", n)
	}
}

func TestFromOutsideTxUsesBaseHandle(t *testing.T) {
	db := open(t)
	if err := mgogorm.From(context.Background(), db).Create(&Post{Title: "direct"}).Error; err != nil {
		t.Fatal(err)
	}
	if n := count(t, db); n != 1 {
		t.Fatalf("count = %d, want 1", n)
	}
}

func TestHealth(t *testing.T) {
	db := open(t)
	if err := db.Health(context.Background()); err != nil {
		t.Fatal(err)
	}
}
