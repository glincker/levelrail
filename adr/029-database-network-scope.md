# ADR 029: Database network scope is enforced at attachment

Status: Accepted

Date: 2026-10-09

## Context

Operators asked for RDS style network controls on managed databases, including
"VPC locking": restrict a database to the apps of one project or environment.

How networking works today:

- Each app gets its own Docker network, `<prefix>-app-<appID>`.
- A database container sits on Docker's default bridge. The application
  controller attaches the database container to the network of every app that
  references it (`connectReferencedDatabases`), so the app resolves it by name.
  Nothing else gets a name based route to it.
- When a mesh zone is configured, apps use routed DNS names and no attachment
  happens (ADR 006).

So a database is already reachable by name only from apps that reference it.
What was missing was a way to say "and only from my project", with a preview of
what that would cut off.

## Decision

Add a per database `network_scope`: `platform` (today's behaviour, the default),
`project` or `environment`. It lives in `database_access_settings` (migration
0402), not on `desired_databases`, so the hot scan path is untouched.

- **Enforcement is level triggered, in the app reconcile.** Each pass, for every
  referenced database, the controller asks whether the app is inside the scope.
  If not, it detaches the app's network from the database container and fails
  that pass with `ErrDatabaseOutOfScope`, so the app's condition names the
  reason. It never connects an out of scope app.
- **Never silent.** `PUT .../network/scope` with `dry_run` lists every
  referencing app and whether it keeps access. Applying a scope that cuts any
  app off needs `confirm`. The reachability map and the app condition both
  say why.
- **Reversible.** Setting the scope back to `platform` stops the detaching and
  the next pass reattaches the networks. No data or container changes.
- **Placement is required.** `project` needs the database to be in a project,
  `environment` needs an environment, otherwise the request is refused with 409
  instead of scoping to nothing.

## Not done, and why

- **Moving the database off the default bridge.** A container on the default
  bridge can still be reached by its bridge address from other containers on
  that bridge. Removing it means a dedicated `<prefix>-db-<name>` network and
  a container replace, which restarts the database and breaks anything that
  reaches it by bridge address (telemetry targets, ad hoc tools). That is a
  separate, explicit "isolate" step with its own preview. The reachability map
  states the caveat plainly (`bridge_reachable`) rather than implying isolation
  it does not provide.
- **Mesh (multi node) enforcement.** With a mesh zone the controller skips
  attachment entirely and apps use routed names, so scope is not enforced there
  yet. It needs the same check in the mesh DNS answer path.

## Rejected alternatives

- **Per database iptables rules.** A second firewall implementation on top of
  the exposure manager, host only, and fragile against Docker's own chains.
  Published port restrictions already go through the exposure manager.
- **One shared Docker network per project.** Breaks per app isolation, forces
  renaming existing networks and still cannot express environment scope.
- **Validate only when an app is attached through the API.** Misses apps moved
  between projects later and apps deployed from `app.yaml`, so the controller is
  the only place that sees every case.
- **Create time only.** Would leave existing databases unscopable. The
  attachment based design works for new and existing databases alike.

## Consequences

- An out of scope app keeps running but cannot reach the database, and its
  reconcile reports `database is out of network scope for this service`.
- Existing established TCP connections from the detached app drop when the
  network is detached. The dry run exists so this is a decision, not a surprise.
- Temporary credentials, users and allowed sources are independent of scope.
