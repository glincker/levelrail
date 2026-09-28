# ADR 002: Custom reconciler over the Docker Engine API

Status: Accepted
Date: 2026-08-11

## Context

Levelrail needs to keep running containers converged with declared desired
state: start what's missing, restart what died, replace what's misconfigured.
Every competitor studied in Phase 0 solves this differently, and none of
them solve it the way this ADR settles on.

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

- **Docker Swarm**: in maintenance mode, and Dokploy's own source shows the
  coupling to Swarm-specific update semantics becomes a liability, not a
  convenience.
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
  (`app/Jobs/V5ReconcileServerStateJob.php`, Coolify's own reconcile job),
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

## Addendum (2026-09-25): content identity, ordering and release hold

Container identity used to hash the image string, so a redeploy of a
floating tag (`latest`, `main`) found its container already running and
reported success while old content served (the Dokploy 5496 class).
Desired state now carries content identity instead:

- Registry images are pinned at deploy time to the digest the registry
  reports (`repo:tag@sha256:...`, the manifest list for multi-arch). The
  pinned string is `DesiredService.Image`, so container names, ingress
  lookups, rollback history and pulls on any node all follow content with
  no second identity field to keep in sync. A registry failure falls back
  to the locally cached digest and says so on the deploy
  (`PullFailedUsingCached`); `pull: true` makes it fail instead.
- Local builds, which have no registry digest, record the built image ID
  (`image_id`, valid only while `image_id_ref` equals `image`) and fold it
  into the container name hash, so a rebuild of the same tag cuts over.
- The application controller compares the running container's image ID
  with what the desired reference resolves to on its node and never
  reports Ready on a mismatch (`ImageDigestMismatch`). Unpinned legacy tags
  are not verified, to avoid flagging apps nobody redeployed.

Rejected: a separate digest column feeding the name hash (every
`ContainerName` caller would need both fields, and rollback history would
need both); always re-pulling in the reconciler (turns every resync into a
registry call and makes content change without a deploy); comparing tags
(the bug itself).

Automatic deploys carry a per-service sequence plus provider commit
ordering, checked in the same SQLite transaction as the desired-state
write, so an older push or a slower build cannot overwrite a newer deploy.
Manual deploys and rollbacks are exempt but advance the sequence. Freeze
windows hold automatic deploys as `held` attempts that a releaser replays
when the window ends. After a blue-green or rolling cutover the previous
release is kept for `APP_DEPLOY_PREVIOUS_RELEASE_HOLD`, derived from
container creation times so the decision stays level-triggered and survives
a control-plane restart. See docs/deploy-safety.md.
