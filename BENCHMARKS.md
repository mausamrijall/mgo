# MGO Benchmarks

> Reproduce: `cd benchmarks && go test -bench=. -benchtime=1000000x -count=3 -run=NONE .`
> Machine for the numbers below: Intel i5-8400 @ 2.80GHz, Go 1.26.5, linux/amd64. Medians of 3 runs.

## The claim MGO certifies

**MGO adds no measurable overhead to the router you chose.** That is the
whole point of glue-not-wrapper: `mgochi.New()` embeds `*chi.Mux`,
`stdmux.New()` embeds `*http.ServeMux` — the serve path is the library's
own code. `TestGlueOverheadCertification` FAILS the build if either
adapter exceeds 105% of its raw library.

| benchmark | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| raw chi | 1156 | 752 | 6 |
| **mgo-chi** | 967 | **752** | **6** |
| raw ServeMux | 485 | 64 | 3 |
| **mgo-stdmux** | 484 | **64** | **3** |

Identical allocation profiles; ns/op differences are machine noise
(the adapters add zero code to the hot path).

## Cross-framework context

Same endpoint (`GET /posts/{id}` → small JSON), each framework's
idiomatic handler:

| framework | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| gin | 416 | 96 | 3 |
| echo | 417 | 80 | 2 |
| **mgo-stdmux** | 484 | 64 | 3 |
| **mgo-chi** | 967 | 752 | 6 |
| mgo-chi + full middleware stack¹ | 4375 | 1354 | 20 |

¹ RequestID (crypto/rand token) + Recover + structured request logging +
SecureHeaders. Features cost what features cost — and they are opt-in
per route, not baked into the router.

**Honest reading:** gin and echo out-run chi's router on this
micro-benchmark — pooled contexts and custom JSON writers buy real
nanoseconds. MGO does not compete on that axis: it makes *your chosen
router's* performance be the performance, and the stdlib-mux path is
within ~15% of gin/echo with fewer bytes per request. If routing
nanoseconds are your bottleneck, pick the fast router — MGO's overhead
on top is certified ~0 either way.

**Fiber is deliberately absent:** it runs on fasthttp, so an in-process
`net/http` comparison would measure the compatibility shim, not fiber.
A fair fiber comparison needs a socket-level load test.

## Other certified numbers (in-repo, gated by tests)

| subsystem | result | gate |
|---|---|---|
| DI singleton hit | 43 ns/op, **0 allocs** | `di.TestHotPathZeroAllocations` |
| DI scoped hit | 56 ns/op, **0 allocs** | same |
| Cache `Remember` herd | 1000 concurrent misses → **1** upstream call | `cache.TestHerd` |
| Kernel size | 1,649 LOC of 8,000 budget | `benchmarks.TestKernelStaysWithinLOCBudget` |
| contracts dependencies | **0**, forever | `benchmarks.TestContractsHaveZeroDependencies` |
| framework dependencies | contracts only | `benchmarks.TestFrameworkRequiresOnlyContracts` |
