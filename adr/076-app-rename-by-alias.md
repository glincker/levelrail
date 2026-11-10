# 076. App rename by alias

Status: proposed (needs owner acceptance)

Date: 2026-11-08

## Context

An app cannot be renamed today. The only path is to deploy a new app and delete the old one, which loses history, secrets, domains, backups and metrics. The app name is more than a label:

- `apps.id` and `apps.name` exist (`internal/store/migrations/0039_apps.sql`), but the backfill set `id` equal to the first service name, and about 24 later tables key on `service_name` (for example `service_secrets`, `service_secret_values` in `internal/store/secret.go`, `ci_oidc_trusts`, `branch_deploy`, `split_tests`). The API path `/api/v1/apps/{name}` is that same key.
- Docker volume names are `app-<service>-<volume>` (`store.ServiceVolumeName`, `internal/store/service_query.go`), and ownership labels carry the app and service (`internal/docker/volume_ownership.go`).
- Containers are matched by an `<app>-` name prefix (`hiddenContainerPrefixes` in `internal/api/containers.go`, `deriveAppNameFromContainer` in `internal/api/containers_orphaned.go`, `resolveResourceID` in `internal/alerting/crashloop.go`).
- The auto domain is derived from the name (`ingress.FallbackDomainFor`, `dnsForAutoDomain` in `internal/api/apps_base_domain.go`).
- Telemetry writes samples and logs under `resourceIDForApp(name)` and the Loki shim filters on the `app` label (`internal/api/metrics.go`, `internal/telemetry/logql.go`). A rename that does not keep these in step orphans history.

A rewrite of every `service_name` column, every volume and every container would be a long, risky, many-table transaction, and Docker volumes cannot be renamed at all (data would need a copy).

## Decision

1. The internal key never changes. The service name an app was created with stays its immutable internal key in every table, volume name, container name and telemetry resource ID. Rename changes only the public name.
2. The public name is `apps.name`, the one already unique. A new table `app_name_aliases (alias TEXT PRIMARY KEY, app_id TEXT NOT NULL REFERENCES apps(id) ON DELETE CASCADE, expires_at TEXT)` holds retired names. The primary key on `alias` is the only index.
3. `PATCH /api/v1/apps/{name}` accepts a new `name`. The old public name becomes an alias with `expires_at` set from a runtime setting (`apps.rename_alias_days`, applies live, env only seeds the default, 0 means no alias). While an alias is live, no other app can take that name, and the alias resolves to the app.
4. Resolution happens once at the API edge. A router step maps `{name}` to the internal key: current public name first, then a live alias. The internal key is never matched on its own, so after a first rename the old name (which is also the internal key) resolves only as an alias, gets the `Deprecation` header and stops once the alias is gone or the window is 0. Before any rename the public name equals the internal key, so nothing changes. Handlers keep using the internal key, so no handler, reconciler or store query changes. A request through an alias answers normally and adds a `Deprecation` header naming the new name.
5. Rename is refused while a deploy or an agent operation is running on the app, for retained deploy copies, and for a name that is invalid, taken, or held as a live alias of another app. It needs the app write ability. A new public name that matches another app's internal key is refused too.
6. One shared name-availability check covers current public names, live aliases and every internal key (including internal keys whose app has since been renamed). Every path that creates or renames an app calls it: API create, deploy upsert, stack and IaC apply, MCP create tools and rename. An internal key stays reserved for the life of its app, even after its alias lapses, because tables, volumes and containers still carry it.
7. Default domain: the auto domain follows the public name on the next reconcile. The old auto hostname stays served for the alias window as a redirect (308) to the new one, then stops. Custom domains are untouched because they are keyed on the domain, not the name. DNS records created for the old auto hostname are removed when the alias expires, by the same cleanup that runs when a domain is removed.
8. Secrets, volumes, labels, backups, telemetry, alerts and CI trusts are untouched because they use the internal key. Dashboard, CLI (`app rename <old> <new>`) and API show the public name everywhere and the internal key only in `app show --json` as `internalName`.
9. Audit: a rename writes `app.rename` with actor, app ID, old name, new name and alias expiry. Audit history stays queryable under both names, because audit rows store the app ID and the name at the time.
10. An expiry sweep deletes lapsed aliases. It runs inside an existing periodic maintenance loop, not a new goroutine, and deletes by `expires_at` in one statement.

## Touchpoints

| Area | Where | Effect of rename |
| --- | --- | --- |
| Reconciler | `internal/reconcile/application` | None, keyed on the internal key |
| Route and ingress | `ingress.FallbackDomainFor`, `dnsForAutoDomain` | Auto host follows the public name, old host redirects |
| Volume | `store.ServiceVolumeName`, `volume_ownership.go` labels | None |
| Containers | prefix matches in `containers.go`, `containers_orphaned.go`, `alerting/crashloop.go` | None |
| Secrets | `internal/store/secret.go` | None |
| Telemetry | `resourceIDForApp`, `telemetry/logql.go` | None, history stays attached. The `app` log label stays the internal key |
| API | `PATCH /api/v1/apps/{name}`, router resolver | New edge step |
| Audit | `internal/api/audit.go` | New `app.rename` event |
| CLI | `internal/clitree/tree.go`, `docs/cli-reference.md` | New `app rename` |

## Migration and rollback

One additive migration (next free number after a fetch, renumbered on conflict) creates `app_name_aliases`. No existing row changes, so no backfill. Rollback of a single rename is another rename back, which the owner of the old alias can always claim. Rollback of the feature is dropping the table; apps keep working because the internal key never moved.

## Cost

One small table, one lookup per request on the already unique `apps.name` (a primary-key lookup on `app_name_aliases` only when that misses, which is the rare alias case). No per-object index beyond the alias primary key, no new goroutine, no change to reconciler or telemetry write paths. Memory is bounded by the number of live aliases, and an alias is capped by the setting and by a maximum count per app, also a runtime setting.

## Consequences

- The internal key can differ from the public name after a rename. Docker volume names and container names keep the old name. This is shown in `app show` so it is not a surprise, and `docker ps` users see the internal name.
- Two apps can never share a name, current, reserved or internal, because creation and rename all go through the one availability check, so a taken-over name cannot receive traffic meant for another app.
- A retired name that was an internal key is not free for a new app even after its alias expires. Only a rename back reuses it.
- A rename that also moves containers or volumes to the new name is out of scope. It would need a data copy per volume and is a separate decision.

## Test plan

Store test: alias create, collision with a live alias refused, expiry sweep. API test: rename, old name resolves with the deprecation header, name collision refused, rename during a deploy refused, read-only role refused, audit row written. Resolver test: after a first rename the old name (also the internal key) resolves as an alias with the header and is refused after expiry or with window 0. Create-path collision test: app create, deploy upsert, stack apply and MCP create refuse a live alias, a current public name and a renamed app's internal key. A real run is required before accepting: rename a deployed app with a custom domain and a volume on a real host, confirm the site, backups and logs continue under the new name.

## Open for the owner

Default alias window (suggested 30 days), whether the redirect for the old auto host is wanted or a plain 404 is enough, and whether renaming the internal key (with volume copy) is ever wanted.
