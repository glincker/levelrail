# ADR 001: Go, single static binary for the control plane

Status: Accepted
Date: 2026-08-11

## Context

Levelrail's whole pitch (CLAUDE.md section 1) is architectural, not cosmetic:
low idle footprint that stays low as managed apps grow, and a wedge against
Coolify and Dokploy that are, respectively, a Laravel monolith with bolted-on
services and a TypeScript monorepo with a separate metrics binary. The
distribution story has to match that pitch. If the control plane ships as a
pile of processes that need `docker compose up` to run, the "lightweight
alternative" positioning (section 1, "Positioning shorthand") is undercut
before a single feature is built.

Phase 0 research into every competitor studied (`docs-local/research/prior-art-*.md`)
confirms this isn't a hypothetical trade-off, it's the exact failure mode of
the category.

## Decision

Control plane is a single static Go binary. Frontend assets are embedded via
`embed.FS`, built once at CI time and baked into the binary, so there is no
Node runtime on the server and no separate frontend deploy step. HTTP routing
uses `net/http`'s Go 1.22+ enhanced `ServeMux` (method-aware, wildcard-capable
routing patterns) or Chi as a thin wrapper over the same `http.Handler`
interface. No heavy web framework.

This decision only makes sense combined with 4.4 and 4.5: BuildKit
(`moby/buildkit`) and Caddy (`caddyserver/caddy/v2`) are both imported as Go
libraries directly into this binary, not run as sibling processes. The
single-binary decision and the "embed BuildKit and Caddy as libraries"
decisions are the same architectural bet made three times.

## Rejected alternatives

- **Multi-process architecture, the pattern every competitor studied uses.**
  - **Coolify v4**: Laravel + Livewire frontend, MySQL/Postgres, Redis,
    Horizon queue workers, Soketi websockets, and a separately orchestrated
    Traefik container (`prior-art-coolify.md`, stack line). Idle cost isn't
    theoretical: with zero apps deployed, `ServerManagerJob` runs every
    minute against every registered server (`app/Console/Kernel.php:38-99`),
    plus an hourly `DockerCleanupJob` per server, plus a dedicated hourly
    `CleanupStaleMultiplexedConnections` job that exists purely to garbage
    collect SSH connection-multiplexing sockets left over from the
    architecture's own transport choice.
  - **Coolify's in-progress v5 rewrite doesn't collapse this, it changes the
    shape of it.** Per `prior-art-coolify.md` "Implications for Levelrail"
    point 2: V5 is still Laravel + Postgres/Redis/Horizon/Soketi, plus a
    separately-orchestrated Caddy container the control plane generates
    Compose YAML and a Caddyfile for (`GenerateCaddyIngressConfiguration::handle()`),
    plus a Rust agent (`coold`) and a Rust hub (`Flux`) talking gRPC. That is
    a more complex multi-process, multi-language system than v4, not a
    simpler one. Levelrail's single-binary claim survives V5 fully intact
    and is a strictly tighter architecture than even Coolify's own rewrite.
  - **Dokploy**: a TypeScript/Node monorepo, `apps/dokploy` (Next.js + tRPC)
    driving `packages/server` (the actual orchestration logic), plus
    `apps/monitoring`, a wholly separate Go binary just for metrics polling
    (`prior-art-dokploy.md` section 1 and Q5). Node is the primary server
    runtime for the control plane itself, and the metrics function had to be
    pulled into its own binary because the Node process wasn't the right
    place for it. Levelrail folds observability into the same binary from
    the start (4.8) instead of bolting it on as a second deployable later.
  - **CapRover**: Node.js control plane (`captain-captain`) plus a separate
    `captain-nginx` Swarm service, plus a persistent "sleeping" certbot
    container it `docker exec`s into for cert operations, plus Docker
    Swarm's own Raft-backed control-plane overhead running even on a
    single node (`prior-art-caprover.md` section 5, `DockerApi.initSwarm()`
    called unconditionally at `src/docker/DockerApi.ts:157`). CapRover's own
    research doc concludes the bigger cost of this shape isn't raw idle
    RAM, it's the complexity of keeping N independently-versioned processes
    in sync, exactly the argument for a single binary.
- **Rust.** This is the honest counterpoint, not a strawman: Coolify's own
  v5 rewrite chose Rust for both new components, `coold` (the agent) and
  `Flux` (the hub), which is a live, current signal from a direct competitor
  that a from-scratch rewrite in this exact problem space didn't default to
  Go. Rust would give a stronger static memory-safety guarantee and a
  comparably trivial single-binary distribution story. It's rejected here
  for a narrower, more specific reason than "Go is fine too": 4.4 and 4.5
  aren't just "use BuildKit" and "use Caddy," they're "import BuildKit's and
  Caddy's own Go client libraries directly into this process and drive them
  through their native Go APIs." `moby/buildkit` and `caddyserver/caddy/v2`
  are both Go-native projects with no first-class Rust binding; embedding
  them into a Rust binary would mean FFI, a subprocess, or reimplementing
  their control surfaces, all of which reintroduce exactly the multi-process
  or wrapper-shim complexity this whole ADR set exists to avoid. Go is the
  language that lets 4.1, 4.4, and 4.5 all be true simultaneously with real
  library imports, not bindings.
- **A heavy web framework** (the kind that bundles ORM, templating, session
  management, and a plugin ecosystem). Go 1.22 folded method-aware and
  wildcard routing directly into stdlib `net/http`, which was historically
  the main reason projects reached for a framework. What remains, thin
  middleware chaining and clean route grouping, is exactly Chi's whole
  scope: it's a router built on `http.Handler`, not a framework with its own
  request lifecycle. Pulling in something heavier adds a dependency surface
  and an opinionated request/response model without moving the needle on
  the single-binary goal, since a framework doesn't reduce what has to be
  compiled in, it just adds to it.

## Consequences

- Deployment is "copy one binary," not "bring up a Compose stack." This is
  the literal mechanism behind the "AI-ready lightweight Coolify
  alternative" positioning (section 1), not just marketing language.
- Frontend and backend release cycles are coupled by construction: because
  `embed.FS` reads assets at compile time, a frontend-only fix requires a
  full binary rebuild and release, not a static-asset redeploy. This is a
  deliberate trade, it keeps "one binary, one version" true rather than
  letting frontend/backend version skew creep in the way it did for
  Dokploy's monorepo-but-still-two-deployables structure.
- CI must build the frontend (`/web`) before the Go build step, and the Go
  build is only reproducible if the embedded frontend build artifact is
  reproducible too. This is a real ordering dependency to get right in CI,
  not a footnote.
- Cross-compilation stays simple because the project standardizes on
  CGO-free dependencies wherever the choice exists (see 4.7's
  `modernc.org/sqlite`), so `CGO_ENABLED=0` builds keep working as the
  binary picks up more embedded libraries.
- Choosing Go here isn't independent of 4.4 and 4.5: BuildKit and Caddy are
  both Go projects. That the language decision and the two biggest library
  dependencies line up isn't a coincidence, it's the same bet made three
  times, and it's worth remembering that if either of those dependencies
  ever forced a non-Go rewrite, this ADR would need to be revisited too.
