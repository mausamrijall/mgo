# Changelog

## v1.0.0-alpha.1 — 2026-07-18

The complete 14-phase roadmap, built under the glue philosophy
(doc 07): small kernel, thin adapters over best-in-class libraries,
everything deletable.

- **P1 Kernel** — app lifecycle, DI container, layered config, stdlib
  HTTP runner. Zero third-party deps in contracts (enforced by test).
- **P2 Routing** — `Router = http.Handler + Use + Mount`; chi + stdmux
  adapters; stdlib-shaped middleware set; conformance suite. No verb
  wrappers, no `mgo.Ctx` — ever.
- **P3 Container hardening** — compiled resolution plans, 0-alloc
  singleton/scoped hits (gated), contextual + deferred bindings, typed
  FuncBinder path, request scopes, golden diagnostics.
- **P4 ORM glue** — `Transactor`/`HealthChecker`/`Migrator` contracts;
  db-sql, orm-gorm, orm-ent (zero ent dep via generics), orm-sqlc,
  migrate-goose; blog example identical on 3 drivers.
- **P5 CLI** *(frozen after slice 2)* — `mgo new` (<30s, tested output),
  hash-guarded `swap`/`add`/`remove`, `make`, `dev` hot reload,
  manifest-powered `info`/`diff`/`doctor`. mgo.json is the brain.
- **P6 Auth** — Guard/Identity contracts, multi-guard middleware,
  ability Gate, CSRF; auth-jwt, session-scs (rotation on login),
  hash-argon2 (PHC). Feature-tested on two routers + security audit.
- **P7 Cache + Queue + Scheduler** — `Remember[T]` with 1000→1 herd
  gate; redis adapter with compare-and-delete locks; queue contract
  with graceful drain; asynq adapter; gocron with `OnOneServer`.
- **P8 Events** — first-party typed bus; queued listeners over the
  queue contract; `DispatchAfterCommit` (commit delivers exactly once,
  rollback drops — gated); CloudEvents envelope; outbox + relay.
- **P9 Modules** — namespaced vertical slices (routes/config/
  migrations), relocatable by contract, isolation demo + linter.
- **P10 OpenAPI 3.1** — free baseline from route metadata,
  `SchemaOf[T]` reflection, progressive `Describe`, Swagger UI.
- **P11 Distributed** — gRPC lifecycle adapter with health service;
  live/ready aggregation; monolith→gRPC extraction with an unchanged
  test suite.
- **P12 mgotest** — app harness, HTTP DSL, contract fakes,
  `InRollback`; framework self-coverage 86.6%, race-clean.
- **P13 Performance** — benchmark publication vs gin/echo; certified
  ≤5% adapter overhead (measured ~0); kernel budget gates as tests.
- **P14 Release** — fuzzing (found and fixed a real DoS: hostile PHC
  strings could panic argon2 Verify), govulncheck clean after
  upgrading jwt v5.2.2 / go-redis v9.7.3 / grpc v1.79.3.
