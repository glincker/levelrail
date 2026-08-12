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

### 1.1 App spec (`internal/spec`)

- [~] Go structs for the app.yaml shape (CLAUDE.md 4.9)
- [ ] Hand-written JSON Schema, embedded, validated at parse time
- [ ] Candidate-filename discovery (`app.yaml`, `deploy.yaml`, plus the
      branded name from `brand.yaml`, per CLAUDE.md section 3)
- [ ] `EnvVar` union type: literal string, `{ from: ... }` reference, or
      `{ secret: true, required: true }`
- [ ] Semantic validation beyond JSON Schema's reach (e.g. `build.path`
      required for `dockerfile`/`compose` but not `static`)

### 1.2 Docker wrapper extensions (`internal/docker`)

- [ ] Extend `ContainerSpec`: ports, env vars, resource limits (memory/cpu),
      restart policy
- [ ] Health check wiring (readiness/liveness from the app spec down to
      Docker's own `HEALTHCHECK` or an internal prober, decide which)
- [ ] Image tagging by commit SHA (rollback depends on this existing
      before rollback itself is built, not after)

### 1.3 Application controller (`internal/reconcile`)

- [ ] New `Controller`: desired state read from `internal/store` (not
      hardcoded, unlike `nginxdemo`), one controller instance per declared
      service
- [ ] Readiness/liveness probing against the app spec's `health` block
- [ ] Rolling deploy: start new, wait for readiness, cut traffic, drain
      old (blue-green as the default per CLAUDE.md 6, "easier to get
      right than rolling with a single replica")
- [ ] Rollback: pin the previous N images so garbage collection can't
      orphan a rollback target (CLAUDE.md 6 calls this out as "a real bug
      in competitors", confirmed independently in Phase 0's Coolify
      research)

### 1.4 Build integration

- [ ] Wire `internal/build` (currently a standalone spike) into the
      deploy flow: app spec's `build.type` selects the frontend
- [ ] Remote cache (`SolveOpt.CacheImports`/`CacheExports`, untouched by
      the Phase 0 spike per its own findings)
- [ ] Build log streaming over SSE (BuildKit's own
      `Vertex`/`VertexStatus`/`VertexLog` progress channel already
      carries what's needed, per `docs-local/research/buildkit-spike.md`)

### 1.5 Git integration

- [ ] Webhook receiver
- [ ] Deploy keys or GitHub App auth (decide which; deploy keys are
      simpler, GitHub App is the better long-term UX)
- [ ] Commit SHA tracking through to the image tag (depends on 1.2's
      SHA-tagging)

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
