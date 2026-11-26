# ADR 086: Database networking model and the Networking page

Status: Proposed

## Context

The owner asked for a real networking story for databases: firewall level and IP level access, planned the way a managed cloud database service does it with a VPC, subnet groups, security groups and a public accessibility switch. ADR 082 section 6 fixed the exposure levels, allowed sources with expiry and TLS per engine, and its slices 9 to 12 (lane N) name the work. This ADR turns that into one model a user can reason about, one page that shows it, and the enforcement facts behind every badge. It does not change ADR 082 decisions; where it refines one, it says so.

What exists on this branch, read in code:

Network placement and app reach

- Every managed database runs as container `db-<name>` on its own Docker network `db-<name>-net` and never on the default bridge (`internal/reconcile/database/private_network.go`; `leaveDefaultBridge` migrates old containers).
- An app reaches a database by reference: `DatabaseEnv` entries and `DatabaseAttachment` on the desired service. The app controller calls `NetworkConnect` to put the database container on the app's network, so the app resolves `db-<name>` by container name (`internal/reconcile/application/controller.go`, `connectReferencedDatabase`). Hostnames and passwords for `{ from: ... }` env values come from `internal/reconcile/database/address.go`.
- Network scope `platform | project | environment` (`internal/dbaccess/scope.go`, stored in `database_access` settings, default `platform`). It is enforced at attach time: an out of scope app is not connected and gets a reason (`enforceDatabaseScope`). `PUT /network/scope` has a dry run that lists apps that would lose reach.
- Cross node: when the app and the database sit on different nodes, the attach step is skipped (`crossNodeDatabaseName`). The database gets a mesh port (`EnsureDatabaseMeshPort`, range from settings) bound on its node's WireGuard mesh address, never a wildcard (`mesh_endpoint.go`), and the app gets an extra hosts entry that pins `db-<name>` to that mesh address (`mesh_database.go`). Scope is still checked before this, but the mesh port itself has no source filter: any mesh peer that knows the port reaches it.

Publishing and allowed sources

- Public access (`PUT|DELETE /databases/{name}/public-access`, `AbilityWriteSensitive`) publishes a host port with bind `127.0.0.1` by default or all interfaces (`database_public_access.go`, migrations `0026`, `0104`).
- Allowed source rules: `GET /network` (read), `POST /network/rules/preview`, `PUT /network/rules`, `DELETE /network/rules`, `POST /network/make-private`, `PUT /network/scope`, `PUT /network/tls`, `PUT /tls`, `GET /tunnel` (all `AbilityRoot` except the read), routed in `routes_database_access.go`. Rules are CIDRs plus an optional "local containers" source.
- Enforcement is `internal/exposure`: tagged rules in the `DOCKER-USER` iptables chain, a lockout guard for the caller's own address and SSH ports, and a pending window where an applied change reverts unless confirmed (`pending.go`). When `iptables` is missing, the chain does not exist (nftables engines) or the host is not Linux, the chain is unreadable and the API reports `can_restrict: false` with a reason. It does not enforce there.
- Rules are applied only when the database runs on the control plane's own node (`databaseRuleRestriction` refuses others).
- Persistence: `exposure_restrictions` (migration `0399`) is keyed by `(port, protocol)` for the whole host, holds `allow`, `created_by`, `created_at`. There is no database column, no node column, no expiry and no description column. Descriptions sent with a rule are echoed in the plan response (`noteMap`) and then lost.
- Audit: `database.network.rules.apply`, `.rules.remove`, `.make_private`, `.scope.set`, `.tls.set`, `database.tls.toggle` (`internal/dbaccess/actions.go`).

Host and cloud firewall

- `internal/reconcile/firewall` converges the `firewall_rules` table (migration `0262`) onto ufw on the local node only; remote node rules are skipped and logged.
- `internal/exposure/drift.go` reports drift between declared and actual exposure: unmanaged public port, restriction missing, restriction undeclared, listener undeclared.
- Cloud firewall: there is no cloud firewall ADR. ADR 043 is concurrent reconcile (ADR 082 already corrected this). The only cloud firewall code is `internal/provision/aws_network.go` (creates a security group for a provisioned node and revokes unexpected ingress) and the AWS capability list in `internal/cloudconn/aws_capabilities.go` that includes `ec2:DescribeSecurityGroups`. Nothing reads a cloud firewall for drift today, and there is no Oracle security list code.
- `edge_firewall_rules` (migration `1040`) is the HTTP ingress firewall. It does not apply to database TCP ports.

TLS

- Server TLS: Postgres and Redis through URL changes (`SupportsTLS`), MySQL and MariaDB server side only (`ServerTLSEngine`, clients opt in). Others none. No CA download route today.

UI

- `web/src/routes/databases/$name/network.tsx` redirects to Settings, section `networking`. `web/src/components/databaseNetwork/NetworkPanel.tsx` renders four cards: Reachability, Allowed sources, Scope, TLS. Queries in `web/src/queries/databaseAccess.ts`.

CLI

- `levelrail databases network show | allow | deny | make-private | scope | tls` (`cmd/levelrail-cli/databases_network.go`) and `databases public-access`.

The gaps, in plain words: no single model a user can map to the cloud concepts they know; rules are host wide and per port, not per database, with no expiry and no stored description; the mesh port is reachable by every mesh peer; remote nodes get no enforcement; macOS and Windows hosts get none at all; nothing tells a user why an app cannot connect; there is no reusable rule set; branches and hosted databases have no network view.

## Decision

Recommendation in one paragraph: model database networking with five named concepts that map one to one onto the managed cloud database vocabulary: a **network** (per project and per environment, private by default), **placement** (the subnet group: which nodes and Docker networks the database is attached to), **security groups** (named, reusable, per database or shared, ingress rules by source type with port, protocol, optional expiry and description, plus egress rules for hosted databases), an **exposure level** (the public accessibility switch with four values and TLS rules per level) and **endpoints** (per database and per branch). Parameter groups are out of scope here (engine params already have their own routes). Security groups compile down to the enforcement layers that really exist (Docker network attach, `DOCKER-USER` on Linux, the mesh port filter, cloud firewall drift as read only), and every rule shows which layer enforces it on this host, or that nothing does. The Networking page is a first class database tab built around a topology view, the exposure selector, the rules table, a reachability analyzer and a "why can't my app reach the database" checklist.

### 1. Mental model and mapping

| Managed cloud concept | Levelrail concept | Backed by |
| --- | --- | --- |
| VPC | Network: one per project environment, plus the platform network for unplaced databases | Docker networks `db-<name>-net` plus app networks, scope `project` or `environment`, the WireGuard mesh between nodes |
| Subnet group | Placement: the node a database runs on, the Docker networks it is attached to, its mesh endpoint when it has cross node consumers | `desired_databases.node_id`, attach list from the app controller, mesh port |
| Security group | Security group: named rule set bound to one or more databases or branches | new tables, compiled to the layers below |
| Public accessibility | Exposure level: `private`, `project`, `loopback`, `published` | public access fields plus scope plus rules |
| Endpoint | Endpoints: internal (`db-<name>:<port>`), mesh (`<mesh ip>:<mesh port>` for cross node apps), published (`<host>:<port>`), pooled (later, with the pooler) | computed, no new storage |
| Parameter group | out of scope | `routes_database_params.go` already exists |

Network and default privacy

- A database placed in a project gets scope `project` on create; placed in an environment it gets `environment`. Unplaced databases keep `platform`. This refines ADR 082, which kept `platform` as the default everywhere. The setting `databases.network.default_scope` (seed `APP_DB_DEFAULT_SCOPE`, values `platform | placement`, default `placement`) controls it. Existing databases keep their stored scope; nothing changes on upgrade.

Security group rules (ingress)

| Source type | Meaning | Enforced by |
| --- | --- | --- |
| `app` | one app (all replicas) | Docker attach (same node); mesh source filter by the app's node mesh address (cross node, node granularity only, see section 2) |
| `project` | every app in a project | Docker attach plus scope |
| `environment` | every app in one environment | Docker attach plus scope |
| `node` | any process or container on a node, by its mesh address | `DOCKER-USER` on the database node for the mesh port |
| `cidr` | an IPv4 or IPv6 range | `DOCKER-USER` for the published port |
| `my_ip` | the caller's source address as the control plane sees it (behind the trusted proxy setting), stored as a /32 or /128 | `DOCKER-USER`; expiry required |
| `this_machine` | processes on the host that runs the database (loopback) | the `127.0.0.1` bind itself |
| `control_plane` | the control plane, for probes and remote explorer paths | system rule, shown, cannot be removed while the explorer or monitoring needs it |

Note: ADR 082 used "this machine" for the caller's IP. This ADR splits it into `my_ip` (the caller) and `this_machine` (the database host) because users on a laptop mean the second and users on a server mean the first; the UI labels them "My current IP (203.0.113.7)" and "This machine (loopback only)".

Each rule: `source_type`, `source_ref`, `port` (default the engine port; for published databases the host port is derived), `protocol` (`tcp`; `udp` refused for databases), `expires_at` (optional; required for `my_ip`), `description` (up to 120 characters, now stored), `created_by`, `created_at`.

Egress rules exist only on groups bound to hosted or external databases (ADR 085): destination host or CIDR plus port. They gate where the probe and explorer helper may connect. Managed local databases have no egress rules in this ADR (open question 3).

Exposure levels (ADR 082 table, unchanged rules, renamed `public` to `published` in the UI because "public" reads as "open to all")

| Level | What it means | TLS rule | Rules needed | Risk badge |
| --- | --- | --- | --- | --- |
| private | no host port; only attached apps on the same node and in scope, and cross node consumers over the mesh | optional | none | Low |
| project | private plus scope `project` or `environment` | optional | none | Low |
| loopback | host port on `127.0.0.1` for local tools on that host | optional, warn when off | none | Medium on a server, Low on a laptop |
| published | host port on all interfaces | required (refused if the engine has no server TLS here) | at least one non `this_machine` ingress rule | High; Critical if `databases.public.allow_open` was used |

Published is refused, not shown as protected, when the host cannot enforce rules (chain unreadable). `databases.public.allow_open` (ADR 082, default off, step up) remains the only way to publish without rules and is shown as Critical.

### 2. Enforcement layers and honest limits

| Layer | What it enforces | Where it works | Where it does not |
| --- | --- | --- | --- |
| Docker network attach | which containers can resolve and reach `db-<name>` on the same node | every runtime | does not stop a container that an operator attaches by hand outside Levelrail; drift check flags unknown attachments |
| `DOCKER-USER` chain (`internal/exposure`) | source CIDRs for published ports and, after slice 10, for the mesh port | Linux with iptables and the chain present, on the control plane node only today | nftables only engines, macOS, Windows, remote nodes (until agent RPC) |
| Loopback bind | only local processes | everywhere | other users on the same host can still connect; credentials are the boundary |
| Mesh | encryption and peer identity between nodes | every enrolled node | does not limit which peer reaches which port; slice 10 adds the source filter on the database node |
| ufw controller | host level port rules | local node only | remote nodes |
| Cloud firewall | nothing enforced by Levelrail; read only drift view after slice 13 | AWS nodes Levelrail provisioned, if the connection has `ec2:DescribeSecurityGroups` | Oracle, other providers, hand built nodes |

How the UI says it: every rule row carries an enforcement chip: "Enforced (Docker network)", "Enforced (host firewall)", "Recorded, not enforced on this host" (amber), "Not enforceable here" (red, rule saved but published refused). When the database host cannot enforce IP rules, a banner on top of the page reads: "This host cannot enforce IP rules. Levelrail runs here without a Linux iptables firewall it can manage (for example on macOS, Windows or an nftables only Docker engine). Only Private, Project and Loopback are available. Apps on this machine are isolated by Docker networks; IP based rules are saved for when the database moves to a Linux node." The reason string from `Chain.Reason` is shown under a "Details" disclosure. On a macOS install that runs Docker inside a Linux VM the chain may exist inside the VM but the control plane cannot manage it from the host: treat as not enforceable until proven (see "What cannot be verified locally").

Remote nodes: in 1.5.11 rules for a database on a remote node are stored and shown "Recorded, not enforced (remote node)", and published is refused there, matching today's refusal in `databaseRuleRestriction`. Enforcement on remote nodes waits for the agent RPC (slice 10, later).

### 3. The Networking page

Route: `web/src/routes/databases/$name/network.tsx` stops redirecting and becomes the tab again ("Networking" in the database tab bar); the Settings `networking` section links to it. Order of sections, top to bottom, compact and two columns at wide widths:

1. **Header strip**: exposure level chip, risk badge (Low, Medium, High, Critical), TLS chip, enforcement chip for this host, node name and location, "Test connectivity" and "Diagnose" buttons.

2. **Topology view**: a left to right graph: sources (apps grouped by project and environment, nodes, CIDR rules, "My IP", the control plane) to placement (node, Docker networks, mesh endpoint) to the database (and its branches as small children). Edges are solid when reachable now, dashed when allowed but not running, red with a strike when blocked, with a tooltip naming the rule or the reason (scope reason from `dbaccess.Allows`, missing rule, unenforced). The control plane edge is labelled with the explorer path in use: "explorer runs inside the container (exec)" for managed engines, "explorer through helper on node X" for hosted ones. Built from `GET /network` plus a new `topology` block; no live polling beyond the existing query refresh.

3. **Exposure level selector**: four radio cards with one sentence each and the requirements inline. Choosing Published opens a checklist: TLS required (link to the TLS card, disabled until the engine supports it), at least one rule (link to the rules table), host can enforce. The save goes through the existing preview then apply with the pending confirm window from `internal/exposure`; the dialog shows the exact plan commands (`Plan.Commands`) collapsed and the persistence note. Moving to a lower level is one click with a confirm that lists apps or rules that stop working.

4. **Security groups and rules table**: groups bound to this database as tabs (default group `default` per database). Columns: source (icon by type, resolved name, address when relevant), port, protocol, enforcement chip, expires (relative time, amber under one hour), description, created by, actions (edit description, extend expiry, remove). "Add rule" dialog: source type picker (App, Project, Environment, Node, CIDR, My current IP, This machine), a searchable picker for the chosen type prefilled with apps already referencing the database, port prefilled with the engine port, expiry prefilled for My current IP with `databases.network.temp_source_hours` and an "Never" option for other types, description. The dialog previews which layer will enforce the rule before saving and refuses `0.0.0.0/0` and `::/0` unless `allow_open` is on. "Attach group" adds a shared group; shared groups show "used by N databases".

5. **Test connectivity** (reachability analyzer): choose a source: "Control plane", "An app" (picker), "A node" (picker), "This machine" (shows the CLI command to copy, because a browser cannot open raw TCP). The control plane runs a static analysis first (path, layers, the first rule that matches or the rule set that blocks) and then an active probe within `APP_DB_PROBE_TIMEOUT_MS`. Result card: verdict `reachable | refused | timeout | blocked_by_rule | dns_failed`, TCP connect time, TLS handshake result (`ok` with protocol version and certificate subject and expiry, `not_offered`, `failed` with the reason), the matched or blocking rule with a link to its row, and the enforcing layer. The app probe runs a short lived helper container that joins the app container's network namespace (`--network container:<app container>`) so it sees exactly what the app sees, without exec into the user's container. The node probe runs the same helper on the chosen node through the agent.

6. **Why can't my app reach the database?**: pick an app; a checklist with pass, fail or skipped and one line fix each:
   - app references the database (env binding or attachment exists)
   - scope admits the app (reason from `dbaccess.Allows`)
   - same node: app network attached to the database container; cross node: mesh port allocated, mesh path usable, extra hosts entry present in the app container
   - a rule admits the source (for published or mesh paths)
   - firewall enforced or not applicable
   - port published and bound where expected (for published levels)
   - TLS: required by server and the app's URL asks for it (for example `sslmode` for Postgres)
   - credentials: the app's resolved secret matches the current database secret (compare secret versions, never values) and an auth ping from the helper succeeds, with the password passed on stdin, not in the environment
   - database running and healthy
   Each fail row carries a typed fix (`exposure.ReachFix`, auto or guided) the same way the exposure findings already do.

7. **Connection endpoints**: rows for internal, mesh, published and pooled (when the pooler is on), and per branch, each with host, port, a copy button for the host:port and for a full URL with the password masked until reveal (reveal stays `AbilityRoot` through the existing credentials reveal route).

8. **TLS card**: mode (off, on, required), server certificate subject and expiry with a warning under `APP_DB_TLS_EXPIRY_WARN_DAYS`, CA bundle download (`GET /databases/{name}/tls/ca`, slice 8 after ADR 082 slice 11), client verify mode snippets per engine, and an "unsupported for this engine" state that names the engine.

9. **Connection limits**: engine max connections (read from the engine where possible), current connections, brokered login limit (`APP_DB_BROKERED_CONN_LIMIT`), explorer limits. Read only in 1.5.11; edits later with ADR 082 slice 12.

10. **Network audit log**: last 50 network events for this database (filter of `audit_log` by `database.network.*` and resource), with actor, time, change summary and a link to the full audit page.

Branches: a branch page shows the same tab. A branch starts `private` with the `inherit` mode: it is bound to the parent's security groups read only, plus nothing published. "Override" detaches the inherited groups and creates a branch group. Published on a branch needs its own rules (ADR 082: allowed sources are never copied as active published rules). Inherit is a binding, not a copy, so changing the parent group changes every inheriting branch; the UI says so in the group header.

Hosted and external databases: the tab shows placement as "Runs at <provider and region>, reached from node X", the exposure selector is replaced by a read only "provider side exposure" card (filled by the provider adapter when it can read it, ADR 085), ingress rules are hidden (the provider owns them), and an egress section shows the outbound allow list, the private range policy (`databases.external.private_ranges` from ADR 082), the pinned resolved address and the TLS verify mode (`verify-full` default when the provider has a public CA, ADR 085 section 5). Test connectivity and Diagnose work the same, run from the chosen node.

### 4. Data model, API, CLI, IaC, IAM, audit, settings

Tables (names only):

- `database_security_groups` (id, name, scope: database or shared, project id, description, created by and at)
- `database_security_group_rules` (id, group id, direction, source type, source ref, port, protocol, expires at, description, created by and at)
- `database_security_group_bindings` (group id, database name, mode: direct or inherited, bound by and at)
- `database_network_settings` (database name, exposure level, last applied plan hash, last enforcement state)
- `exposure_restrictions` stays the kernel desired state but gains `node_id` and `owner` (database name) columns; the compiler writes it, users no longer write it directly.

API (all under `/api/v1/databases/{name}/network`, existing routes kept as aliases during migration):

- `GET /network` adds `level`, `risk`, `enforcement`, `topology`, `endpoints`, `groups`.
- `GET|POST /network/groups`, `PATCH|DELETE /network/groups/{id}`, `POST /network/groups/{id}/rules`, `DELETE /network/groups/{id}/rules/{rule}`, `POST /network/groups/{id}/bind`.
- `PUT /network/exposure` with `level`, `dry_run`, `confirm` (wraps public access and the pending confirm flow).
- `POST /network/test` with `source: {type, ref}`; `POST /network/diagnose` with `app`.
- `GET /tls/ca`.
- Shared groups at `/api/v1/network/security-groups` for reuse across databases.

CLI:

```
levelrail databases network show <db>
levelrail databases network rules add <db> --source app:web|project:shop|env:staging|node:n2|cidr:10.0.0.0/8|my-ip|this-machine [--port N] [--expires 8h] [--description TEXT] [--group NAME]
levelrail databases network rules rm <db> <rule id>
levelrail databases network expose <db> private|project|loopback|published [--confirm]
levelrail databases network test <db> --from control-plane|app:<name>|node:<name>|here
levelrail databases network diagnose <db> --app <name>
```

`--from here` runs the TCP and TLS probe on the user's machine, which is the only honest "this machine" test. Existing `allow`, `deny`, `make-private`, `scope` stay as aliases.

IaC and spec (ADR 076 levelrail as code): a top level `security_groups` list with `name`, `rules` (`source`, `port`, `expires`, `description`), and on each database `network: { exposure: private, scope: project, security_groups: [db-app-access] }`. Plan shows layer and enforceability per rule; apply refuses `published` where the target node cannot enforce. `my_ip` and expiring rules are refused in IaC (they are interactive by nature).

IAM: new actions in the ADR 082 `db:*` set, mapped to today's abilities until policies say otherwise:

| Action | Covers | Default holder |
| --- | --- | --- |
| `db:network:read` | page, topology, endpoints (masked), audit view | `read` (today's `GET /network`) |
| `db:network:test` | test connectivity, diagnose | `read:sensitive` (probes reveal topology), rate limited |
| `db:network:write` | rules, groups, scope, private, project, loopback levels, TLS toggle | `root` (today) |
| `db:network:expose` | the `published` level and `allow_open` | `root` plus step up |

Audit events (extend the existing `database.network.*` names): `database.network.group.create|update|delete|bind|unbind`, `database.network.rule.add|remove|expire|extend`, `database.network.exposure.set`, `database.network.test` and `database.network.diagnose` (info level, sampled by the rate limit), plus the existing apply, remove, make private, scope and TLS events.

Settings (live, env only seeds the default):

| Setting | Seed env | Default |
| --- | --- | --- |
| `databases.network.default_scope` | `APP_DB_DEFAULT_SCOPE` | `placement` |
| `databases.network.temp_source_hours` | `APP_DB_TEMP_SOURCE_HOURS` | 8 |
| `databases.network.my_ip_max_hours` | `APP_DB_MY_IP_MAX_HOURS` | 72 |
| `databases.network.rule_sweep_seconds` | `APP_DB_RULE_SWEEP_SECONDS` | 60 |
| `databases.public.allow_open` | `APP_DB_PUBLIC_ALLOW_OPEN` | off |
| `databases.network.probe_timeout_ms` | `APP_DB_PROBE_TIMEOUT_MS` | 3000 |
| `databases.network.probes_per_minute` | `APP_DB_PROBE_PER_MINUTE` | 20 per user |
| `databases.tls.expiry_warn_days` | `APP_DB_TLS_EXPIRY_WARN_DAYS` | 21 |
| `databases.external.private_ranges` | from ADR 082 | per ADR 082 |

Defaults: private by default; published needs at least one rule and server TLS; `my_ip` always expires (default 8 hours, cap 72); new rules default to the engine port; loopback is the default bind when a user publishes from a laptop.

### 5. Migration without breaking existing exposures

1. For every database with a published port on the local node and a matching `exposure_restrictions` row, create its `default` group and one `cidr` rule per allow entry (no expiry, description "migrated"), set `owner` and `node_id` on the restriction row. The compiled restriction equals the existing one, so the chain is not touched (`inSync` is true).
2. A published database with all interfaces bind and no rules keeps working: level `published`, flag `legacy_open`, risk Critical, doctor warning, and an amber callout "Published to everyone before rules existed. Add a rule or switch to Loopback." Nothing is closed automatically. The next edit must satisfy the new rules.
3. A published database without TLS keeps working with the same pattern (`legacy_no_tls`).
4. Scope values are kept. Apps already attached stay attached.
5. Restrictions whose port matches no database stay untouched and keep showing in the host exposure page as today.
6. Existing routes and CLI verbs stay as aliases for at least one minor release.

## Alternatives rejected

- A TCP proxy in front of every database (one enforcement point on every OS). Adds a hop, a single point of failure and a protocol aware piece for MySQL; ADR 082 already put SNI routing later. Kept as a future option for macOS where it would be the only enforcement.
- Enforcing IP rules inside the engine (`pg_hba` per CIDR, MySQL host grants). Works per engine only, Redis and Mongo have no equivalent, and it mixes network and credentials. Engine level host rules remain a later hardening for brokered logins (ADR 082).
- Per app source enforcement on the mesh by IP. Containers do not have stable mesh addresses; node granularity is the honest limit until per app mesh identities exist.
- Copying rules into branches. Copies drift and leak published access; binding with inherit or override is explicit.
- Automatic cloud firewall changes. Rejected for 1.5.11 (ADR 082 open question 6): read only drift first.
- Keeping rules host wide per port. Cannot express per database ownership, expiry or remote nodes.

## Security

- Lockout: the existing caller and SSH guard (`guard.go`) and the pending confirm window apply to every compiled change, including expiry sweeps that would remove the caller's own `my_ip` rule (the sweeper never removes a rule inside a pending window; it waits).
- `my_ip` trusts the client address only behind the configured trusted proxies; with no trusted proxy setting and a proxy in front, the UI shows the address it saw and warns that it may be the proxy.
- Probes: the helper never receives database passwords in its environment (stdin only), runs with no extra capabilities, a read only filesystem, and is removed after `probe_timeout_ms` plus a margin. Probe targets are only the database's own endpoints, never user supplied hosts, so the test route cannot become a port scanner. Rate limited per user.
- Diagnose compares secret versions, never values, and never returns the resolved password.
- Topology and endpoints reveal internal addresses: gated `db:network:read`, published host addresses only for `read:sensitive`.
- Shared groups: binding a shared group to a database needs `db:network:write` on the database and on the group's project.
- IaC cannot create `allow_open`, `my_ip` or rules without a description on published databases.

Security review checklist (lr-reviewer):

1. No rule compiles to `0.0.0.0/0` or `::/0` without `allow_open` and step up.
2. Published refused when `Chain.Readable` is false or the node is remote, by the API, the CLI and IaC plan.
3. Expiry sweeper removes kernel rules and the row in one transaction order (kernel first, then row), retries on failure, and audits `rule.expire`.
4. Helper container: no secrets in env, no Docker socket, no host network, removed on every path.
5. Test route cannot target an address other than the database's endpoints.
6. `my_ip` address source matches the trusted proxy logic used elsewhere (`peerIsTrustedProxy`).
7. Every write route maps to the right `db:network:*` action and writes an audit row.
8. Migration leaves `DOCKER-USER` byte for byte unchanged on upgrade.
9. Mesh source filter never blocks the control plane's own mesh address (monitoring and backups).

## Failure modes

- Chain read fails transiently: page shows "unknown", no apply; the existing pending revert covers a half applied change.
- Rule sweeper down: expired rules stay enforced; the page marks them "expired, removal pending" in red and doctor flags it after two sweep intervals.
- Mesh path flaps: the existing binding is kept (`meshBinding`), diagnose shows "mesh path unusable" rather than a firewall cause.
- App on a node the agent cannot reach: app and node probes return `skipped` with the reason, not `timeout`.
- Docker `container:` network mode unavailable for a runtime: app probe falls back to a helper attached to the app's network, labelled "approximate".
- A database moves nodes: compiled restrictions are removed on the old node and applied on the new one; until the new node can enforce, published is downgraded to loopback with an audit row and a notification (owner decision, open question 2).
- Inherited group edited while a branch is published with override: no effect, overrides are independent.

## What cannot be verified locally

- `DOCKER-USER` behaviour on the mesh port path (traffic arriving on the WireGuard interface and DNAT to the container). Needs a Linux node pair (VERIFY on the test hosts).
- Docker inside a Linux VM on macOS: whether the chain is reachable and manageable from the control plane. Treated as not enforceable until proven.
- nftables only engines with `iptables-nft` shims: chain detection works in fakes; real behaviour per distro is unproven.
- Cloud firewall drift: needs a provisioned AWS node with read permission; Oracle security lists have no code yet.
- TLS handshake details per engine image for the probe (MySQL and MongoDB TLS depend on ADR 082 slice 11).
- Agent RPC for remote node enforcement does not exist yet.

## Testing plan

- Unit: level computation from public access, bind, scope, rules and chain state; rule compiler from groups to attach sets, restrictions and mesh filters; expiry sweeper with a fake clock and the pending window; migration from `exposure_restrictions` producing an identical plan.
- API: each route's action mapping and audit row; published refused when unenforceable or remote; `0.0.0.0/0` refused; test route rejects foreign targets; diagnose never returns secrets.
- Exposure fakes (`NewFakeManager`) for linux, darwin and windows `goos` values and the no chain case.
- Web: Networking page read only state (as `NetworkPanel.readonly.test.tsx` does today), add rule dialog prefill, banner on unenforceable hosts; check at two widths.
- Real host (Linux test node): publish with a CIDR rule, probe from inside and outside the range, let a `my_ip` rule expire, cross node app via mesh with and without the node rule.

## Build slices

SEC means an lr-reviewer pass. "1.5.11" means it can ship on the current release; "later" means after.

| # | Slice | Files (owner) | Settings | Review | Release |
| --- | --- | --- | --- | --- | --- |
| 1 | Persist per database rules: `node_id`, `owner` on `exposure_restrictions`, rules table with expiry and description, migration of existing rows, sweeper | `internal/store/migrations/*_database_network_rules.sql`, `internal/store/database_network_rules.go`, `internal/exposure/sweep.go` (new), `internal/api/database_network_rules.go` | `APP_DB_TEMP_SOURCE_HOURS`, `APP_DB_MY_IP_MAX_HOURS`, `APP_DB_RULE_SWEEP_SECONDS` | SEC | 1.5.11 |
| 2 | Exposure level, risk and enforcement block in `GET /network` (ADR 082 slice 9) | `internal/api/database_network.go`, `internal/api/database_exposure.go` (new) | `APP_DB_PUBLIC_ALLOW_OPEN` | no | 1.5.11 |
| 3 | Networking tab shell: route un redirect, header strip, exposure selector, enforcement banner | `web/src/routes/databases/$name/network.tsx`, `web/src/components/databaseNetwork/NetworkPage.tsx`, `ExposureSelector.tsx`, locales | none | no | 1.5.11 |
| 4 | Rules table and add rule dialog with source types app, project, environment, CIDR, my IP, this machine | `web/src/components/databaseNetwork/RulesTable.tsx`, `AddRuleDialog.tsx`, `web/src/queries/databaseAccess.ts`, `internal/api/database_network_rules.go` | as slice 1 | SEC | 1.5.11 |
| 5 | `PUT /network/exposure` wrapping public access, rules and the pending confirm; CLI `expose`, `rules add|rm` | `internal/api/database_network_exposure.go` (new), `cmd/levelrail-cli/databases_network.go` | none | SEC | 1.5.11 |
| 6 | Static reachability analyzer and diagnose checklist (no active probe) | `internal/api/database_network_diagnose.go` (new), `internal/exposure/classify.go` (read only use), web `DiagnosePanel.tsx` | none | no | 1.5.11 |
| 7 | Active probe from the control plane and from an app (helper in the app namespace), CLI `test --from here` | `internal/dbprobe/*` (new), `internal/api/database_network_test.go`, `cmd/levelrail-cli/databases_network_test_cmd.go`, web `TestConnectivityDialog.tsx` | `APP_DB_PROBE_TIMEOUT_MS`, `APP_DB_PROBE_PER_MINUTE` | SEC | 1.5.11 |
| 8 | Topology view, endpoints card, TLS card with expiry, network audit list | web `TopologyView.tsx`, `EndpointsCard.tsx`, `TlsCard.tsx`, `NetworkAuditList.tsx`, `internal/api/database_network.go` (topology block) | `APP_DB_TLS_EXPIRY_WARN_DAYS` | no | 1.5.11 (CA download later with ADR 082 slice 11) |
| 9 | Named and shared security groups, bindings, compiler; IAM actions `db:network:*`; IaC fields | `internal/store/migrations/*_database_security_groups.sql`, `internal/dbnet/*` (new compiler), `internal/api/iam.go`, `internal/api/database_security_groups.go`, IaC schema files | `APP_DB_DEFAULT_SCOPE` | SEC | later |
| 10 | Mesh port source filter and remote node enforcement through the agent | `internal/reconcile/database/mesh_endpoint.go`, `internal/exposure/*`, agent RPC files | none | SEC | later |
| 11 | Branch inherit and override | `internal/api/database_branch*.go`, `internal/dbnet/*`, web branch tab | none | SEC | later (after ADR 081 slices) |
| 12 | Hosted egress: allow list, pinned address, verify full, read only provider exposure card | `internal/extdb/conn.go`, `internal/extdb/helper.go`, web `EgressCard.tsx` | ADR 082 private range setting | SEC | later (with ADR 085) |
| 13 | Cloud firewall drift, read only, AWS first | `internal/cloudconn/*`, `internal/provision/aws_network.go` (read only use), `internal/api/database_network.go` | none | SEC | later |
| 14 | Node probe, connection limits card, doctor checks for legacy open and legacy no TLS | `internal/dbprobe/*`, `internal/doctor/*`, web `LimitsCard.tsx` | ADR 082 limit settings | no | later |

Order: 1 then 2; 3 can start beside 2 against a mocked response; 4 and 5 after 1; 6 after 2; 7 after 6; 8 after 2. Slices 1, 4 and 5 all touch `internal/api/database_network_rules.go` and are sequenced. Slice 9 is the first to touch `internal/api/iam.go` and must follow ADR 082 slices 1 and 2.

## Consequences

- Users get one vocabulary that matches what they know from managed cloud databases, and one page that answers "who can reach this database and why not my app".
- Every rule says which layer enforces it, so a laptop install stops looking protected when it is not.
- Rules become per database with expiry and stored descriptions; the host wide per port table becomes an internal compile target.
- More moving parts: a sweeper, a probe helper image, a compiler. Doctor checks and the audit list keep them visible.
- New databases in a project are scoped to it by default, which can surprise a user who referenced a database across projects; the attach reason and the dry run show it.

## Open questions

1. Default scope for new placed databases: `placement` (proposed) or keep `platform` and only suggest it?
2. When a published database moves to a node that cannot enforce, downgrade to loopback automatically (proposed) or refuse the move?
3. Egress rules for managed local databases (stop a compromised database from calling out): wanted now or later?
4. Mesh source filter granularity: node only (proposed) until per app mesh identities exist, acceptable?
5. Should `db:network:test` require `read:sensitive` (proposed) or plain `read`?
6. Cloud firewall drift: AWS only first (proposed), or wait for Oracle security list support too?
7. Is a TCP proxy for macOS enforcement worth a later ADR, given laptop installs are mostly single user?
