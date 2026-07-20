// Command blog is the Phase 4 exit demo: the SAME application on GORM,
// raw database/sql, or ent, chosen by config — identical bootstrap shape,
// shared handlers, txn-in-ctx working across all three:
//
//	MGO_DB_DRIVER=gorm go run .   # default
//	MGO_DB_DRIVER=sql  go run .
//	MGO_DB_DRIVER=ent  go run .
//
// Endpoints:
//
//	POST /posts        {"title": "..."}            create one post
//	GET  /posts                                    list posts
//	POST /posts/batch  {"titles": ["a", "b"]}      all-or-nothing via InTx
//
// The batch endpoint rejects empty titles mid-transaction; the whole batch
// rolls back — same behavior on both drivers, driven by orm.Transactor.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	stdmux "github.com/mgo-framework/mgo/adapters/router-stdmux"
	"github.com/mgo-framework/mgo/contracts/orm"
	"github.com/mgo-framework/mgo/framework/conf"
	"github.com/mgo-framework/mgo/framework/httpserver"
	"github.com/mgo-framework/mgo/framework/mgo"
	"github.com/mgo-framework/mgo/framework/middleware"
	"github.com/mgo-framework/mgo/framework/openapi"
	"github.com/mgo-framework/mgo/framework/web"
)

// Post is the shared domain shape; each store maps it natively.
type Post struct {
	ID    int64  `json:"id" gorm:"primarykey"`
	Title string `json:"title"`
}

// PostRepo is the app's own port — MGO does not define repository
// contracts; data access is the application's business.
type PostRepo interface {
	Create(ctx context.Context, title string) (int64, error)
	List(ctx context.Context) ([]Post, error)
}

func main() {
	cfg, err := conf.NewLoader().DotEnv(".env", true).Env("MGO_").Load()
	if err != nil {
		slog.Error("config", "error", err)
		os.Exit(1)
	}

	driver := cfg.String("db.driver", "gorm")
	repo, tx, err := openStore(driver, cfg.String("db.dsn", "file:blog.db?_pragma=busy_timeout(5000)"))
	if err != nil {
		slog.Error("store", "driver", driver, "error", err)
		os.Exit(1)
	}
	slog.Info("store ready", "driver", driver)

	// Identical from here on, whatever the driver: this is the exit shape.
	router := stdmux.New()
	router.Use(middleware.RequestID(), middleware.Recover(nil), middleware.Logger(nil))
	router.HandleFunc("POST /posts", createHandler(repo))
	router.HandleFunc("GET /posts", listHandler(repo))
	router.HandleFunc("POST /posts/batch", batchHandler(repo, tx))

	// OpenAPI: baseline from the router's own route metadata, enriched
	// with typed schemas. Docs routes register after FromRouter, so the
	// meta-endpoints stay out of their own spec.
	spec := openapi.FromRouter(router, openapi.Info{
		Title: "Blog API", Version: "1.0.0",
		Description: "The MGO blog example — one API, three storage drivers.",
	})
	spec.Describe("GET /posts", openapi.Op{
		Summary:   "List posts",
		Responses: map[int]openapi.R{200: {Schema: openapi.SchemaOf[[]Post]()}},
	})
	spec.Describe("POST /posts", openapi.Op{
		Summary: "Create a post",
		Request: openapi.SchemaOf[CreatePost](),
		Responses: map[int]openapi.R{
			201: {Schema: openapi.SchemaOf[Post]()},
			400: {Description: "Malformed body"},
		},
	})
	spec.Describe("POST /posts/batch", openapi.Op{
		Summary: "Create posts atomically — all or nothing",
		Request: openapi.SchemaOf[CreateBatch](),
		Responses: map[int]openapi.R{
			201: {Schema: openapi.SchemaOf[[]Post]()},
			422: {Description: "Invalid title in batch; whole batch rolled back"},
		},
	})
	router.HandleFunc("GET /openapi.json", spec.Handler())
	router.HandleFunc("GET /docs", openapi.SwaggerUI("/openapi.json"))

	app := mgo.New(
		mgo.WithConfig(cfg),
		mgo.WithProviders(httpserver.Provider("http", router)),
	)
	if err := app.Run(context.Background()); err != nil {
		slog.Error("app failed", "error", err)
		os.Exit(1)
	}
}

// ---- handlers: ordinary net/http, driver-agnostic ----

// CreatePost is the create-post request body.
type CreatePost struct {
	Title string `json:"title" doc:"Post headline"`
}

// CreateBatch is the batch request body.
type CreateBatch struct {
	Titles []string `json:"titles" doc:"All created in one transaction"`
}

func createHandler(repo PostRepo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in CreatePost
		if err := web.Bind(r, &in); err != nil {
			web.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		id, err := repo.Create(r.Context(), in.Title)
		if err != nil {
			web.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		web.JSON(w, http.StatusCreated, Post{ID: id, Title: in.Title})
	}
}

func listHandler(repo PostRepo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		posts, err := repo.List(r.Context())
		if err != nil {
			web.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		web.JSON(w, http.StatusOK, posts)
	}
}

// batchHandler creates all titles inside one transaction: any invalid
// title rolls the whole batch back, on either driver.
func batchHandler(repo PostRepo, tx orm.Transactor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in CreateBatch
		if err := web.Bind(r, &in); err != nil {
			web.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		created := make([]Post, 0, len(in.Titles))
		err := tx.InTx(r.Context(), func(ctx context.Context) error {
			for _, title := range in.Titles {
				if title == "" {
					return fmt.Errorf("empty title in batch")
				}
				id, err := repo.Create(ctx, title)
				if err != nil {
					return err
				}
				created = append(created, Post{ID: id, Title: title})
			}
			return nil
		})
		if err != nil {
			web.Error(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		web.JSON(w, http.StatusCreated, created)
	}
}
