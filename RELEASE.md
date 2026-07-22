# Path to a stable v1.0.0

What v1.0.0-alpha.1 already has: 14 phases implemented, 86.6% framework
coverage, race-clean, fuzz corpus committed, govulncheck clean, certified
adapter overhead, kernel budget gates in CI-runnable tests.

What still stands between alpha and a stable 1.0 — deliberately not
faked in-repo:

- [ ] **License** — the repository has no LICENSE file; the owner must
  pick one (MIT or Apache-2.0 are the ecosystem norms) before anyone
  can legally adopt MGO.
- [ ] **Module publishing** — module paths are `github.com/mgo-framework/mgo/*`
  but the repo lives at `github.com/mausamrijall/mgo`, held together by
  `replace` directives. Before 1.0: either move the repo / set up the
  vanity import domain, then tag each module
  (`contracts/v1.0.0`, `framework/v1.0.0`, ...) so `go get` works
  without replaces, and drop the replace-generation from `mgo new`.
- [ ] **Contracts freeze + semver policy** — declare contracts/ API
  frozen at the 1.0 tag; adopt an RFC process for any addition.
- [ ] **External security audit** — the auth/session/CSRF surfaces
  deserve independent eyes; in-repo fuzzing is a floor, not an audit.
- [ ] **Pilot applications** — the roadmap's bar: 3 real apps in
  production ≥ 30 days on the alpha.
- [ ] **Release pipeline** — CI running the full workspace matrix +
  certification gates, SBOM generation, cross-compile artifacts for the
  mgo CLI, OCI base images.
- [ ] **Deferred features** (post-1.0 or with the CLI unfreeze):
  OIDC + casbin adapters, OTel end-to-end tracing, registry/resilience
  adapters, testcontainers helpers, ent/sqlc generator axes,
  `mgo migrate`, curated adapter index, `mgo add frontend`.
