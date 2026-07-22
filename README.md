# MGO — the Go Application Platform

**Glue, not a cage.** MGO gives Go the Laravel-grade developer experience —
project scaffolding, auth, jobs, events, caching, OpenAPI, testing — while
your code stays *native*: chi routes are chi's API, GORM queries are GORM,
handlers are `net/http`. Delete MGO any time; working Go code remains.

```sh
mgo new my-saas --preset api    # compiling, tested app in seconds
cd my-saas && mgo dev           # hot reload
```

## Why MGO is different

- **No wrappers.** The router contract is `http.Handler + Use + Mount` —
  a bare `*chi.Mux` satisfies it with zero adapter code (there's a
  conformance test proving it). There is no `mgo.Ctx`; helpers are plain
  functions over `ResponseWriter`/`*Request`.
- **Certified ~0 overhead.** A CI test fails the build if an adapter
  exceeds 105% of its raw library. See [BENCHMARKS.md](BENCHMARKS.md).
- **`mgo.json` + `mgo swap`.** Your stack choices live in a manifest;
  `mgo swap router stdmux` or `swap db sql` regenerates exactly the files
  each axis owns — and refuses to touch anything you've edited
  (sha256-tracked). Architectural experimentation without destruction.
- **The kernel is 1,649 lines** with zero third-party dependencies in
  `contracts/` — enforced by tests, forever.

## What's in the box

| capability | how |
|---|---|
| App lifecycle | providers, runners, graceful shutdown (`framework/mgo`) |
| DI container | compiled plans, 0-alloc hits, contextual/deferred bindings |
| Routing | chi / stdlib ServeMux adapters, stdlib-shaped middleware |
| Database | GORM / ent / sqlc / raw SQL adapters, tx-in-ctx (`orm.Transactor`) |
| Migrations | goose adapter behind one `Migrator` hook |
| Auth | JWT + sessions (scs) + argon2id, guards, ability Gate, CSRF |
| Cache | `Remember[T]` + singleflight, memory + redis, locks, counters |
| Jobs | queue contract, memory + asynq drivers, graceful drain |
| Scheduler | gocron adapter, `WithoutOverlapping`, `OnOneServer` |
| Events | first-party typed bus, queued listeners, `DispatchAfterCommit`, outbox |
| Modules | namespaced vertical slices with isolation linting |
| OpenAPI 3.1 | free baseline from route metadata + typed `SchemaOf[T]`, Swagger UI |
| gRPC | server lifecycle, health service, monolith→service extraction pattern |
| Health | live/ready aggregation over one `Checker` contract |
| Testing | `mgotest`: app harness, HTTP DSL, contract fakes, `InRollback` |

## Layout

```
contracts/   the constitution — stdlib-only interfaces, zero deps forever
framework/   kernel + first-party glue (requires only contracts)
adapters/    one thin module per library (router-chi, orm-gorm, ...)
cli/         the mgo binary
examples/    hello, routing, blog (3 DB drivers), authdemo, greeter (gRPC)
benchmarks/  published numbers + the gates that keep them honest
```

## Status

**v1.0.0-alpha** — all 14 roadmap phases implemented and tested
(86.6% framework coverage, race-clean, fuzzed, govulncheck-clean).
See [CHANGELOG.md](CHANGELOG.md) and [RELEASE.md](RELEASE.md) for the
remaining path to a stable 1.0.
