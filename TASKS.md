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

### 1.7 Secrets (`internal/secrets`). DONE (2026-08-12)

Built directly, not delegated, per CLAUDE.md 8's rule on anything
touching secrets. Deliberately scoped as a standalone primitives
package, the same shape `internal/build` and `internal/ingress` were
built as Phase 0 spikes before being wired into the real flow: the
integration points (`internal/deploy`'s env resolution,
`internal/reconcile/application`'s container creation) were being
actively modified by the parallel 1.5/1.6/1.8/1.9 wave when this was
built, wiring into them here would have guaranteed a merge conflict
with the ingress controller's export of `ContainerName` and the
database controller's extension of `docker.ContainerSpec`.

- [x] Envelope encryption using `filippo.io/age`'s `HybridIdentity`/
      `HybridRecipient` (the library's own current recommended default,
      safe against future cryptographically-relevant quantum computers,
      not the older plain X25519 API). `MasterKey` wraps and unwraps
      per-app data encryption keys (`GenerateDEK`/`UnwrapDEK`); the DEK
      itself encrypts actual secret values with AES-256-GCM
      (`EncryptValue`/`DecryptValue`), the right tool for repeated
      same-key encryption, reserving age's asymmetric machinery for the
      one wrap operation per DEK
- [x] Master key sourcing: `LoadMasterKey`/`MasterKey.String()` handle
      parsing and serialization only; reading the serialized key from a
      file or env var is deliberately left to the caller (matching
      `internal/brand.Load`'s own shape), not baked into this package.
      External KMS stays a later addition per CLAUDE.md 4.10
- [x] 82.8% coverage, with real negative-case security tests, not just
      happy-path round-trips: wrong master key can't unwrap another's
      DEK, tampered ciphertext fails GCM's auth tag, tampered wrapped
      DEK fails to unwrap, same plaintext encrypted twice produces
      different ciphertext (proving the nonce is actually randomized,
      nonce reuse under AES-GCM is a critical vulnerability class), and
      a full master-key-to-value-and-back round trip through a
      simulated fresh process (only the serialized master key and the
      two ciphertexts persist, proving the pieces compose correctly, not
      just that each works in isolation)
- [x] **Integration closed (2026-08-12), single-session per CLAUDE.md 8's
      rule on anything touching secrets.** Env injection at
      container-create time, in full, `{ secret: true }` only (`{ from:
      ... }` cross-resource references stay explicitly unsupported, see
      below):
      - Schema: migration 0006 adds `service_secrets` (one wrapped DEK
        per service, generated on first use, never replaced) and
        `service_secret_values` (one AES-256-GCM ciphertext per
        service/env-key pair), plus a `secret_env` column on
        `desired_services` holding key *names* only, never values, the
        same shape `domains` already established. `internal/store`'s new
        `secret.go` is pure byte storage, it never sees a plaintext
        value or a raw DEK, matching `reconcile_status.go`'s existing
        separation of "what happened" from "what decides"
      - `internal/secrets.Manager` (`manager.go`) is the integration
        layer the package's own doc comment always said would follow the
        primitives: generate-or-reuse a service's DEK, encrypt and
        persist a value (`SetValue`), decrypt one back (`Resolve`), or
        check existence without decrypting (`Exists`, what
        `internal/deploy` uses so a deploy can fail loudly on a missing
        required secret without ever touching its value)
      - `internal/deploy.Pipeline` gained `WithSecretChecker`: with one
        configured, `{ secret: true }` env vars pass validation (a
        `required: true` one with no value set still fails the deploy,
        matching `spec.EnvVar.Required`'s documented meaning); without
        one, exactly the old behavior, an explicit rejection, so a
        control plane started without a master key fails clearly rather
        than silently dropping a variable. `{ from: ... }` stays rejected
        unconditionally: it needs managed database credentials, which
        are themselves blocked on this very work per 1.8's own scope note
      - `internal/reconcile/application.Controller` gained
        `WithSecretResolver`: immediately before
        `docker.Runtime.Create`, `resolveEnv` decrypts each
        `desired.SecretEnv` name and merges it into the container's env
        map. That map is the only place a secret's plaintext ever exists
        in this controller's memory; it is never written back to the
        store, logged, or returned in a `reconcile.Result`. A service
        with `SecretEnv` set but no resolver configured fails Reconcile
        loudly (`Reason=CreateFailed`) before ever calling `Create`. An
        unset *optional* secret is silently omitted from the container's
        environment, not an error: `internal/deploy` already rejected the
        deploy outright if it was required and unset, so by the time
        Reconcile runs, an unset one in `SecretEnv` can only be optional
      - `internal/api` gained `PUT /api/v1/apps/{name}/secrets/{key}`
        (`secrets.go`), the operator-facing way to actually set a value:
        session-authenticated, set-only (deliberately no `GET`, a
        decrypted value has no business in a response body), 501 when no
        master key is configured (`WithSecretSetter`), 404 for an unknown
        app, 400 for an empty value
      - `cmd/levelrail/main.go` sources the master key from
        `APP_MASTER_KEY` (CLAUDE.md 4.10: "file, env, or a future KMS
        interface"; env is what this pass wires). Unset is not fatal,
        the same non-fatal choice `bootstrapAdmin` already makes: the
        control plane still starts, every secrets-touching route or
        reconcile path just fails specifically and loudly instead of the
        feature not existing at all
      - Live-verified end to end, not just unit tested: a real generated
        master key, a real store, `Manager.SetValue` for a real
        `API_KEY`, a real `Controller.Reconcile` creating a real
        container, independently verified via the raw Docker Engine
        API's own `ContainerInspect` (not this controller's return
        value) that the container's actual environment contains the
        decrypted value, and via the store directly that only ciphertext,
        never that exact plaintext string, ever reached disk
      - Coverage: 81.0% on `internal/secrets` (`manager_test.go`'s 9
        cases include a wrong-master-key negative test and a real
        store-error-propagates-not-swallowed-as-not-found test),
        91.0% on `internal/reconcile/application` (up from 89.7%), 96.4%
        on `internal/deploy` (up from 95.6%), 78.1% on `internal/store`.
        85.0% on `internal/` overall, gate is 70%
      - Known gap, honestly out of scope here: nothing in
        `cmd/levelrail/main.go` constructs an `internal/deploy.Pipeline`
        or mounts `internal/webhook` yet, so `{ secret: true }` declared
        in an app.yaml has no path from a real git push into this
        control plane today. That's the webhook-to-main.go wiring gap,
        not a secrets one; every piece this task owns is real and
        live-tested independently of it

### 1.8 Managed databases. DONE (2026-08-12), Postgres scoped down

- [x] `internal/docker`: `VolumeMount` (`Name`, `ContainerPath`) added to
      `ContainerSpec`, wired into `Client.Create` via
      `container.HostConfig.Mounts` (`mount.Mount{Type: TypeVolume,
      Source, Target}`, the exact shape checked with `go doc` first per
      the buildkit spike's own lesson about not guessing Docker SDK
      shapes). New `Runtime.EnsureVolume` method wraps `VolumeCreate`,
      idempotent by construction since the Engine API itself returns an
      existing volume by name rather than erroring
- [x] Redis as a first-class resource, real and working:
      `internal/reconcile/database.Controller` creates the volume,
      creates and starts a container with it mounted at `/data`, no
      restart policy (same reasoning as `internal/docker`'s
      `ContainerSpec` doc comment: the reconciler owns restart), reports
      Ready once running. Deterministic naming, but *not* a reuse of
      `application.containerName`'s hash-of-image approach: a database
      container exclusively owns its data volume, so unlike a stateless
      application container it cannot safely run an old and a new
      version side by side during a cutover the way blue-green does.
      This controller keeps exactly one container name per database
      (`db-<name>`) and, when the desired engine version changes, does a
      strictly sequential stop, remove, then create/start against the
      same stable volume (`db-<name>-data`), never two containers
      holding the volume at once. Documented in `containerName`'s doc
      comment in `internal/reconcile/database/controller.go`
- [x] Volume management for both: `EnsureVolume` plus the mount wiring
      above are engine-agnostic: Postgres gets the identical mechanism
      the moment it's unblocked (below), not a separate implementation
- [~] Postgres as a first-class resource: **deliberately not started**.
      `store.DesiredDatabase` (built ahead of this task) already has no
      credentials field, exactly because database auth needs the same
      envelope-encrypted secret storage CLAUDE.md 4.10 specifies, which
      doesn't exist until TASKS.md 1.7 lands. Rather than work around
      that with Postgres trust-auth (no password) or any other
      "temporary" insecure shortcut, `Controller.Reconcile` for a
      Postgres database checks for credentials, finds none, and returns
      `Status=False Reason=CredentialsNotYetSupported` with a message
      naming 1.7 as the blocker, every time, without ever calling
      `runtime.Create`. Structured so this activates cleanly once 1.7
      exists rather than needing a rewrite: `Controller` already carries
      an optional `*PostgresCredentials` (`Username`, `Password`, always
      nil today, set via `WithPostgresCredentials`), and the same
      `reconcileEngine` volume-then-container logic Redis uses is what
      runs the moment that field is populated
      (`TestController_Reconcile_Postgres_WithCredentials_Reconciles`
      proves the activation path against the fake runtime). No new
      migration or schema change was made or is needed for this: 1.8's
      Postgres support is fully blocked on 1.7, not partially done.

Live-verified, not just unit tested: real Docker, a real named volume
created via `EnsureVolume`, a real Redis container created with it
mounted, independently confirmed by inspecting the container's mounts
and the volume itself through the raw Docker Engine API
(`ContainerInspect`, `VolumeInspect`) rather than trusting this package's
own return values, then a second reconcile proving the no-op path doesn't
recreate anything. 86.1% coverage on `internal/reconcile/database`, 79.2%
on `internal/docker` (was 78.2% before this task).

### 1.9 HTTP API (`internal/api`). DONE (2026-08-11)

- [x] `/api/v1/brand` (CLAUDE.md section 3, the frontend's brand
      indirection depends on this existing). Serves `internal/brand`'s
      loaded `*Brand` as JSON, public, no auth, since a login screen
      needs branding before a session exists. `cmd/levelrail/main.go`
      now actually calls `brand.Load` (it never did before this), env
      override `APP_BRAND_FILE`, default `./brand.yaml`
- [x] Apps CRUD, layered directly on `store.DesiredService`
      (`internal/store/service.go`), the closest existing resource,
      rather than a new speculative domain type. `DeleteDesiredService`
      added to `internal/store` since CRUD's "D" didn't exist yet.
      Known gap, deliberately not papered over: `DesiredService` has no
      domains/replicas/strategy fields, so the API can't expose what
      app.yaml (`internal/spec`) can express but the store can't yet
      hold. That's a store-schema and deploy-pipeline change (1.3/1.4/
      1.5), not something invented here
- [x] Deploy trigger (`POST .../deploys`), deploy history
      (`GET .../deploys`). Trigger only updates `desired_services.image`,
      the same mechanism TASKS.md 1.3's rollback note describes ("pointing
      desired.Image back at an older tag and reconciling converges to it
      the same way any other redeploy does"), run forward instead of
      backward: it takes an already-built image tag as input rather than
      building one, since `internal/build`/`internal/deploy` (1.4) were
      owned by a different concurrent session while this was written.
      History returns the latest reconcile condition per
      (controller, condition type) pair via `store.GetConditions`, which
      is genuinely all `UpsertConditions` persists today, not a
      row-per-deploy-attempt log; no new log format was invented to fake
      one
- [x] Single admin user, session auth. No teams, no RBAC (explicitly
      out of scope until Phase 4). `internal/store` gained an
      `admin_user` table (migration 0003, single row enforced by
      `CHECK (id = 1)`) and bcrypt password hashing
      (`golang.org/x/crypto/bcrypt`, already an indirect dependency,
      promoted to direct). Sessions are a server-side in-memory map keyed
      by an opaque cookie token, deliberately nothing more elaborate
      (no JWT/OAuth) per CLAUDE.md 6 Phase 1's explicit scope.
      `cmd/levelrail/main.go` bootstraps the admin account from
      `APP_ADMIN_USERNAME`/`APP_ADMIN_PASSWORD` on startup, non-fatal if
      unset (the server still starts; auth-required routes just stay
      inaccessible until an operator sets them)

**Closed (2026-08-12), single-session per CLAUDE.md 8's rule on the
reconcile loop**: `cmd/levelrail/main.go` now wires `reconcile.Engine` to
a dynamic controller set instead of the Phase 0 hardcoded `nginxdemo`
controller. `reconcile.Engine` gained a `Source` (`SetSource`), a
function re-called at the start of every `ReconcileAll` pass that lists
`desired_services`/`desired_databases` from the store and returns one
`application.Controller` per service, one `database.Controller` per
database, and a single shared `ingress.Controller`, in addition to
whatever fixed controllers `NewEngine` was built with. Level-triggered by
construction like everything else here: nothing about which apps exist is
cached in the engine, it's re-derived from the store every pass, so a
deploy triggered through this API (or an app/database created or deleted)
takes effect on the very next reconcile with no restart. A `Source` error
is logged and skipped for that pass rather than treated as fatal, and
never blocks the fixed controller set from still running. Smoke-tested
end to end: binary starts with zero apps, the ingress controller
reconciles `Routed0Services` cleanly, Caddy starts and stops without
error on SIGTERM.

HTTP layer is stdlib `net/http.ServeMux` with Go 1.22+ pattern routing
(`"GET /api/v1/apps/{name}"`), not Chi: CLAUDE.md 4.1 says "do not pull
in a heavy framework," and the one thing a framework would earn its
weight on, auth middleware, is a plain `http.HandlerFunc`-wrapping
closure stdlib composes with directly.

Tested with a real temp-file SQLite store per handler (`httptest`
against `Router.Handler()`), not a mock, matching `internal/store`'s own
test convention: every handler covers its 200/201/204 path, its 401
(missing/invalid session), its 404 (unknown app), and at least one
invalid/half-succeeded-input 400 case verified not to have reached the
store. 75.9% coverage on `internal/api`, 77.7% on `internal/store`
(up from before this pass, `DeleteDesiredService` and the admin_user
table both gained direct store-level tests too). 83.4% on `internal/`
overall, gate is 70%.

### 1.10 Frontend (`/web`). DONE (2026-08-12)

- [x] Vite + React 19 + TypeScript + Tailwind scaffold, TanStack
      Router/Query wired in (`web/src/main.tsx`, `web/src/routes/__root.tsx`).
      Predates this pass; not touched here beyond what's listed below.
- [x] **Embedding into the Go binary via `embed.FS`** (CLAUDE.md 4.1/4.12),
      package `web` (`web/handler.go`, `embed_real.go`, `embed_stub.go`).
      `DistFS` sits behind a `-tags embedweb` build tag rather than a
      bare `//go:embed dist`: `web/dist` is gitignored and doesn't exist
      until `npm run build` runs, and a bare directive would break plain
      `go build ./...` repo-wide on any checkout without a frontend
      build, including every quick sanity command both sessions' git
      hooks and CI rely on constantly. Default build embeds nothing and
      serves an actionable 501 instead of a bare 404 (this needed an
      explicit `index.html` existence probe, not just trusting
      `fs.Sub`'s error return, which is lazy and doesn't fail until
      first file access, caught by the package's own test). SPA
      fallback (any unmatched path serves `index.html` so a hard
      refresh on a client-side route like `/apps/foo` resolves
      correctly) is what `handler.go` actually does. `main.go`'s
      `rootHandler` mounts the 1.9 API at `/api/` (subtree, full path
      passed through unchanged) and this at `/`, one process per
      CLAUDE.md 4.1's single-binary story. 100% coverage on the new
      package; live-verified end to end with a real `npm run build`
      and a real `-tags embedweb` binary, not just unit tested: `/`,
      a client-side route, a real static asset, and `/api/v1/brand`
      all curled through the same running server, all correct.
      **Bug fixed after this landed**: `handler_test.go`'s
      `TestHandler_UsesPackageDistFS` hardcoded the stub build's 501
      assertion, so `go test -tags embedweb ./web/...` failed the moment
      `DistFS` actually held the real embedded frontend instead of the
      stub's empty zero value, exactly the build mode the test claimed
      to be pinning. Split into `handler_stub_test.go` (`!embedweb`,
      asserts 501) and `handler_embedweb_test.go` (`embedweb`, asserts a
      real 200 with the built `index.html` shell in the body), so both
      build tags are actually exercised rather than one silently never
      running its own assertion in CI. Still 100% coverage on `web`.
- [x] App list (`web/src/routes/apps/index.tsx`,
      `queries/apps.ts`'s `fetchApps`/`appListQueryOptions`). **Contract
      mismatch closed (2026-08-12)**: the list route originally assumed a
      `{ items, nextCursor }` page of `AppSummary` rows (`id`, `status`,
      `primaryDomain`, `lastDeployAt`, `lastDeployStatus`) with cursor
      pagination, built before 1.9 landed and never reconciled against
      it. The real, frozen `GET /api/v1/apps` returns a bare
      `[]appResource` array, the identical shape the detail endpoint
      returns, no cursor, no status, no id. Fixed by deleting the
      speculative `AppSummary`/`AppPage` types (`types/app.ts` is gone,
      the list and detail routes now share `types/appDetail.ts`'s single
      `AppDetail` type), switching the list query from
      `useSuspenseInfiniteQuery` to a plain `useSuspenseQuery` (nothing
      server-side to page against), and dropping `AppRow`'s status badge
      rather than faking one: appResource has no status field, and
      deriving one would mean an extra `GET .../deploys` per row, an N+1
      fetch for a list CLAUDE.md 7 requires to stay virtualized and cheap
      at 50+ rows. Verified against the real binary, not just
      typechecked: started it, created an app via `POST /api/v1/apps`,
      confirmed `GET /api/v1/apps` returns exactly the flat-array shape
      the fixed client now expects.
- [x] **App detail (`web/src/routes/apps/$name.tsx`), this pass.** Built
      against the real, frozen `internal/api` contract directly (not the
      list route's mismatched assumptions above):
      - View: `AppOverview.tsx` renders name, image, port, resource
        limits, and readiness/liveness probe config from
        `GET /api/v1/apps/{name}`, formatted by new `lib/format.ts`
        helpers (`internal/store.ServiceResources`/`ServiceProbe`'s raw
        bytes/nano-CPUs/nanosecond units, read-only, no edit affordance).
      - Status: `ConditionsPanel.tsx` renders
        `GET /api/v1/apps/{name}/deploys`. Deliberately worded and typed
        as current reconcile *status*, not a history log, matching
        `internal/api/deploys.go`'s `handleDeployHistory` doc comment
        (`internal/store.UpsertConditions` only ever keeps the latest
        condition per type). `types/deploy.ts`'s `ReconcileCondition`
        intentionally keeps `internal/reconcile.Condition`'s capitalized
        field names (`Type`, `Status`, `Reason`, `Message`,
        `LastTransitionTime`) since that Go struct carries no `json`
        tags and encoding/json falls back to the exported name verbatim.
      - Domains: `DomainEditor.tsx`, add/remove list, React Hook Form +
        Zod (per `docs-local/research/frontend-plan.md` section 2),
        `useFieldArray` with basic domain-shape validation, saved via
        `PUT /api/v1/apps/{name}` sending the full `AppDetail` back
        (`handleUpdateApp`'s full-replace semantics, not a patch).
      - Env vars: `EnvEditor.tsx`, same add/remove-list and full-replace
        PUT pattern, key/value pairs bound to `AppDetail.env`, duplicate-key
        validation via a Zod `superRefine`. Explicitly plain string
        values only, both in the help copy shown in the UI and in the
        package comment: `internal/deploy.requireNoUnresolvedEnv`
        (TASKS.md 1.7) still rejects `{ secret: true }`/`{ from: ... }`
        outright, and `store.DesiredService.Env` is a flat
        `map[string]string` with no room to represent a secret
        reference. Nothing here implies secret support exists.
      - Deploy trigger: `DeployTriggerForm.tsx`, a single image-tag
        field, `POST /api/v1/apps/{name}/deploys`. No log viewer, no
        progress bar, no "deploying..." polling beyond the mutation's
        own pending state: `handleTriggerDeploy` only repoints
        `desired_services.image` at a new tag, and there is no backend
        SSE endpoint to stream against, so nothing here pretends one
        exists.
      - `DomainEditor`/`EnvEditor` both use RHF's `values` +
        `resetOptions: { keepDirtyValues: true }` instead of a manual
        `useEffect`/`reset`, specifically because they render side by
        side against the same `AppDetail` prop: a successful save in one
        editor must not silently clobber unsaved edits sitting in the
        other when the shared query cache updates.
      - `AppRow.tsx` (list row) swapped from a plain `div` to a
        `<Link to="/apps/$name" params={{ name: app.name }}>`, per its
        own comment inviting exactly this once the detail route existed.
        Links by `name`, the only identifier `appResource` actually has,
        not `AppSummary.id`.
- [x] **Live build logs (`web/src/routes/apps/$name/deploys/$deployId/logs.tsx`),
      this pass.** Client half only, per `docs-local/research/frontend-plan.md`
      section 2: `useDeployLogStream` wraps native `EventSource` (no
      hand-rolled reconnect), an 8,000-line ring buffer, and a
      pause/resume flag the route's auto-scroll consumes without ever
      stopping ingestion. Rendered through TanStack Virtual, fixed-height
      monospace rows, ANSI escape codes stripped (`lib/ansi.ts`, full
      ANSI-to-HTML color rendering left as a stretch goal). The most
      isolated route in the app per CLAUDE.md 4.12: confirmed via
      `npm run build`'s per-chunk output that the SSE hook, the ANSI
      stripper, and this route's own `useVirtualizer` instance appear
      only in its own chunk, never the shared or app-list bundles.
      **Still not wired to anything real**: `internal/api` (1.9) has no
      log-streaming endpoint, so this was built against a documented,
      unconfirmed contract (`GET /api/v1/apps/{name}/deploys/{deployId}/logs`,
      SSE, each `data:` payload a bare line or `{line, stream}` JSON),
      written directly into `queries/deployLogs.ts` and the hook's own
      comment. Follow-up, unchanged from before: add the SSE endpoint
      (likely surfacing `internal/build`'s already-decoupled
      `build.ProgressEvent`/`SlogProgress`, see TASKS.md 1.4) and confirm
      the contract matches. Not yet linked from the app detail page
      (`DeployTriggerForm.tsx` was under active concurrent work at the
      time this landed); reachable directly by route today.
- [x] Route-level code splitting: confirmed by build output, not just
      configured. `npm run build` produces a separate chunk for the
      `$name` route (`_name-*.js`, ~107 KB) from the `apps` list route
      (`apps-*.js`, ~27 KB) and the shared `index-*.js` bundle, i.e. a
      session that never opens an app's detail page doesn't pay for
      React Hook Form, Zod, or the editor components.

No test runner is configured for `/web` yet (no `test` script in
`package.json`, no Vitest/Testing Library installed), and that predates
this pass too. Verification here is what the scaffold's own list route
already established as the bar: `npm run typecheck`, `npm run lint`,
`npm run format:check`, and `npm run build`, all clean, plus a manual
`grep -rlP '[\x{2014}\x{2013}]' web/src` confirming no em/en dashes
anywhere in `/web/src`.

### 1.11 Whole-chain E2E proof (`test/e2e`, `test/fixtures`). DONE (2026-08-12)

Every piece landed under 1.1-1.9 had its own live test against real
Docker/BuildKit/Caddy, but nothing before this had run the whole chain in
one process: build, save desired state, converge a container, converge
routing, and get back real traffic. `cmd/levelrail/main.go`'s
`dynamicSource` (closed under 1.9) is what actually wires
`reconcile.Engine` to one `application.Controller` per service, one
`database.Controller` per database, and a shared `ingress.Controller`;
this task is the first test to exercise that same three-controller shape
end to end rather than trusting that each piece's isolated live test
implies they compose.

- [x] `test/fixtures/hello-e2e/Dockerfile`: a real HTTP-serving fixture,
      unlike `internal/deploy/testdata/Dockerfile` (deliberately listens
      on nothing, no probe possible). `busybox:1.36`'s built-in `httpd`
      applet serving a static `index.html` whose body is the distinctive,
      greppable string `"hello from levelrail e2e"`, so a passing
      assertion proves a specific response reached this fixture, not just
      that some 200 came back from somewhere. Small image, no compile
      step, fast to build and pull in CI
- [x] `test/e2e/deploy_test.go`
      (`TestDeploy_Live_BuildToHTTPS`): live-only, skips cleanly (not a
      failure) if Docker or BuildKit aren't reachable, the exact pattern
      `internal/deploy/deploy_live_test.go` and
      `internal/reconcile/ingress/controller_live_test.go` already use.
      Real `internal/build.Client` builds the fixture (called directly
      rather than through `internal/deploy.Pipeline`, since this test
      needs to set `Domains` and `Health.Readiness` on the saved
      `store.DesiredService` directly, fields `internal/deploy`'s
      `Request`/`translate.go` don't currently expose a path to from a
      bare build with no `app.yaml`). Saves desired state in a real
      temp-file SQLite store with domain `e2e-test.levelrail.internal`
      (matching the internal-hostname convention
      `controller_live_test.go` already established) and a readiness
      probe at `/`. A real `application.Controller` converges the
      container; a real `ingress.Controller`, constructed the same way
      `controller_live_test.go` does (real `ingress.Driver`, ephemeral
      Caddy/admin ports via `t.TempDir()`-backed storage), converges
      routing. A real HTTPS request through Caddy
      (`InsecureSkipVerify: true`, `//nolint:gosec`, same justification
      comment as `controller_live_test.go`: Caddy's internal issuer cert
      is never installed into this process's trust store) asserts the
      fixture's exact response body comes back, not just a 200
- [x] Independent verification, not trusting return values: after the
      application controller's `Reconcile`, the container is directly
      inspected via `runtime.ListByPrefix`/`InspectByName` (confirming
      exactly one container, running, tagged with the image `Build()`
      produced) before the ingress and HTTP steps run at all, the same
      rigor `deploy_live_test.go` applies
- [x] Cleanup in `t.Cleanup`, matching every other live test's
      convention here: container removed by service-name prefix, image
      removed by exact tag, Caddy driver stopped. Registered before the
      build even runs (not after a successful one), so a failure partway
      through still leaves nothing behind on a re-run. Verified by
      running the test twice in a row and by inspecting Docker directly
      afterward: no leftover container or image either time

What this deliberately does not prove, left as real, open gaps rather
than implied coverage:

- No rollback exercised. The mechanism (pointing `desired.Image` at an
  older tag and reconciling) is already proven by 1.3's own live test;
  this task never deploys a second image over the first
- No webhook/git-push path exercised. This test calls
  `internal/build.Client.Build` directly; `internal/webhook`'s HMAC
  verification, branch gating, and `go-git` fetch (1.5) have their own
  live test and are not re-exercised here
- Single service only. Nothing here proves two services' ingress routes
  coexist correctly; `internal/reconcile/ingress`'s own live test
  (`controller_live_test.go`) is what actually proves that, with two
  real backends and two real domains
- No multi-node. Everything runs in one process against one local Docker
  daemon, matching every other live test in this repo and Phase 1's own
  scope (CLAUDE.md 6: multi-server is explicitly Phase 3)
- Real ACME is still unverified. Like `internal/reconcile/ingress`, this
  test only exercises Caddy's internal (self-signed) issuer; the
  Phase 1 exit criterion's "valid cert" against a real public domain
  remains the open gap noted under 1.6

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
