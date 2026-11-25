# ADR 085: Managed and hosted databases managed from Levelrail

Status: Proposed

## Context

Owners already run databases outside Levelrail: AWS RDS and Aurora, ElastiCache, DocumentDB, DynamoDB, Oracle Cloud Autonomous Database and MySQL HeatWave, a serverless Postgres account, or any hosted database reachable by a connection string (Atlas, Supabase, Upstash, DigitalOcean and Hetzner managed databases). They want to connect the account once, see those databases in the same Databases list as local ones, use the explorer, access audit, env wiring, backups view and monitoring on them, and optionally create and delete them from Levelrail. They also want Oracle Database Free as a local engine, and the UI should always say where a database runs.

What exists on `release/v1.5.11`, read in code:

External databases (`internal/extdb`, `internal/api/external_databases*.go`, `internal/store/external_database.go`)

- A record holds name, engine, host, port, user, database, TLS mode, Docker network, node id, project id, source container and the last health result. The password lives in `internal/secrets` under `external-database/<name>`; the wire shape only exposes `has_password`.
- Engines: postgres, mysql, mariadb, mongodb, redis (`extdb.Supported`). The viewer target is SQL only (postgres, mysql, mariadb).
- TLS modes are `disable`, `prefer`, `require` only. There is no `verify-ca` or `verify-full` and no CA bundle field, so a hosted database is never certificate verified today.
- Address policy (`extdb.Conn.Validate`, `CheckResolved`): loopback, unspecified, multicast, link local and cloud metadata addresses are refused, metadata opt in through `APP_EXTERNAL_DB_ALLOW_LINK_LOCAL`. Private ranges are allowed. A failed DNS lookup is treated as allowed (container names), and the resolved address is not pinned: the helper container resolves the name again, so a rebinding host can pass the check and then point elsewhere.
- Every probe and viewer call runs the engine client inside a short lived helper container (`extdb.Helper`, `sleep 300`, removed on close). The password reaches the helper as an environment variable, so anyone with Docker inspect rights on that host sees it during the helper lifetime.
- `execRuntimeFor(nodeID)` already picks the Docker runtime of a node, so a probe and the viewer can run from a chosen node. This is the existing network position mechanism; the UI does not present it as such.
- The unified list already exists: `GET /api/v1/databases` appends external records (`appendExternalDatabases`) with an `external` sub object.

Cloud accounts and AWS database services (`internal/api/cloud_*.go`, `internal/awsx/*`, `internal/cloudguard`, `internal/cloudspend`)

- Two account concepts exist side by side: `cloudconn` connections (`store.CloudConnection`, provider registry, read only `Provider` interface with `Check` and `ListRegions`, Oracle and the others registered as not implemented except AWS) and cloud accounts (`store.CloudAccount`, used by every AWS service route under `/api/v1/cloud-accounts/{id}/aws/...`, with sealed credentials, SSO, Vault and role assumption).
- RDS already has list, get, clusters, snapshots, price estimate, create (instance and cluster), stop, start, snapshot, adopt (tags), modify, delete, and `connect-as-database` (`internal/api/cloud_rds.go`). Connect as database records the endpoint as an external database with `tls_mode=require` but copies no password: the master password stays in Secrets Manager, so the record is unusable for the explorer until someone types the password.
- ElastiCache has read, reboot, snapshot and delete routes. There is no DocumentDB, DynamoDB or MemoryDB client in `internal/awsx`.
- `cloudguard` gates every AWS create: kill switch, monthly cap per account (with per account overrides), per create maximum, instance type allowlist, services enabled, mandatory tags (`managed`, `owner`, `env`, `project`, `id`, optional `expires` from a default TTL). Settings are live runtime settings with env seeds (`APP_CLOUD_GUARD_*`). RDS refuses multi AZ and Aurora without an explicit confirm and refuses a create without a stop or teardown timer. Mutations act only on resources carrying the managed tag (`requireManaged`).
- IAM capability tiers (`internal/cloudconn/aws_capabilities.go`): inventory (read), deploy (create, modify, start, stop, tag scoped), destructive (delete, tag scoped), cost, secrets. Each tier is a policy document the owner attaches, with simulation and dry run probes, and `CapabilityForAction` maps a missing action to a tier.
- Oracle: `cloudconn.OracleProber` signs requests with the stored key and probes instance listing, usage and budgets. Nothing reads Autonomous Database or MySQL HeatWave.
- There is no approval flow (a second person approving a billable create) anywhere in the cloud routes.

Related decisions: ADR 081 (branches are normal databases plus one row), ADR 082 (one resolver `dbauthz.Resolve`, `database-kind:external`, TLS verify modes, private range setting `deny`, `allow_listed`, `allow`, pinned resolved address).

## Decision

Recommendation in one paragraph: hosted databases are external database records with a provider adapter attached. One small `dbprovider.Adapter` interface (discover, status, endpoint, capabilities, metrics, snapshots, then provision and delete) sits behind a registry keyed by provider kind; the generic connection string adapter is the fallback and the first one built. Adapters reuse the existing cloud account credentials and never store their own. Every outbound database connection goes through the ADR 082 address policy with a pinned address, and runs from a chosen network position (the control plane or a node), which is also how the explorer reaches private VPC databases. Read only discovery and adoption ship first for every provider; provisioning extends the RDS path that already exists, behind the existing guard plus a dry run plan, a typed confirm and an approval step, and Oracle Cloud provisioning comes last. Oracle Database Free becomes a local engine only after its image, licence, arm64 and memory facts are verified on a throwaway container.

### 1. Data model: one list, a location object

Keep two tables (managed databases and external databases); add columns to the external record instead of a third table:

- `provider` (kind: `generic`, `aws-rds`, `aws-docdb`, `aws-elasticache`, `aws-dynamodb`, `oci-adb`, `oci-mysql`, `serverless-pg`), `provider_ref` (ARN, OCID or provider resource id), `cloud_account_id`, `region`, `adopted_at`, `managed_by` (`levelrail` when Levelrail created it and the managed tag is present, `external` otherwise), `tls_verify`, `ca_bundle_ref`, `network_position` (empty for the control plane, otherwise a node id; this renames the meaning of the existing `node_id` without moving data).
- The list API returns a computed `location` object for every database, local or not:

```json
{"kind": "this_server|node|cloud|hosted", "node_id": "", "provider": "aws-rds", "account": "prod-aws", "region": "eu-west-1", "reached_from": "node-2"}
```

`this_server` and `node` come from managed placement; `cloud` means an adapter with a cloud account; `hosted` means the generic adapter. The UI badge reads "this server", "node-2", "AWS eu-west-1 (prod-aws)" or "hosted". The create flow (I02) uses the same object as its "where it runs" step.

### 2. Provider adapter interface and registry

New package `internal/dbprovider`:

```go
type Adapter interface {
    Kind() string
    Capabilities() Caps // engines, discover, metrics, snapshots, provision, delete, branches
    Discover(ctx context.Context, a Account, region string) ([]Found, error)
    Status(ctx context.Context, a Account, ref Ref) (Status, error)
    Endpoint(ctx context.Context, a Account, ref Ref) (Endpoint, error) // host, port, engine, TLS hints, credential reference
    Metrics(ctx context.Context, a Account, ref Ref, q MetricQuery) ([]Series, error)
    Snapshots(ctx context.Context, a Account, ref Ref) ([]Snapshot, error)
}

type Provisioner interface { // optional, asserted at runtime
    Plan(ctx context.Context, a Account, req ProvisionRequest) (Plan, error) // dry run: price, IAM actions, tags, refusals
    Provision(ctx context.Context, a Account, p Plan, confirm Confirm) (Ref, error)
    Delete(ctx context.Context, a Account, ref Ref, confirm Confirm) error
}
```

- `Registry` maps kind to adapter; unknown capabilities return `ErrUnsupported`, which the UI shows as a disabled control with the reason, never a hidden one.
- The AWS adapters wrap `internal/awsx/rds` and `internal/awsx/elasticache`; they do not copy their logic. RDS provisioning stays in `awsx/rds` behind `cloudguard`; `Provision` only adds the plan and confirm envelope.
- `Account` is a reference to a `store.CloudAccount` resolved by the API layer; adapters receive a signed client, never raw keys.
- The generic adapter implements `Endpoint` and `Status` (the existing probe) only. Provider hints for Atlas, Supabase, Upstash, DigitalOcean and Hetzner are data (default port, TLS mode, CA bundle source, connection string shape), not separate adapters. A provider gets real code only when it has an API worth calling (the serverless Postgres API, AWS, Oracle Cloud).
- The serverless Postgres provider adapter authenticates with an API key stored as a cloud account of a new kind; discovery lists projects, branches and endpoints and adopts them with pooled and direct URLs.

### 3. Secrets and IAM

- Cloud credentials stay where they are: sealed cloud account values, SSO sessions, Vault and role assumption. Adapters never persist a credential. The serverless Postgres API key becomes a cloud account (sensitive setting, root only, step up), not an external database field.
- Database passwords: for an adopted RDS or Aurora instance, the record stores a credential reference (`secretsmanager:<arn>`) instead of a copy. The helper resolves it at use through the account's `secretsmanager:GetSecretValue` on that ARN only. This needs a new, narrow statement in a database read tier (`rds!*` secrets), because the current secrets tier grants get and put only under the Levelrail name prefix. Until that tier is attached, adoption asks for a password and stores it under `external-database/<name>` as today.
- IAM auth tokens (RDS IAM database authentication) are preferred where the engine supports them: short lived, nothing stored. Built after the password path, flag off.
- IAM tiers: add a `databases-read` tier (describe RDS, DocumentDB, ElastiCache, DynamoDB tables, CloudWatch metrics for those namespaces, list tags) and keep create, modify and delete in the existing deploy and destructive tiers, tag conditioned. Each provision action lists the exact actions it needs in its plan; `CapabilityForAction` turns a denied action into "attach tier X".
- Oracle Cloud: the same split as policy statements per compartment (read: `inspect` and `read` on `autonomous-databases` and `mysql-family`; create and delete in separate statements), shown as copyable policy text like the AWS tiers.

### 4. Outbound connections, SSRF and private ranges

All of ADR 082 applies, and this ADR makes it the only path:

- Every adapter endpoint and every generic connection string goes through `extdb.Conn.Validate` and `CheckResolved` before any credential is sent.
- Pinned address: the control plane resolves once, checks every address, and passes the chosen IP to the helper with the hostname kept for TLS SNI and verification. The helper never resolves the name itself. This closes the rebinding gap described in Context.
- A lookup failure is allowed only when the host is a container on a Docker network the record names; any other unresolvable host is refused.
- Private ranges follow `databases.external.private_ranges` (`deny`, `allow_listed`, `allow`) from ADR 082. Cloud adapters add one rule: a private endpoint returned by the provider API for an adopted resource is added to the allow list automatically, scoped to that record, because the operator did not type it and the provider vouched for it. Typed private hosts still need the setting.
- Credentials are sent only to the saved origin of a record; a test probe with typed credentials is not stored and not retried.

### 5. TLS verify modes and CA bundles

- Add `verify-ca` and `verify-full` to `extdb` TLS modes, plus a `ca_bundle_ref` that names either a shipped bundle or a secret.
- Shipped bundles are data files with a source URL and a fetch date, refreshed by a script, not at runtime: the AWS RDS global bundle (covers RDS, Aurora and DocumentDB), the Oracle Cloud wallet path for Autonomous Database (a wallet per database, downloaded through the API and stored as a secret, or the TLS without wallet mode where the database allows it), and the system roots for providers with public CA certificates.
- Defaults: cloud adapters default to `verify-full` with the provider bundle; the generic adapter defaults to `verify-full` with system roots when the host is public and `require` otherwise, and the doctor flags `disable` off loopback (ADR 082). The helper image must carry the bundle; it is mounted read only into the helper, not baked into user images.

### 6. Network position and reach into private VPC databases

The control plane often cannot reach a database in a private subnet. The answer is to run the client where it can reach:

1. Node as network position (recommended, first): a Levelrail node inside the VPC (an EC2 or OCI instance already joined) runs the helper. The record's `network_position` names the node; probes, explorer execs and metrics that need a session run there through the existing `execRuntimeFor`. The UI asks "reach it from" with the nodes that can resolve and connect, tested by a reachability probe per node.
2. SSH jump host: later, flag off. A port forward opened by the control plane for the lifetime of one helper call, host key pinned, key in secrets. Only for owners without a node in the VPC.
3. Provider private networking (VPC peering, PrivateLink, OCI private endpoints): documented, not automated. Levelrail never changes VPC routing or security groups to make a database reachable.

The explorer, audit, and ADR 082 resolver are the same whatever the position; the audit row records `reached_from`.

### 7. Monitoring, backups view, env wiring

- Metrics come from the provider (CloudWatch `GetMetricData` for RDS, DocumentDB, ElastiCache, DynamoDB; OCI Monitoring for Autonomous Database and MySQL HeatWave) through the adapter, cached with a TTL from `APP_DBPROVIDER_METRICS_TTL_SECONDS` (default 120) and stored in the existing rollup tiers (ADR 080) only when the owner turns on retention. The generic adapter has the probe latency only.
- Backups view lists provider snapshots next to Levelrail backups, labeled by source. Restore as new into the cloud is a provisioning action and comes later.
- Env wiring binds an external database to an app like a managed one: the app receives the URL at deploy, resolved from the credential reference at that moment, pooled and direct where the provider has both.

### 8. Provisioning (billable), after read only is proven

Order of gates for every create, resize and delete:

1. Plan (dry run): price estimate (`cloudspend` price lookup; refuse when unavailable and a per create maximum is set, as RDS does today), exact IAM actions, tags, network placement, and every `cloudguard` refusal, shown before the confirm button exists.
2. Guard: existing `cloudguard` (kill switch, caps, per create maximum, allowlists, mandatory tags, expires timer).
3. Typed confirm: the resource id typed by the user for create, delete, multi AZ and Aurora.
4. Approval (new): when `databases.cloud.approval_required` is on (default on for create and delete), the plan is saved as a pending request; a second user with root, or the same root after step up when the install has one admin, approves it within `APP_DBPROVIDER_APPROVAL_TTL_MINUTES` (default 60). The executed call must match the approved plan hash.
5. Ownership: mutations only on resources with the managed tag, as `requireManaged` does. Adopting an existing resource adds tags only when the owner asks; adoption without tags is read only from Levelrail.
6. Drift: a sweep compares tags, class, storage and status against the record and shows drift; it never fixes drift on its own. A resource deleted outside Levelrail turns the record into "gone at provider" and keeps it until the owner removes it.

AWS RDS and Aurora first (code exists, the envelope is new). DocumentDB and ElastiCache create later. DynamoDB create later and on demand billing only. Oracle Cloud create last, with a free tier check (Always Free Autonomous Database count per tenancy) in the plan.

### 9. Oracle Database Free as a local engine

Gated on verification on a throwaway container (pull, start, connect, check memory and architecture, remove). Facts to confirm, none verified in this ADR:

- Image path: expected under `container-registry.oracle.com/database/free`, tag per release. Confirm the path, whether pulling needs an account or licence acceptance, and the current release name.
- Licence: Oracle Free Use Terms and Conditions, which as understood allow development and production use with resource limits and restrict redistribution. We never redistribute the image: the user's host pulls it from the vendor registry, like MongoDB today. The engine card shows the licence name and link, and the first create needs a "I accept the vendor terms" checkbox recorded in the audit.
- Limits as understood: about 2 CPU threads, 2 GB RAM and 12 GB user data enforced by the database itself. Confirm, and set the engine card minimum memory from `APP_ORACLE_FREE_MIN_MEMORY_MB` (default 2048).
- arm64: confirm a native `linux/arm64` image exists for the current release; if not, the engine is hidden on arm64 hosts instead of running under emulation.
- Explorer: `sqlplus` is in the image per the vendor docs; the explorer runs it through the existing exec model with the same statement guard. SQLcl is a separate download and is not assumed.

## Alternatives rejected

- A third table for cloud databases: splits list, authz and audit code three ways. A provider column on the external record keeps one path.
- One adapter per hosted provider with a connection string only (Atlas, Supabase, Upstash, DigitalOcean, Hetzner): code without an API to call. Hint data is enough.
- Copying the RDS master password into Levelrail secrets at adoption: two copies drift on rotation, and Levelrail becomes a second store of a cloud owned secret. A reference resolved at use avoids both.
- Running the explorer driver in the control plane process: breaks the property that the control plane never holds an open session with database credentials, and cannot reach private subnets anyway.
- Auto creating security group rules or VPC peering so the control plane can reach a database: changes the owner's network on their behalf, high blast radius. A node in the VPC is the network position instead.
- Automatic drift correction: a tag or class change made in the provider console may be deliberate.
- Provisioning before discovery: no create path ships for a provider until its read only adapter has run against a real account.

## Security

- Authz: every new route goes through `dbauthz.Resolve` (ADR 082) with `database-kind:external`, plus the cloud account check `requireCloudAccess` for any route that calls a provider. Abilities: discover and view (`view`), adopt (`operate`), provision (`build` plus approval), delete (`delete` plus typed confirm plus approval). Each route has an audit row and a deny test for a team scoped caller and a viewer.
- SSRF: pinned addresses, metadata always refused, private ranges per setting, provider returned private endpoints allowed per record only.
- Secrets: credential references, not copies; helper env exposure stays a known risk (Docker inspect on the helper host during the call). A follow up passes the password on stdin or a tmpfs file instead of env; tracked as a slice, not claimed here.
- Provider output is never shown raw: errors map to fixed classes as `cloudconn` does today.
- Cost: no create without a plan, guard pass, typed confirm and, by default, approval. Kill switch stops all creates.
- Explorer on cloud databases runs as the stored user; ADR 082 broker mode stays off by default for external databases.

## Failure modes

- Provider API throttled or down: discovery and metrics show "provider unavailable" with the last good value and its age; the database itself is still probed directly.
- Credential expired (SSO session, rotated key): the account shows `auth_failed`; records keep their last status, the explorer still works if the database password is stored locally, and fails with a clear reason if it is a reference.
- Endpoint changes (failover, restore to a new instance): the sweep refreshes the endpoint from `Endpoint`; the pinned address is re-resolved on every call, never cached across calls.
- Network position node offline: probes and explorer fail with "node not reachable"; the record suggests other nodes that passed the reachability probe.
- Price unavailable: create refused when a per create maximum is set.
- Approval expires or plan changed: the create is refused, a new plan is needed.
- Resource deleted outside Levelrail: record marked gone, never recreated.
- Two Levelrail installs adopting the same resource: the managed tag carries the install id; a second install sees "managed by another install" and stays read only.
- Oracle Database Free image pull needs licence acceptance or login: the create fails with that reason and a link, nothing retried.

## What cannot be verified locally

- Every real cloud call: RDS, Aurora, DocumentDB, ElastiCache, DynamoDB, CloudWatch, Secrets Manager reference resolution, IAM auth tokens, Oracle Autonomous Database wallets, OCI Monitoring, MySQL HeatWave, the serverless Postgres API. Unit tests use fakes (`awsxtest`, `cloudconn` fakes); fakes prove request shape, not provider behavior.
- Private VPC reach through a node in that VPC.
- Real prices against the plan estimate, and spend cap behavior over a billing day.
- CA bundles against real endpoints (`verify-full` with SNI on a pinned IP).
- Oracle Database Free image path, licence flow, arm64 image and memory use. No container was started for this ADR.
- The rebinding fix needs a hostile DNS server to prove; a local fake resolver test is the closest local check.

## Testing plan

- Unit: adapter registry and capability table; generic adapter against the existing probe; plan hashing and approval match; pinned address passed to the helper (fake resolver that changes answers between calls); private range setting with provider returned endpoints; TLS mode validation including `verify-full` without a bundle; location object for every database kind; drift comparison.
- API: route authz deny tests (team scoped caller, viewer, expired step up), audit rows, typed confirm and approval refusals, `ErrUnsupported` mapped to a disabled reason.
- Fakes: RDS, ElastiCache, CloudWatch and Secrets Manager reference through `awsxtest`; OCI signed requests through the existing Oracle test server pattern.
- Real cloud proofs (owner approved, smallest sizes, budget guard set, teardown timer on, resources removed the same session): listed per slice below.
- Oracle Database Free: one throwaway container on amd64 and one on arm64, recorded facts, container and volume removed.

## Slices

Order is top to bottom. Env names seed live runtime settings; every tunable is changeable live through API, CLI and dashboard. SEC means lr-reviewer before merge. PROOF means an owner approved real cloud or real host proof before the flag can default on.

| # | Slice | Files (ownership) | Settings and env | Flags |
| --- | --- | --- | --- | --- |
| 1 | Location object and provider columns on external records, list API and badge | `internal/store/external_database.go` and migration, `internal/api/databases.go`, `internal/api/external_databases_list.go`, `web/src/components/databases` badge | none | |
| 2 | `internal/dbprovider` interface, registry, generic adapter with provider hint data (Atlas, Supabase, Upstash, DigitalOcean, Hetzner) | `internal/dbprovider/*` (new), `internal/dbprovider/hints.yaml` | none | |
| 3 | Pinned resolved address and strict lookup failure rule in the helper | `internal/extdb/conn.go`, `internal/extdb/helper.go`, `internal/extdb/probe.go` | none | SEC |
| 4 | Private range setting wired from ADR 082 plus provider returned endpoint rule | `internal/extdb/conn.go`, `internal/api/external_databases.go`, runtime settings key | `APP_EXTERNAL_DB_PRIVATE_RANGES` (seed `deny` new, `allow_listed` on upgrade) | SEC |
| 5 | TLS `verify-ca` and `verify-full`, CA bundle refs, shipped AWS bundle with refresh script | `internal/extdb/conn.go`, `internal/extdb/helper.go`, `internal/extdb/cabundles/*`, `scripts/refresh-ca-bundles.sh` | `APP_EXTERNAL_DB_CA_DIR` | SEC, PROOF (RDS endpoint) |
| 6 | Network position: "reach it from" node picker, reachability probe per node, audit `reached_from` | `internal/api/external_databases.go`, `web/src/components/databases` settings networking | `APP_DBPROVIDER_REACH_TIMEOUT_SECONDS` (default 5) | PROOF (node inside a VPC) |
| 7 | AWS discovery adapter: RDS, Aurora, ElastiCache read and adopt, `databases-read` IAM tier, credential reference to Secrets Manager | `internal/dbprovider/aws/*` (new), `internal/cloudconn/aws_capabilities.go`, `internal/api/cloud_rds.go` (connect as database only) | none | SEC, PROOF |
| 8 | Provider metrics (CloudWatch) with TTL cache and optional rollup retention | `internal/dbprovider/aws/metrics.go`, `internal/awsx/cloudwatch`, `web` monitoring panel | `APP_DBPROVIDER_METRICS_TTL_SECONDS` (default 120) | PROOF |
| 9 | Backups view: provider snapshots next to Levelrail backups | `internal/api/external_databases_backups.go` (new), `web` backups panel | none | |
| 10 | Env wiring of external records to apps with reference resolution at deploy | `internal/api` app bindings for external databases, `internal/deploy` env resolution | none | SEC |
| 11 | Provision envelope for RDS and Aurora: plan, typed confirm, approval with plan hash, drift sweep | `internal/dbprovider/plan.go`, `internal/api/cloud_rds.go`, `internal/api/dbprovider_approvals.go` (new), store table for pending plans | `APP_DBPROVIDER_APPROVAL_TTL_MINUTES` (default 60), `APP_DBPROVIDER_DRIFT_INTERVAL_MINUTES` (default 30) | SEC, PROOF (billable) |
| 12 | DocumentDB and DynamoDB read adapters (DynamoDB explorer is lane D) | `internal/awsx/docdb` and `internal/awsx/dynamodb` (new), `internal/dbprovider/aws/*` | none | SEC, PROOF |
| 13 | Oracle Cloud adapter: Autonomous Database and MySQL HeatWave discover, adopt, wallet as secret, OCI Monitoring metrics | `internal/dbprovider/oci/*` (new), `internal/cloudconn/oracle_capabilities.go` | `APP_OCI_WALLET_MAX_BYTES` | SEC, PROOF |
| 14 | Serverless Postgres adapter via API key: projects, branches, endpoints, pooled and direct URLs | `internal/dbprovider/serverlesspg/*` (new), cloud account kind | `APP_DBPROVIDER_HTTP_TIMEOUT_SECONDS` (default 15) | SEC, PROOF |
| 15 | Oracle Database Free local engine, after verification | `internal/store/database_engines.yaml`, `internal/reconcile/database`, `internal/dbviewer` sqlplus runner | `APP_ORACLE_FREE_MIN_MEMORY_MB` (default 2048) | SEC, PROOF (amd64 and arm64 host) |

Slices 1 to 6 touch shared `internal/extdb` and `internal/api/external_databases.go` files and run in sequence. Slices 7, 12, 13, 14 live in separate adapter packages and can run in parallel once slice 2 lands. Provisioning beyond RDS (DocumentDB, ElastiCache, DynamoDB, Oracle Cloud creates) is not in this list on purpose.

## Consequences

- Hosted and cloud databases show in one list with one explorer, audit and authz path; providers are thin.
- The external database record grows; migrations must keep existing records working with `provider=generic`.
- TLS verification becomes real for hosted databases, which will break some existing records that worked with `prefer` against bad certificates; the upgrade keeps their mode and the doctor flags them.
- A node inside a VPC becomes the recommended way to reach private databases, which owners must be told plainly.
- Approval adds friction to every billable create, by design; single admin installs use step up as the second factor.
- Shipped CA bundles need periodic refresh in releases.
- The two cloud account concepts (`cloudconn` connections and cloud accounts) stay separate in this ADR; adapters use cloud accounts only.

## Open questions

1. Private range default: ADR 082 recommends `deny` on new installs, while an earlier working default kept private ranges allowed. Which one ships for 1.5.11?
2. Approval: is a second human required for billable creates, or is step up by the single root enough by default?
3. Secrets Manager references need `GetSecretValue` on `rds!*` secrets in a new read tier. Accept that grant, or keep typed passwords only?
4. Should `cloudconn` connections and cloud accounts merge into one model, and when?
5. Oracle Database Free: accept the vendor terms checkbox flow, and hide the engine on arm64 if no native image exists?
6. Provider metrics retention in Levelrail storage: off by default, or on with the rollup tiers?
7. Which real cloud proofs does the owner approve now (RDS read and adopt, node in VPC, RDS create with the guard, Oracle Autonomous Database read, serverless Postgres read)?
