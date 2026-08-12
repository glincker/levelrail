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

### 1.5 Git integration. DONE (2026-08-12)

- [x] Webhook receiver: new `internal/webhook` package. `Handler` is a
      plain `http.Handler` (no `http.ListenAndServe` inside it), so the
      HTTP API package (1.9) mounts it later at whatever path it
      chooses, the same "don't stand up a server, expose a handler"
      shape the rest of this phase has kept to
- [x] Signature verification: HMAC-SHA256 over the raw body against
      `Config.Secret`, GitHub's `X-Hub-Signature-256` scheme, compared
      with `hmac.Equal` (constant-time by construction). An empty
      configured secret always rejects rather than degrading to "any
      signature passes." Table-driven tests cover valid, wrong-secret,
      missing-header, non-hex, and the case that actually exercises
      whether the comparison uses the right bytes: a tampered payload
      checked against a signature computed over the *original* body,
      not just a missing-signature check. A dedicated test also proves
      a signature differing in only its last byte is rejected exactly
      like one differing in its first, guarding against a future
      refactor accidentally reintroducing a short-circuiting compare
- [x] Branch gating: only a push whose `ref` matches `Config.Branch`
      (default `main`) triggers a deploy. Any other branch returns 200
      (so GitHub doesn't retry) with a response body that says the push
      was ignored and why, verified by a test that also asserts the
      git-fetch step never runs for the wrong branch
- [x] Fetching source at a SHA: `github.com/go-git/go-git/v5`, no `git`
      binary dependency, consistent with `internal/docker` and
      `internal/build`'s "no shelling out" doc comments. Full clone, not
      shallow: documented tradeoff in `cloneAndCheckout`'s doc comment,
      a depth-1 clone isn't guaranteed to contain the pushed SHA if the
      branch moved between the push and the fetch. Checks out into a
      fresh temp dir per call and always cleans it up (`defer cleanup()`
      covers both the success and failure paths), so repeated webhook
      calls don't leak disk
- [x] Commit SHA tracking through to the image tag: the pushed `after`
      SHA flows straight into `deploy.Request.CommitSHA`, which
      `internal/deploy` (1.4) already tags the built image with, no new
      plumbing needed on that side, this was the missing caller
- [x] Auth decision: neither deploy keys nor a GitHub App, a shared
      HMAC webhook secret (`Config.Secret`) instead. Simpler than both
      for Phase 1's single-repo scope, and GitHub's push webhook payload
      only needs read access to know what was pushed; fetching the
      source itself goes over `Config.RepoURL`, which is server-side
      static configuration, never taken from the untrusted payload,
      so a forged payload can't redirect a clone at an
      attacker-controlled remote. Revisit deploy keys/GitHub App if a
      private repo needs authenticated clones (this repo's own clone
      today assumes a reachable/public or otherwise pre-authorized
      remote; wiring credentials into the go-git clone call is a small
      follow-up once thesvg.org's actual repo visibility is decided,
      not a redesign)
- [x] `Config` is deliberately static, single-app configuration (one
      secret, one repo, one branch, one `spec.Service`), not a
      multi-tenant "which repo maps to which app.yaml" registry.
      Documented as a scope boundary in the package doc comment,
      matching CLAUDE.md 7's "don't design for hypothetical future
      requirements": a second app arriving is the trigger to build that
      registry, not a hypothesis about one arriving

Response codes verified by table-driven `httptest`-based tests against a
fake `Deployer` (mirroring the `ImageBuilder`/`ServiceStore` narrow
interfaces `internal/deploy` already established): 200 for a triggered or
correctly-ignored push, 400 for a malformed payload or one missing
`ref`/`after`, 401 for a missing or invalid signature, 500 with a fixed,
non-leaky message (checked directly: the fake's real error text must not
appear in the response body) when either the git fetch or the deploy call
fails. A live test builds a real two-commit repo with `go-git` itself in a
temp dir (no clone of an actual GitHub repo, so nothing here can flake on
GitHub's availability) and proves `cloneAndCheckout` lands the first
commit's file content, not the branch tip's, and that a bad SHA fails the
checkout instead of silently landing on some other commit. 86.9% coverage
on `internal/webhook`.

Not built here, deliberately: GitHub App auth (see the auth decision
above), deploy progress streaming to a UI (build progress is logged via
`slog` through `build.SlogProgress`, actual SSE streaming is 1.9's job,
same boundary 1.4 already drew), and any multi-repo/multi-app registry
(see the `Config` scope-boundary note above).

### 1.6 Ingress integration

- [ ] Wire `internal/ingress` (currently a standalone spike) into the
      deploy flow: app spec's `domains` drives Caddy config
- [ ] Certificate/ACME account state moved into `internal/store`
      (currently the spike uses Caddy's default file-system storage
      module, per ADR 005's Verified section)
- [ ] Real ACME spot-check against an actual public domain. Not
      optional: the Phase 1 exit criterion specifically requires
      real HTTPS, and only the internal/self-signed issuer path has
      been verified so far

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
