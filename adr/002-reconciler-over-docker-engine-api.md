# ADR 002: Custom reconciler over the Docker Engine API

Status: Accepted
Date: 2026-08-11

## Context

Levelrail needs to keep running containers converged with declared desired
state: start what's missing, restart what died, replace what's misconfigured.
Every competitor studied in Phase 0 (see `docs-local/research/prior-art-*.md`)
solves this differently, and none of them solve it the way this ADR settles
on.

## Decision

A custom reconcile loop, modeled on Kubernetes controllers, running directly
against the Docker Engine API (never the `docker` CLI, never SSH-and-shell).
Controllers are idempotent, level-triggered, and safe to interrupt: every
`Reconcile` call re-derives what to do from current observed state rather
than assuming a previous call completed. Observed state comes primarily from
Docker's event stream, not polling, with a periodic resync as a safety net
for events the stream might have missed.

Implemented in `internal/reconcile` (the generic engine and `Controller`
interface) and `internal/docker` (the Engine API wrapper, exposed as a
narrow `Runtime` interface controllers depend on rather than the full SDK
client, so tests can fake it without a daemon).

## Rejected alternatives

- **Docker Swarm**: in maintenance mode, and Dokploy's own research
  (`prior-art-dokploy.md`) shows the coupling to Swarm-specific update
  semantics becomes a liability, not a convenience.
- **Nomad**: BSL license.
- **K3s / Kubernetes**: defeats the point, brings back the whole surface
  area this project exists to avoid.
- **SSH + shelled `docker` CLI commands** (Coolify v4, CapRover, Dokku):
  every one of these was independently confirmed by Phase 0 research to
  force either continuous polling (Coolify: `ServerManagerJob` every
  minute, `docker container inspect` over SSH per registered server,
  confirmed at `app/Jobs/ServerManagerJob.php`) or accept real gaps (CapRover:
  zero per-app health check confirmed by grep; Dokku: no daemon at all,
  paid for with two admitted hacks per its own source comments).
- **No persistent agent, pure SSH one-shot commands** (Kamal): genuinely
  zero idle cost, but Kamal's own highest-comment GitHub issue (76 comments,
  `Net::SSH::Disconnect`) is a pure transport-layer failure with nothing to
  do with deploy logic, exactly the failure class a persistent, reverse-dialed
  connection (ADR 003) removes.

## Consequences

- Every reconcile has to be written defensively: a controller cannot assume
  the container it's about to create doesn't already exist, or that an
  in-flight operation from a moment ago finished. `internal/reconcile/
  nginxdemo`'s test suite includes an explicit test for the half-succeeded
  case (container created, start failed), matching the project's testing standard that every reconciler must have a test for the operation-half-succeeded case. That discipline
  has to hold for every future controller, not just this one.
- No controller may shell out to `docker` or SSH into anything. The
  `docker.Runtime` interface is the only way `internal/reconcile` is allowed
  to touch container state, and it's implemented entirely over the Engine
  API's HTTP/socket transport.
- The event-stream-primary, ticker-as-safety-net shape (`Engine.Run` in
  `internal/reconcile/engine.go`) is not a novel design: it's independently
  what Coolify's own in-progress v5 rewrite converges on
  (`app/Jobs/V5ReconcileServerStateJob.php`, per `prior-art-coolify.md`),
  which is reassuring rather than concerning: two independent efforts
  arriving at the same shape from different starting points is a decent
  signal it's the right shape, not just a Levelrail idiosyncrasy.

## Verified

Phase 0 exit demo run live 2026-08-11: `internal/reconcile/nginxdemo`
converges a single hardcoded `nginx:alpine` container. Killed by hand via
`docker kill`; the engine's event-driven trigger caught the `die` event and
restarted it within 2 seconds, logged as `Ready.reason=Restarted`. Clean
shutdown on SIGTERM confirmed (`context canceled` treated as normal exit,
not an error). Full test suite (`internal/brand`, `internal/reconcile`,
`internal/reconcile/nginxdemo`) passes, including the half-succeeded-create
case.
