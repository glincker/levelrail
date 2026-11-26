# ADR 088: Databases on remote nodes and cloud placement

Status: Proposed

## Context

Operators want to run a database on a remote node and connect to it at that node directly. The control plane should not sit in the network path of a database connection. A remote node should behave as a standalone unit with its own network when needed, so a control plane restart or outage costs as little database availability as possible. Operators also want to start a database on a new cloud machine (AWS, Azure, Google Cloud, Oracle Cloud, Hetzner, DigitalOcean) from the create database wizard, and see a clear way to connect a cloud account or add a node when none is connected yet.

This ADR decides the split between the control plane and the node for databases, the connection model, the scale and failure domain rules, the Placement step of the wizard, and the data model and API changes. Read replicas are in scope as a placement rule only. Automatic failover is out of scope and gets its own ADR.

### What exists today (read on `work/stage`)

Node model and agent:

- `internal/store/nodes.go`: `Node` has `Address`, `Status`, `Schedulable` (cordon), `AcceptsAppWorkloads`, `AcceptsBuildWorkloads`, `MeshPublicKey`, `MeshAddress` (WireGuard), certificate lifecycle fields, agent self report (`AgentVersion`, `AgentOS`, `AgentArch`) and a free text `Region` that is display only, never a routing or access input.
- `internal/agent/transport.go`: `Transport` embeds `docker.Runtime`. `Local` is the in process form, `GRPCTransport` (`grpc_transport.go`) the remote form. The remote agent is a remote Docker runtime: it executes calls, it does not decide anything.
- Agent link: heartbeats every 15 s (`internal/agent/client.go`), `last_seen` written per frame, reconnect with backoff 1 s to 30 s plus jitter, client certificates valid 90 days and renewed by the control plane (`internal/agent/server.go`, `cmd/levelrail/agent_certs.go`).
- `internal/agent/silence.go`: an agent side watcher of control plane silence with `APP_AGENT_CP_SILENCE_*` knobs (grace 5 min default). It alerts; it does not reconcile.
- `internal/agent/offline_send.go`: metrics and events that could not be sent are buffered and replayed after reconnect.
- `internal/agent/exec_relay.go`, `logs_relay.go`, `tunnel.go`, `tunnel_node.go`: exec, logs and an ingress tunnel are carried over the agent's outbound connection, through the control plane.
- `internal/agent/host_report.go` states it directly: "The agent has no desired state". There is no cached, signed desired state on the node today.
- `internal/agentupdate`: agent self update with a lock and package managed detection. Whether an agent update restarts database containers: to verify (Docker owns the containers, so it should not).

Database reconcile:

- `cmd/levelrail/boot_source.go` (`databaseControllersFor`): one `database.Controller` per database, built in the control plane with `resolveNodeTransport(..., desired.NodeID)`. If the node transport is unavailable the database is skipped for that pass ("node transport unavailable"). Nothing is torn down.
- `internal/reconcile/database/controller.go`: container, volumes, WAL archive volume, certs volume, private network (`private_network.go`), TLS (`tls.go`, `tls_mount.go`), pooler, crash loop guard, agent version gate (`agent_gate.go`, `WithRemoteNode`).
- `internal/reconcile/database/mesh_endpoint.go`: a database with an allocated mesh port (`GetDatabaseMeshPort`, the `database_mesh_ports` table) is published on its node's mesh address, so apps on other nodes reach it over WireGuard. A mesh flap keeps the existing binding.
- `internal/api/databases.go`: create accepts `node_id` and auto placement (`resolveCreateNodePlacement`, `recordCreatePlacement`); `PUT /api/v1/databases/{name}/node` changes placement. `internal/placement` has affinity, taints, zones and region and provider spread weights for replica scoring. `internal/api/node_pools.go` has named pools for apps; databases do not use pools yet (to verify).
- `internal/api/database_public_access.go`: public access binds a host port on the node where the container runs, with `bindaddr` validation and refusal of public binds for engines without a password. So a public or private bind on a remote node already serves connections directly at the node, with no control plane hop. What host the UI shows in the connection string for a remote database: to verify (`internal/api/database_connect_snippets.go` takes a `Host` from its caller).
- `internal/api/database_tunnel.go`: the websocket tunnel only works for databases on the control plane's own node.

Which flows go through the control plane today:

| Flow | Path today | Data in the control plane |
| --- | --- | --- |
| Provision and reconcile | Control plane controller drives the node's Docker through the agent | Desired state, root credential as container env |
| Explorer (viewer, query, engine stats) | `resolveDatabaseExecContainer` then `Exec` over the agent relay | Query text and result rows pass through |
| Backups | `backup.Runner` with `ContainerDumper{Runtime: client}` and `routedStorage(agentRegistry)` for upload (`cmd/levelrail/boot_stages_services.go`); whether a remote dump streams through the control plane or uploads from the node: to verify | Possibly the dump stream |
| PITR WAL shipping | `internal/backup/wal_ship_scheduler.go` skips databases whose node is not local | Remote PITR is not supported |
| Metrics and logs | Pulled on demand per call over the agent session (`docs/resilience.md`, scale audit) | Samples |
| App connections on the same node | Docker network on the node | None |
| App connections across nodes | Mesh port on the node's WireGuard address | None |
| Public client connections | Host port on the node | None |
| HTTP ingress for apps | Embedded ingress in the control plane, over the mesh (ADR 078 and 079 move this) | Request bytes, not relevant to database protocols |

Cloud and node install:

- `internal/api/node_provision.go`: creates a server at a provider with a cloud-init script that redeems a freshly minted join token (TTL 60 min), then waits for the agent to enroll. Providers: `hetzner`, `digitalocean`, `vultr`, `aws`, `azure`, `gcp`, `oracle` (`internal/provision`). Live region and size catalogs with prices, cached.
- `internal/api/node_provision_guard.go`: server count cap `provision.max_servers` (default 1, env `APP_PROVISION_MAX_SERVERS`), monthly price cap `provision.max_monthly_price_usd` (env `APP_PROVISION_MAX_MONTHLY_PRICE_USD`), unknown price refused unless `provision.allow_unknown_price`, enroll timeout `APP_NODE_PROVISION_TIMEOUT` (20 min), a leaked server check that keeps counting a failed row whose server was not confirmed deleted, and orphan lookup by provision tag.
- `internal/cloudconn` (AWS and Oracle connections, capabilities, SSO), `internal/spendcap`, `internal/spendguard`, `internal/cloudspend`, `internal/costestimate`: account connections and spend rules.
- `internal/api/node_join_command.go`, `node_join_token_admin.go`, `node_ssh_provision.go`: install command with join token, and SSH provisioning of an existing host.
- `internal/nodemove`, `internal/api/node_move_exec.go`, `node_drains.go`: drain and move, including database teardown on the old node after a move.
- `internal/nodefailure`: rescheduling of stateless apps, opt in, never for stateful ones.

Web:

- `web/src/components/CreateDatabaseFields.tsx` already handles a node choice; `DatabaseLocationBadge.tsx` shows where a database runs; `AddNodeWizard.tsx` is the add node flow. Where exactly the node picker sits in the create flow and whether it shows zone, disk and load: to verify.

### Goals

1. No control plane in the data path of any database connection, for clients, apps and operators reaching the engine port.
2. A remote node keeps its databases running, restarted and backed up while the control plane is unreachable.
3. Credentials for a database live on its node and in the operator's vault (ADR 087), and stay in the control plane only as long as needed to deliver them.
4. Placement is a first class step of the create wizard, including a new cloud machine created in the same flow, with spend confirmed and capped.
5. The design holds at hundreds to low thousands of nodes without per call round trips on hot loops.

### Non goals

- Automatic primary failover, consensus or multi primary engines. Separate ADR.
- Running the control plane itself in several places (ADR 033 line of work).
- Moving a live database without a short write pause. A move is backup and restore or branch based, with a stated cutover.

## Decision

Recommendation: treat each node as the owner of its databases' data plane. The agent gains a small local reconciler for databases that runs from a signed, versioned desired state bundle it keeps on disk, plus a local backup runner, a local metrics buffer and an authenticated endpoint for explorer exec. The control plane stays the source of desired state, authorization, audit aggregation, scheduling and UI, but it is never in a connection path. Connection strings point at the node address. The wizard gets a Placement step with four choices and CTAs instead of empty lists. Ship in three proofs: direct connection to a database on an existing remote node, the Placement step with existing nodes and CTAs, then the cloud node create flow behind the experimental flag.

### A. Data plane and control plane split

| Concern | Owner after this ADR | Behavior when the control plane is unreachable |
| --- | --- | --- |
| Container, volumes, private network | Node agent (local reconciler) | Keeps running; crash restarts and recreates use the cached bundle |
| Local firewall and bind addresses | Node agent | Rules from the last bundle stay applied |
| Engine endpoint and TLS certificate | Node agent | Serves with the cached certificate until expiry |
| Scheduled backups and WAL shipping | Node agent, uploads straight to the target | Runs on schedule, reports history later |
| Metrics and events | Node agent buffer (`offline_send.go` extended) | Buffered up to a cap, shipped on reconnect |
| Explorer exec | Node agent, exec inside the container | Unavailable unless the browser can reach the node directly (later slice) |
| Desired state, versions, params | Control plane, signed into the bundle | Frozen at the last version received |
| Authz, audit, scheduling, UI | Control plane | Unavailable; node keeps a local audit spool |
| Moves, scale, new databases | Control plane | Unavailable |
| Alerts | Control plane, plus agent silence watcher | Agent watcher alerts on silence (exists); database alerts wait |

Desired state bundle. Today the agent has no desired state (`host_report.go`). The new bundle holds, per database on that node: engine, image digest, params, volumes, binds, firewall allowlist, TLS material reference, backup schedule and target reference, and a monotonic version. It is signed by a control plane key the agent pins at enrollment, stored with mode 0600 under the agent data directory, and applied only if the signature verifies and the version is newer. The control plane keeps reconciling as today while connected; the local reconciler only acts when the control plane has been silent past `agent.local_reconcile_after` or when a container exits. Both use the same spec code, so they cannot disagree on what the container looks like. One writer at a time: while a session is live the control plane path wins and the local loop only watches.

Credentials. The root credential is already container env (ADR 087). The bundle carries it encrypted to the node's key (the agent identity key from enrollment), never in clear, and the control plane drops its delivery copy after the agent acknowledges. With an ADR 087 vault source the bundle carries a reference and the agent resolves it with a node scoped vault identity, so the control plane never holds the value. Explorer exec keeps the earlier rule: the client runs inside the container with a brokered login (ADR 082), so the browser never receives an engine password.

Explorer path. First cut keeps the existing relay through the control plane for exec (it is a management path, not a data path) and states that it needs the control plane. A later slice lets the dashboard open a direct authenticated session to the agent (short lived token minted by the control plane, verified by the agent, mTLS or the mesh) so query results do not transit the control plane.

### B. Connection model

- The connection string host is the node's database endpoint, chosen per database by endpoint mode:
  - `private`: the node's private address (cloud VPC subnet or LAN, RFC1918). Default.
  - `mesh`: the node's WireGuard mesh address (exists through `database_mesh_ports`). Default for apps on other nodes.
  - `public`: the node's public address, only with an allowlist and TLS on.
  - `local`: loopback on the node, for apps on the same node and for laptops.
- New node fields: `public_address`, `private_address`, optional `endpoint_hostname`. The agent reports the addresses it sees; the operator confirms. A verification route checks that the endpoint answers from the control plane and, for public mode, that the allowlist is in force.
- Firewall: the agent owns host rules for database ports from the bundle allowlist (ADR 086 enforcement layers apply unchanged). Default deny for public mode until at least one CIDR is set.
- TLS: per database endpoint certificate. Default issuer is a Levelrail database CA whose intermediate is on the control plane and whose leaf is issued for the node's names and addresses, delivered in the bundle, renewed ahead of expiry with a validity longer than a plausible outage (setting below). ACME on the node is an option for public hostnames only, because it needs port 80 or DNS on the node.
- Apps across nodes connect over the mesh address (no control plane hop, WireGuard encrypted). Same node apps use the node's Docker network. Latency is one WireGuard hop; cross region placement is shown in the wizard as a warning.
- "Own network if needed": a node may run with only its local Docker network and private subnet, with the mesh optional. A node in a cloud VPC can serve private mode to apps in the same VPC that Levelrail does not manage.
- Laptop: local mode only, the endpoint is `127.0.0.1:<port>`, unchanged from today.

### C. Scale and single points of failure

- No control plane in any database data path after slice 1. The remaining shared dependencies are the mesh (only for cross node app traffic) and the vault (only at container create, ADR 087 level 2).
- Failure domains: node, zone, region. `Region` becomes a placement input for databases through a new `zone` label (today `Region` is display only, so this is a deliberate change, gated behind the placement rule, never a routing input).
- Replicas: read replicas on other nodes are in scope as a placement rule (anti affinity with the primary, zone spread through `internal/placement` weights). Replication setup per engine and failover are out of scope here.
- Placement policy for databases: schedulable, accepts databases, Docker present, agent version above the gate, free disk above `databases.placement.min_free_disk_percent` after the requested volume size, memory headroom, pool selector, anti affinity for replicas, per node cap `databases.placement.max_per_node`.
- Thousands of nodes: the bundle is pulled by the agent on connect and on a version nudge, not pushed in full on every pass; bundle size is per node, so it does not grow with fleet size. Heartbeat `last_seen` writes coalesce in memory and flush per interval (scale audit item). Stats move to one batched call per node per tick.
- Agent upgrade without database downtime: Docker owns the containers; the agent restarts without touching them. Requires Docker `live-restore` for daemon upgrades (not set by install today; to verify per distribution).
- Drain and move: databases are never moved automatically. Move is backup and restore (exists through `node_move_exec.go`) or a branch based move later; the wizard shows the write pause.
- What to measure: bundle apply time, local reconcile actions during outages, backup success while disconnected, buffer drop counts, endpoint verification failures, reconnect storm size.

### D. Placement step in the create database wizard

A Placement step after engine and version, before resources. Four choices:

1. This server. Default when no other node exists. Shows free disk and memory.
2. An existing node. A list with name, zone, free disk, memory headroom, database count, agent version, and a reason when a node is not eligible (cordoned, outdated agent, low disk). The Auto option uses the placement policy and shows the node it would pick.
3. New node on a cloud. Provider picker over connected accounts (AWS EC2, Azure VM, Google Cloud, Oracle Cloud, Hetzner, DigitalOcean, and Vultr as the provisioner already supports it). Shows region, size, disk size, estimated monthly cost from the live catalog, the price cap and server cap left, and a confirm box with the cost. One job then creates the server, installs the agent through cloud-init with a join token (exists), waits for enrollment, verifies the endpoint and deploys the database, with step progress and rollback.
4. Datacenter or any Linux host. Shows the install command with a join token (exists), waits for the node to appear, then continues the same flow. SSH provisioning (exists) is offered as an alternative.

CTAs, never a dead end:

- No cloud account connected: option 3 shows "Connect a cloud account" that opens the existing connect flow in a side panel and returns to the step.
- No other node: option 2 shows "Add a node" (opens `AddNodeWizard`) and points at option 4.
- Provider connected without create scope (for example a read only Oracle session account): the reason is shown with the fix.

Availability:

| Provider | Create node today | In this flow |
| --- | --- | --- |
| Hetzner, DigitalOcean, Vultr | Yes | Slice 3 |
| AWS EC2 | Yes, with preflight | Slice 3 |
| Oracle Cloud | Yes, API key accounts only, allowance check | Slice 3 |
| Azure, Google Cloud | Provisioner exists; real host proof: to verify | Behind the flag until proven |

Spend rules: never create a billable server without an explicit confirm that shows the monthly estimate, and never above `provision.max_monthly_price_usd` or `provision.max_servers` (both exist, live). A new `databases.placement.cloud_confirm` setting stays on and cannot be turned off for the new node path. Unknown price is refused unless `provision.allow_unknown_price`.

Teardown: deleting the database never deletes the cloud server unless the operator ticks "also delete the node I created for it", which is offered only when the node runs nothing else. The node keeps a `created_for` link; orphan detection lists provisioned servers with no workload for `provision.idle_alert_hours` and shows their monthly cost on the node and spend pages.

### E. Data model, API, CLI, IaC, settings, security

Data model (additive migrations only):

- Database: `endpoint_mode` (`local`, `private`, `mesh`, `public`), `endpoint_host` (resolved, response only), `placement_pool`, `placement_zone`, `placement_source` (`manual`, `auto`, `cloud_job`). `node_id` exists.
- Node: `public_address`, `private_address`, `endpoint_hostname`, `zone`, `accepts_database_workloads` (default true for nodes with Docker), capability report (Docker present, free disk, total memory), `bundle_version_applied`, `created_for`.
- New `database_placement_jobs`: id, database, provider, account, region, size, state, step, failure, server id, node id, created by, cost estimate.

Routes:

- `POST /api/v1/databases/placement/preview`: eligible nodes with reasons, Auto pick, and for a cloud choice the estimate and caps left.
- `POST /api/v1/databases/placement/jobs` and `GET .../jobs/{id}`: states `pending`, `creating_server`, `waiting_enroll`, `verifying_endpoint`, `deploying`, `ready`, `failed`, `rolling_back`, `rolled_back`. Rollback deletes the server only if this job created it and no workload landed.
- `POST /api/v1/nodes/{id}/endpoint/verify`.
- Agent protocol: `GetBundle`, `AckBundle`, `ReportLocalEvents`. Agents without the capability keep today's path.

CLI: `levelrail-cli databases create --node <name>`, `--pool`, `--zone`, `--endpoint private|mesh|public|local`, `--cloud aws --region <r> --size <s> --confirm-cost`. IaC: the same fields in the apply schema, with `cloud` placement refused in plan unless a cost confirm field is set.

Settings (runtime settings, live, env seeds defaults only):

| Key | Env seed | Default |
| --- | --- | --- |
| `databases.placement.min_free_disk_percent` | `APP_DB_PLACEMENT_MIN_FREE_DISK_PERCENT` | 20 |
| `databases.placement.max_per_node` | `APP_DB_PLACEMENT_MAX_PER_NODE` | 20 |
| `databases.endpoint.default_mode` | `APP_DB_ENDPOINT_DEFAULT_MODE` | `private` |
| `agent.local_reconcile_after` | `APP_AGENT_LOCAL_RECONCILE_AFTER` | 2m |
| `agent.metrics_buffer_mb` | `APP_AGENT_METRICS_BUFFER_MB` | 64 |
| `databases.tls.leaf_days` | `APP_DB_TLS_LEAF_DAYS` | 30 |
| `provision.idle_alert_hours` | `APP_PROVISION_IDLE_ALERT_HOURS` | 24 |
| `databases.placement.cloud_job_timeout` | `APP_DB_PLACEMENT_CLOUD_JOB_TIMEOUT` | 30m |

Authz and audit: placing a database on a node needs write on the database and use on the node (`nodeRef`, exists in `databases.go`). Creating a cloud node needs the existing cloud create grant plus a cost approval ability; a project owner below root sees the estimate and can request, a root or a grant holder confirms. Every job step is audited with the cost estimate.

Security:

- Blast radius: a compromised node exposes the databases on it and their credentials, nothing else. The bundle is per node and holds only that node's databases. The node cannot sign bundles.
- Identity: the agent identity and mTLS from enrollment (exists). The bundle signing key is separate from the agent CA and rotates with a two key window.
- Secrets: credentials in the bundle are encrypted to the node key; the control plane keeps no delivery copy after ack. With ADR 087 vault sources the node resolves references with a node scoped identity and policy path `levelrail/<project>/<database>`.
- Public mode requires TLS and a non empty allowlist; engines without auth stay loopback only (rule exists).
- Direct explorer tokens (later slice) are single use, short lived, bound to user, database and node, and audited on both sides.

## Alternatives rejected

- Proxy database connections through the control plane (one front door). Simple TLS and DNS, but it puts the control plane in every query path, doubles latency for remote nodes and turns every control plane restart into a database outage. Rejected, it is the opposite of the goal.
- Keep the control plane as the only reconciler and rely on Docker restart policies during outages. Covers a crash restart, not a recreate after an image or volume problem, and not backups. Kept as the fallback for agents without the bundle capability.
- Full desired state of the whole fleet on every agent. Simpler sync, but a node compromise would leak every database spec and the size grows with the fleet. Rejected for the per node bundle.
- ACME on every node as the only TLS issuer. Needs port 80 or DNS access per node and fails on private only nodes. Kept as an option for public hostnames.

## Failure modes

| Event | Expected behavior |
| --- | --- |
| Control plane down | Connections keep working at the node address. Local reconciler restarts or recreates from the cached bundle after `agent.local_reconcile_after`. Backups run and upload from the node. Metrics buffer. Explorer, new databases, moves and alerts other than the silence watcher are unavailable. |
| Node down | Its databases are down. Nothing is moved automatically. Apps on other nodes get connection errors. Read replicas elsewhere keep serving reads. The UI shows the node offline with its databases. |
| Network partition (node alive, link to control plane gone) | Same as control plane down for that node. On reconnect the newer bundle version wins, local events are replayed, no double apply because steps are idempotent by version. |
| Cloud API failure mid job | Job moves to `rolling_back`, deletes only a server it created, keeps counting a server it could not confirm deleted against the cap (exists), and shows the provider tag for manual cleanup. |
| Disk full on node | Placement refuses new databases below the free disk floor. Existing disk alert fires. Local backups that cannot stage fail with a clear reason and do not delete older copies. |
| Agent upgrade | Containers keep running. The new agent reloads the bundle and verifies it before acting. A Docker daemon upgrade without `live-restore` stops containers: called out in doctor. |
| Clock skew | Bundle validity uses versions, not wall time. Certificate validity has a skew margin. Doctor warns above a skew threshold reported in heartbeats. |
| Bundle signature fails | Agent keeps the last good bundle, refuses the new one, reports it. |
| Certificate near expiry during a long outage | Leaf validity is longer than the outage budget; the agent warns locally at 7 days. |

## What cannot be verified locally

- Real cloud node creation and cloud-init enrollment for Azure and Google Cloud (no proof run recorded here).
- Direct connection from outside a cloud VPC to a node's public address with the allowlist in force.
- Behavior of Docker daemon upgrades without `live-restore` on each distribution.
- Remote backup throughput when the dump uploads from the node instead of through the control plane.
- Fleet scale numbers (hundreds of nodes); only estimates exist.

## Testing plan

- Unit: bundle signing, versioning and refusal of older or unsigned bundles; placement preview reasons; job state machine transitions and rollback rules; endpoint host resolution per mode.
- Agent: local reconciler with a fake runtime, control plane silent, container exits, recreate from bundle, no action while a session is live.
- API: preview and job routes with fake provisioners (exists for node provision), cost confirm required, caps enforced.
- Web: Placement step with zero nodes and zero clouds shows both CTAs; eligible and ineligible nodes render reasons.
- Real host: one database on a remote test node, connect with a client at the node address, stop the control plane, keep querying, kill the container, see the agent recreate it.

## Slices

| # | What | Area | Test | Risk | Depends | Release |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | Direct endpoint: node `public_address` and `private_address`, `endpoint_mode` on databases, connection strings and snippets use the node endpoint, endpoint verify route | `internal/store`, `internal/api/databases.go`, `database_connect_snippets.go`, `database_public_access.go`, migrations | API test per mode | Low | none | 1.5.11 |
| 2 | Placement step with existing nodes, Auto pick and CTAs | `web/src/components/CreateDatabaseFields.tsx` and a new placement step file, `POST .../placement/preview` | Web test with zero and many nodes | Low | 1 | 1.5.11 |
| 3 | Cloud node create inside the database flow behind the experimental flag: placement job, cloud-init, wait, verify, deploy, rollback | `internal/api` new placement job files, reuse `node_provision.go` and guard | Fake provisioner job test | Medium | 1, 2 | 1.5.11 |
| 4 | Datacenter choice in the step: install command, wait for node | web step, `node_join_command.go` | Web test | Low | 2 | 1.5.11 |
| 5 | Database placement policy (free disk floor, per node cap, accepts databases) | `internal/placement`, `internal/api/databases.go` | Unit | Low | 2 | 1.5.11 |
| 6 | Node capability report (Docker, disk, memory) | `internal/agent/host_report.go`, store | Unit | Low | none | 1.5.12 |
| 7 | Signed per node bundle, pull and ack | `internal/agent`, `agentpb`, `cmd/levelrail` | Unit | Medium | 6 | 1.5.12 |
| 8 | Agent local reconciler for databases from the bundle | `internal/agent`, shared spec from `internal/reconcile/database` | Agent fake runtime test | High | 7 | 1.5.12 |
| 9 | Credentials encrypted to the node key, drop control plane delivery copy | `internal/agent/pki.go`, `internal/secrets` | Unit | High | 7 | 1.5.12 |
| 10 | Local backup runner and direct upload from the node | `internal/backup`, `internal/agent` | Fake target test | Medium | 7 | 1.5.12 |
| 11 | Remote PITR WAL shipping | `internal/backup/wal_ship_scheduler.go`, agent | Unit | Medium | 10 | later |
| 12 | Metrics buffer cap and replay for database metrics | `internal/agent/offline_send.go` | Unit | Low | none | 1.5.12 |
| 13 | Per endpoint TLS from a database CA, delivered in the bundle | `internal/reconcile/database/tls.go`, agent | Unit | Medium | 7 | 1.5.12 |
| 14 | Agent owned firewall allowlist for public mode | `internal/hostfw`, agent | Unit | Medium | 7 | 1.5.12 |
| 15 | Zone label as placement input, replica anti affinity | `internal/placement/zones.go`, store | Unit | Low | 5 | 1.5.12 |
| 16 | Orphan cloud node detection and teardown option | `internal/api/node_provision*.go`, web | Unit | Low | 3 | 1.5.12 |
| 17 | CLI and IaC fields | `cmd/levelrail-cli`, IaC schema | CLI test | Low | 1, 3 | 1.5.12 |
| 18 | Direct explorer session to the agent | `internal/agent`, web | Unit | High | 7 | later |
| 19 | Heartbeat coalescing and batched stats | `internal/agent/server.go`, store | Unit | Medium | none | later |

The first three are the smallest proof: a database on an existing remote node reachable at the node address with no control plane proxy, the Placement step with existing nodes and CTAs, then the cloud node flow behind the experimental flag. Slices 1 to 3 touch different files and can run in order with one writer each.

## Consequences

- Database connections stop depending on the control plane once slice 1 lands, which is mostly already true for host port and mesh binds; the change makes it visible and verifiable.
- The agent grows from a remote runtime into a small reconciler for databases. That is new code on every node and needs the same spec code as the control plane to avoid drift.
- Placement becomes a user choice with cost attached; spend guards already exist and are reused, not duplicated.
- Operators get a clear place to start a database on a cloud machine without leaving the wizard.

## Open questions

| Question | Recommended default |
| --- | --- |
| Who may approve cloud cost for a database node? | Root or a holder of the cloud create grant; others can request. |
| Default endpoint mode? | `private` (RFC1918), public only with allowlist and TLS. |
| Who owns DNS names for endpoints? | Optional `endpoint_hostname` set by the operator; Levelrail manages the record only when the zone is connected (ADR 086). |
| Replica scope in this ADR? | Placement rules only; replication setup and failover in a separate ADR. |
| Should the local reconciler be on by default? | Off until slice 8 has a real host proof, then on for new agents. |
| Should Azure and Google Cloud show in the step before a real host proof? | Shown behind the experimental flag only. |
| Delete the cloud node with the database by default? | No, opt in per delete and only when the node runs nothing else. |
