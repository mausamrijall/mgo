package main

// The two stores. Each repository uses its library's NATIVE query API —
// MGO's only footprint is From(ctx, db) for transaction awareness. The
// bootstrap returns the same (PostRepo, orm.Transactor) pair either way:
// that is the "identical bootstrap shape" the Phase 4 exit demands.

import (
	"context"
	"database/sql"
	"fmt"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/glebarez/sqlite" // registers database/sql driver "sqlite" too
	dbsql "github.com/mgo-framework/mgo/adapters/db-sql"
	mgoent "github.com/mgo-framework/mgo/adapters/orm-ent"
	mgogorm "github.com/mgo-framework/mgo/adapters/orm-gorm"
	"github.com/mgo-framework/mgo/contracts/orm"
	"github.com/mgo-framework/mgo/examples/blog/ent"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openStore(driver, dsn string) (PostRepo, orm.Transactor, error) {
	switch driver {
	case "gorm":
		g, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Discard})
		if err != nil {
			return nil, nil, err
		}
		db := mgogorm.New(g)
		if err := db.AutoMigrator(&Post{}).Migrate(context.Background()); err != nil {
			return nil, nil, err
		}
		return gormRepo{db}, db, nil

	case "sql":
		raw, err := sql.Open("sqlite", dsn)
		if err != nil {
			return nil, nil, err
		}
		db := dbsql.New(raw)
		if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS posts (
			id INTEGER PRIMARY KEY AUTOINCREMENT, title TEXT NOT NULL)`); err != nil {
			return nil, nil, err
		}
		return sqlRepo{db}, db, nil

	case "ent":
		// ent requires the foreign_keys pragma on for SQLite. This is the
		// modernc/glebarez driver's pragma syntax (mattn's would be _fk=1).
		if dsn != "" && dsn[0] != ':' {
			dsn += "?_pragma=foreign_keys(1)"
		}
		raw, err := sql.Open("sqlite", dsn)
		if err != nil {
			return nil, nil, err
		}
		drv := entsql.OpenDB(dialect.SQLite, raw)
		client := ent.NewClient(ent.Driver(drv))
		if err := client.Schema.Create(context.Background()); err != nil {
			return nil, nil, err
		}
		tx := mgoent.New(
			client.Tx,
			ent.NewTxContext,
			func(ctx context.Context) (*ent.Tx, bool) {
				t := ent.TxFromContext(ctx)
				return t, t != nil
			},
		)
		return entRepo{client}, tx, nil

	default:
		return nil, nil, fmt.Errorf("unknown db driver %q (want gorm, sql, or ent)", driver)
	}
}

// ---- GORM repository: native GORM calls ----

type gormRepo struct{ db *mgogorm.DB }

func (r gormRepo) Create(ctx context.Context, title string) (int64, error) {
	p := Post{Title: title}
	if err := mgogorm.From(ctx, r.db).Create(&p).Error; err != nil {
		return 0, err
	}
	return p.ID, nil
}

func (r gormRepo) List(ctx context.Context) ([]Post, error) {
	var posts []Post
	err := mgogorm.From(ctx, r.db).Order("id").Find(&posts).Error
	return posts, err
}

// ---- raw SQL repository: native database/sql calls ----

type sqlRepo struct{ db *dbsql.DB }

func (r sqlRepo) Create(ctx context.Context, title string) (int64, error) {
	res, err := dbsql.From(ctx, r.db).ExecContext(ctx,
		`INSERT INTO posts (title) VALUES (?)`, title)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r sqlRepo) List(ctx context.Context) ([]Post, error) {
	rows, err := dbsql.From(ctx, r.db).QueryContext(ctx,
		`SELECT id, title FROM posts ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	posts := []Post{}
	for rows.Next() {
		var p Post
		if err := rows.Scan(&p.ID, &p.Title); err != nil {
			return nil, err
		}
		posts = append(posts, p)
	}
	return posts, rows.Err()
}

// ---- ent repository: native ent calls ----

type entRepo struct{ client *ent.Client }

func (r entRepo) Create(ctx context.Context, title string) (int64, error) {
	client := r.client
	if tx := ent.TxFromContext(ctx); tx != nil {
		client = tx.Client()
	}
	p, err := client.Post.Create().SetTitle(title).Save(ctx)
	if err != nil {
		return 0, err
	}
	return int64(p.ID), nil
}

func (r entRepo) List(ctx context.Context) ([]Post, error) {
	client := r.client
	if tx := ent.TxFromContext(ctx); tx != nil {
		client = tx.Client()
	}
	posts, err := client.Post.Query().Order(ent.Asc("id")).All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Post, len(posts))
	for i, p := range posts {
		out[i] = Post{ID: int64(p.ID), Title: p.Title}
	}
	return out, nil
}
