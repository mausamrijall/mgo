package mgotest_test

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	appc "github.com/mgo-framework/mgo/contracts/app"
	queuec "github.com/mgo-framework/mgo/contracts/queue"
	"github.com/mgo-framework/mgo/framework/di"
	"github.com/mgo-framework/mgo/framework/events"
	"github.com/mgo-framework/mgo/framework/mgo"
	"github.com/mgo-framework/mgo/framework/mgotest"
	"github.com/mgo-framework/mgo/framework/queue"
	"github.com/mgo-framework/mgo/framework/web"
)

type pingService struct{ msg string }

type pingProvider struct{}

func (pingProvider) Register(app appc.App) error {
	return di.Singleton[*pingService](app.Container(), func() *pingService {
		return &pingService{msg: "pong"}
	})
}

func TestAppHarnessBootsAndResolves(t *testing.T) {
	app := mgotest.App(t,
		mgo.WithConfig(mgotest.Config(t, map[string]any{"greeting": "hi"})),
		mgo.WithProviders(pingProvider{}),
	)
	if got := app.Config().String("greeting", ""); got != "hi" {
		t.Fatalf("config = %q", got)
	}
	svc := di.MustMake[*pingService](app.Container())
	if svc.msg != "pong" {
		t.Fatalf("service = %q", svc.msg)
	}
}

type tickRunner struct{ ticks atomic.Int32 }

func (r *tickRunner) Name() string { return "tick" }
func (r *tickRunner) Run(ctx context.Context) error {
	r.ticks.Add(1)
	<-ctx.Done()
	return nil
}

type tickProvider struct{ r *tickRunner }

func (tickProvider) Register(appc.App) error { return nil }
func (p tickProvider) Boot(ctx context.Context, app appc.App) error {
	app.AddRunner(p.r)
	return nil
}

func TestAppStartRunsRunnersAndCleansUp(t *testing.T) {
	r := &tickRunner{}
	mgotest.App(t, mgo.WithProviders(tickProvider{r})).Start()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && r.ticks.Load() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	if r.ticks.Load() == 0 {
		t.Fatal("runner never started")
	}
	// Cleanup shuts it down; the harness fails the test itself if not.
}

func TestHTTPDSL(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /whoami", func(w http.ResponseWriter, r *http.Request) {
		web.JSON(w, http.StatusOK, map[string]string{
			"auth": r.Header.Get("Authorization"),
		})
	})
	mux.HandleFunc("POST /echo", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Msg string }
		if err := web.Bind(r, &in); err != nil {
			web.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		web.JSON(w, http.StatusCreated, in)
	})

	var who struct{ Auth string }
	mgotest.Get(t, mux, "/whoami", mgotest.Bearer("tok")).
		RequireStatus(http.StatusOK).
		JSON(&who)
	if who.Auth != "Bearer tok" {
		t.Fatalf("auth = %q", who.Auth)
	}

	var echo struct{ Msg string }
	mgotest.Post(t, mux, "/echo", map[string]string{"msg": "hello"}).
		RequireStatus(http.StatusCreated).
		JSON(&echo)
	if echo.Msg != "hello" {
		t.Fatalf("msg = %q", echo.Msg)
	}
}

func TestQueueRecorder(t *testing.T) {
	rec := mgotest.NewQueueRecorder()
	rec.Enqueue(context.Background(), queuec.Job{Type: "email:send", Payload: []byte("a")})
	rec.Enqueue(context.Background(), queuec.Job{Type: "report:build"})
	if len(rec.Jobs()) != 2 || len(rec.JobsOf("email:send")) != 1 {
		t.Fatalf("recorded = %+v", rec.Jobs())
	}
}

type SignedUp struct {
	Email string `json:"email"`
}

func TestRecordEventsSyncAndQueued(t *testing.T) {
	q := queue.NewMemory(2, 3)
	bus := events.New(events.WithQueue(q, q))
	rec := mgotest.RecordEvents[SignedUp](bus)

	// A queued listener too, to prove Wait covers async delivery.
	var queued atomic.Int32
	events.Listen(bus, func(ctx context.Context, e SignedUp) error {
		queued.Add(1)
		return nil
	}, events.Queued())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go q.Run(ctx)

	events.Dispatch(ctx, bus, SignedUp{Email: "a@b.c"})
	events.Dispatch(ctx, bus, SignedUp{Email: "d@e.f"})

	got := rec.Wait(t, 2, 2*time.Second)
	if got[0].Email != "a@b.c" || got[1].Email != "d@e.f" {
		t.Fatalf("recorded = %+v", got)
	}
}

// rollbackTx implements orm.Transactor with commit/rollback counters.
type rollbackTx struct{ commits, rollbacks int }

func (f *rollbackTx) InTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if err := fn(ctx); err != nil {
		f.rollbacks++
		return err
	}
	f.commits++
	return nil
}

func TestInRollbackNeverCommits(t *testing.T) {
	tx := &rollbackTx{}
	ran := false
	mgotest.InRollback(t, tx, func(ctx context.Context) { ran = true })
	if !ran || tx.commits != 0 || tx.rollbacks != 1 {
		t.Fatalf("ran=%v commits=%d rollbacks=%d, want true/0/1", ran, tx.commits, tx.rollbacks)
	}
}

func TestInRollbackSurfacesRealErrors(t *testing.T) {
	failing := failingTx{}
	fake := &testingStub{}
	mgotest.InRollback(fake, failing, func(ctx context.Context) {})
	if !fake.failed {
		t.Fatal("real transaction error was swallowed")
	}
}

type failingTx struct{}

func (failingTx) InTx(context.Context, func(context.Context) error) error {
	return errors.New("connection lost")
}

// testingStub captures Fatalf without ending the real test.
type testingStub struct {
	testing.TB
	failed bool
}

func (s *testingStub) Helper()               {}
func (s *testingStub) Fatalf(string, ...any) { s.failed = true }
