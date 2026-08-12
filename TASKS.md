# Tasks

Working task list against CLAUDE.md's phases (section 6). Fine-grained
breakdown only for the phase actively being built; future phases stay at
CLAUDE.md's own summary level until we're actually there; inventing detailed
tasks for Phase 3 now would be exactly the kind of speculative planning
CLAUDE.md 7 argues against ("Don't design for hypothetical future
requirements").

Status legend: `[ ]` not started, `[~]` in progress, `[x]` done.

---

## Phase 0: Research and foundation. COMPLETE (2026-08-11)

- [x] Naming (four rounds, Levelrail, see `docs-local/research/naming/`)
- [x] Competitor research, all five (`docs-local/research/prior-art-*.md`)
- [x] Repo scaffold, brand indirection layer (`internal/brand`)
- [x] Reconcile loop skeleton, live-verified kill-and-recover demo
- [x] BuildKit spike (`internal/build`), live-verified
- [x] Caddy spike (`internal/ingress`), live-verified (internal TLS issuer only, real ACME still unverified)
- [x] SQLite store with migrations (`internal/store`), wired into the reconciler
- [x] CI and lint scaffold
- [x] ADRs 001-008

Exit criterion met: reconciler skeleton demoed live, prior art documented
in writing.

---

## Phase 1: Single node, deploy something real. IN PROGRESS

Exit criterion: thesvg.org deploys from a git push, serves over HTTPS with
a valid cert, and you can roll back to the previous version in one click.

Deliberately excluded from Phase 1 (CLAUDE.md 6): multi-server, templates,
backups to S3, teams, notifications, Compose support.

### 1.1 App spec (`internal/spec`). DONE (2026-08-11)

- [x] Go structs for the app.yaml shape (CLAUDE.md 4.9)
- [x] Hand-written JSON Schema, embedded, validated at parse time
- [x] Candidate-filename discovery (`app.yaml`, `deploy.yaml`, plus the
      branded name, per CLAUDE.md section 3); takes the branded stem as
      a caller-supplied string rather than importing `internal/brand`
      directly, to keep the two packages decoupled
- [x] `EnvVar` union type: literal string, `{ from: ... }` reference, or
      `{ secret: true, required: true }`
- [x] Semantic validation beyond JSON Schema's reach: `build.path`
      required for `compose`, `port` required unless `static`, domain
      uniqueness across all services, service/database name shape
- [x] 82.3% test coverage, real fixture files under `testdata/`, no
      mocks needed since there's no I/O beyond reading a byte slice

### 1.2 Docker wrapper extensions (`internal/docker`). DONE (2026-08-11)

- [x] Extend `ContainerSpec`: ports (`PortBinding`, host port 0 means
      auto-assign), env vars, resource limits (`Resources`: memory
      bytes, nano-CPUs, the Engine API's own units, not app.yaml's
      "512Mi"/0.5-cores units; that translation is the application
      controller's job, 1.3)
- [x] Restart policy: decided against a Docker-native one. Every
      container gets Docker's `"no"` policy explicitly; the reconciler,
      not Docker, is the sole authority on whether a dead container
      comes back (CLAUDE.md 4.2). Verified live: a real container's
      `HostConfig.RestartPolicy.Name` inspected directly is `"no"`.
- [x] Health check: decided on an internal prober (Kubernetes-style, the
      controller calls out to the container) over Docker's own
      `HEALTHCHECK` state machine, the same choice ADR 002 already
      commits to over Coolify's confirmed weaker alternative. The actual
      prober is 1.3 scope; this package only had to make a container's
      address discoverable, done via the new `ContainerState.Ports`.
- [x] Image tagging by commit SHA: no new code needed, `ContainerSpec.Image`
      already accepts any tag. What rollback actually needs is the new
      `ListImages(repo)`, discovering available tagged versions,
      newest first, deduped and live-verified against a real image.

Two real bugs caught by live testing, not by unit tests: `observedPorts`
returned duplicate bindings (Docker reports one entry per bind IP, so a
port published on all interfaces appeared twice) and an empty-but-non-nil
slice instead of `nil` on the "nothing published" path. Both fixed.
Also closed a Phase 0 gap while in this file: `Events` and `mapAction`
had zero test coverage since Phase 0; a live test now proves real
start/die events flow through end to end, not just that the plumbing
compiles. 78.2% coverage.

### 1.3 Application controller. DONE (2026-08-12)

Landed across four packages, each with its own tests, all wired
together and live-verified as one pipeline at the end.

- [x] `internal/docker`: `Stop`, `Remove`, `ListByPrefix` added, the
      three primitives blue-green cutover needs beyond what `nginxdemo`
      required
- [x] `internal/store`: `desired_services` table, `DesiredService`
      (deliberately distinct from `spec.Service`: resolved runtime
      state, an image plus what it needs, not a build recipe)
- [x] `internal/probe`: HTTP readiness checking, the mechanism, not
      Docker's own `HEALTHCHECK` (matches ADR 002 and the decision
      already recorded in `internal/docker`'s `ContainerSpec` doc
      comment). Liveness (continuous monitoring of an already-running
      container, restart after repeated failures) stays out, a
      genuinely separate reconcile-time concern, not scope creep
      avoidance for its own sake
- [x] `internal/reconcile/application`: the real `Controller`. Desired
      state read fresh from the store every reconcile, never cached.
      Deploy strategy is deterministic-name-based: a container's name
      is derived from its image (`containerName`, a short hash), so
      "does the right container exist" needs no memory beyond what
      Docker itself can answer, level-triggered by construction. A
      changed image naturally produces a new, differently-named
      container alongside the old one; only once the new one passes
      its readiness probe does the controller remove every other
      container for the service (blue-green, without needing a
      separate "which one is active" field anywhere)
- [x] Rollback mechanism: no dedicated command yet (nothing to trigger
      it from until the HTTP API, 1.9, or a CLI exist), but the
      mechanics are real and already provable: pointing `desired.Image`
      back at an older tag and reconciling converges to it the same way
      any other redeploy does, and `ListImages` (added under 1.2)
      already exposes what tags are available to roll back to. A
      dedicated rollback command is a thin wrapper over this once 1.9
      exists, not new reconciler logic

Traffic cutover itself (updating Caddy to point at the new container)
is deliberately not this controller's job: CLAUDE.md 4.2's "a reconcile
loop per resource type" makes ingress its own controller (1.6, not yet
wired). This controller's contract with that future controller is
simple: whichever container exists and is running for a service is the
one meant to receive traffic.

Live-verified end to end, not just unit tested: real store, real
Docker, fresh deploy, a no-op second reconcile (proving convergence
doesn't recreate anything), then a full redeploy against a locally
retagged image (no network dependency) proving the actual cutover: new
container created, passes a real HTTP readiness probe, old container
confirmed removed by independently inspecting for it afterward, not by
trusting the returned status alone. 86.2% coverage.

### 1.4 Build integration. DONE (2026-08-12)

- [x] Local build cache wired into `internal/build`
      (`SolveOpt.CacheImports`/`CacheExports`, `type=local`). Scoped
      honestly for Phase 1: a cache genuinely shared across dedicated
      build nodes needs a registry or object-store backend and those
      nodes to exist, neither of which lands before Phase 3. Local
      on-disk still delivers the actual value this phase needs, fast
      incremental rebuilds on one node, without inventing infrastructure
      early. Live-verified to actually accelerate a second build, not
      just wired and assumed: a real `cached=true` step observed on the
      second build of the same Dockerfile
- [x] Progress events decoupled from BuildKit's own `SolveStatus` type
      (`build.ProgressEvent`, a callback `Build` takes instead of a
      hardcoded `*slog.Logger`), so a future SSE handler (1.9) can format
      build progress for a browser without importing `moby/buildkit`.
      `SlogProgress` adapts it back to logging for callers that just
      want that. The actual SSE wire format and HTTP handler are still
      1.9's job, not built here
- [x] New `internal/deploy` package: dispatches by `build.Type` (real
      support for `dockerfile`; `static` explicitly errors until ingress
      exists to serve it, 1.6; `compose`/`railpack` explicitly error as
      not yet supported, rather than pretending), and on a successful
      build, translates the app spec's port/resources/health into
      `store.DesiredService` and saves it, the missing link that makes
      the whole system real: a build now produces something the
      application controller (1.3) actually picks up and converges
- [x] Secrets and cross-resource env references (`{ secret: true }`,
      `{ from: ... }`) are rejected explicitly, before a build is even
      attempted, rather than silently deploying a container missing
      values it needs. Real support lands with 1.7/1.8

Live-verified as one complete chain, not just as separate pieces: a real
`spec.Service` through a real BuildKit build, into a real image, into a
real SQLite desired-state row, into the application controller (1.3)
actually reconciling a real running container from it, independently
confirmed by inspecting the container afterward rather than trusting the
returned status. This is the same chain a git push will drive once 1.5
exists. 87.4% coverage on `internal/build`, 95.6% on `internal/deploy`.

### 1.5 Git integration

- [ ] Webhook receiver
- [ ] Deploy keys or GitHub App auth (decide which; deploy keys are
      simpler, GitHub App is the better long-term UX)
- [ ] Commit SHA tracking through to the image tag (depends on 1.2's
      SHA-tagging)

### 1.6 Ingress integration. DONE (2026-08-12)

- [x] `internal/reconcile/ingress`: the real ingress `reconcile.Controller`,
      wiring the Phase 0 Caddy spike (`internal/ingress`) into the deploy
      flow. ADR 005's Verified section is the load-bearing constraint this
      is built around: `caddy.Load` replaces Caddy's entire process-wide
      config on every call, there's no per-app incremental update. So
      unlike the application controller (one named service per
      `Reconcile`), this controller reconciles as a single whole pass:
      every call lists every `store.DesiredService` via
      `ListDesiredServices`, and for each with non-empty `Domains`,
      derives its currently active container the same way the
      application controller does (`application.ContainerName`, now
      exported specifically so this package can reuse the exact hash
      logic instead of reimplementing it and risking drift), inspects it,
      and if it's running with a published port, builds a route to
      `127.0.0.1:<hostPort>` (`internal/docker.PortBinding`'s own doc
      comment explains why host-port routing, not container-IP routing,
      is this codebase's deliberate choice throughout). A service with no
      valid backend right now (mid-deploy, crashed, never deployed) is
      logged and left out of that pass's config, not treated as a
      reconcile failure; only a real Docker-inspect anomaly, a config-build
      failure, or Caddy itself rejecting the applied config produce
      `Ready=False`
- [x] `internal/ingress.BuildRoutesConfig` (extended the spike, since
      `BuildProxyConfig` only ever builds one route to one backend):
      assembles one `Config` with one shared HTTPS server carrying one
      host-matched route per routable service. An empty route set is
      valid and applies cleanly, the correct shape for a pass over zero
      currently-routable services, not an error. Table-tested in
      `config_test.go` for validation and exact JSON shape, the same
      style `BuildProxyConfig` already established
- [x] TLS: Caddy's `internal` issuer only, per the Phase 0 spike's own
      finding that this is the only issuer path actually verified. Real
      ACME against a public domain is still explicitly unverified and
      deferred, not attempted here
- [x] Admin API: bound to `localhost:2019` explicitly rather than left
      disabled or left to Caddy's implicit default. Caddy's own default
      is already loopback-only, so this changes nothing about exposure,
      it just pins the choice in code (`defaultAdminListen`'s doc
      comment) so a future upstream default change can't silently widen
      it, while keeping local introspection into a stuck ingress
      reconcile available for debugging
- [x] Certificate/ACME account state: deliberately still on Caddy's
      default file-system storage module for this pass, per the task's
      own scoping. The real requirement, a `certmagic.Storage`
      implementation backed by `internal/store`'s SQLite so multi-node
      deployments share cert state, is Phase 3 work and is called out as
      an explicit follow-up in the controller's package doc comment, not
      built speculatively here
- [ ] Real ACME spot-check against an actual public domain. Still not
      done, still not optional before the Phase 1 exit criterion (real
      HTTPS) can be claimed: only the internal/self-signed issuer path
      has ever been verified, Phase 0 or now

A real, currently open gap this pass does not close, recorded in the
controller's package doc comment rather than assumed away: domain
uniqueness within one `app.yaml` is enforced by `internal/spec`'s
`Validate()`, but nothing stops two different services written to the
store by two separate deploys over time from claiming the same domain.
`BuildRoutesConfig` would silently build two routes matching the same
host in that case. Needs either a store-level uniqueness constraint or a
check in this controller before applying; neither exists yet.

Exported `application.containerName` as `application.ContainerName`
(one call site inside the application controller itself, plus its test
and live-test references) specifically so this controller could depend
on the identical derivation instead of a second copy of the same hash
logic.

Live-verified, not just unit tested against fakes: two real Docker
containers (`nginx:alpine` and `httpd:alpine`, chosen because their
default pages are two different, well-known strings), a real Caddy
instance driven entirely by this controller's `Reconcile`, and two real
HTTPS requests through Caddy, one per domain, each checked against the
*other* backend's response body too, not just its own, to actually prove
host-based routing picked the right one rather than merely returning
200. Caddy's own logs confirm real internal-issuer certs obtained for
both hostnames before either handshake succeeds. 98.1% coverage on
`internal/reconcile/ingress`, 91.5% on `internal/ingress` after the
`BuildRoutesConfig` addition.

### 1.7 Secrets (`internal/secrets`)

- [ ] Envelope encryption using `filippo.io/age`, per-app data
      encryption keys wrapped by a master key
- [ ] Env injection at container-create time, never persisted to disk
      on the agent side (single-process today, but write it as if the
      agent were remote, since 4.3's in-memory-transport design means
      it will be later without a rewrite)
- [ ] Master key sourcing: file or env for now, external KMS interface
      is explicitly a later addition per CLAUDE.md 4.10

### 1.8 Managed databases

- [ ] Postgres as a first-class resource (the app spec example in
      CLAUDE.md 4.9 uses this as its own example, `databases.main`)
- [ ] Redis as a first-class resource
- [ ] Volume management for both

### 1.9 HTTP API (`internal/api`)

- [ ] `/api/v1/brand` (CLAUDE.md section 3, the frontend's brand
      indirection depends on this existing)
- [ ] Apps CRUD
- [ ] Deploy trigger, deploy history
- [ ] Single admin user, session auth. No teams, no RBAC (explicitly
      out of scope until Phase 4)

### 1.10 Frontend (`/web`)

- [ ] Vite + React + TypeScript + Tailwind scaffold, embedded into the
      Go binary via `embed.FS` per CLAUDE.md 4.1/4.12
- [ ] App list, app detail, deploy history, live build logs (consumes
      1.4's SSE stream), env editor, domain config
- [ ] Route-level code splitting from the start, per CLAUDE.md 4.12 and
      7 ("the dashboard should not ship the log viewer's bundle")

---

## Sequencing notes

1.1 and 1.2 have no dependencies on anything else in this phase and
should land first, everything else in Phase 1 reads the app spec or
extends the Docker wrapper. 1.3 depends on both. 1.4 and 1.6 can proceed
in parallel with each other once 1.1-1.3 exist, they don't share files.
1.7 (secrets) and the database schema work inside 1.3/1.8 stay
single-session per CLAUDE.md 8's do-not-parallelize list; everything else
in this phase is fair game for parallel agents once its dependencies are
met, same as Phase 0's competitor research and ADR writing were.

## Phase 2 onward

Not broken down yet. See CLAUDE.md section 6 for the phase-level summary
(observability, multi-node, platform surface, hardening).
