# ADR 084: DynamoDB as a local engine and a hosted connection

Status: Proposed

## Context

Teams that ship on AWS want DynamoDB in two places: a local engine for development and preview environments, and the real service for staging and production, with the same explorer over both. Levelrail has neither today. This ADR fixes the shape for both, the explorer, how apps get their config, and how it fits ADR 081, 082 and 083.

What exists on `release/v1.5.11`, read in code:

- Managed engines are a fixed list in `internal/store/database.go` (`EnginePostgres` through `EngineMemcached`). Image, tag and port come from `internal/reconcile/database/address.go` (`ImageRef`, `ImageTag`, `ContainerPort`).
- The latest version resolver (`internal/dbupgrade/latest.go`, called from `internal/api/database_latest.go`) lists tags anonymously from the engine's fixed image repository, accepts only plain numeric tags (`plainTag`, so `latest` and suffixed tags are rejected), keeps tags on supported lines from `catalog.json` (`supportedPrefixes`), caches per engine (TTL and lookup timeout are runtime settings) and reports a newer unknown major as `SkippedNewer` without picking it.
- The explorer runs the engine client inside the database container through exec (`internal/dbviewer`), so the control plane never holds engine passwords. Limits come from env seeded settings (`APP_DB_QUERY_TIMEOUT`, `APP_DB_QUERY_MAX_ROWS`, `APP_DB_QUERY_MAX_BYTES`, `APP_DB_QUERY_MAX_CELL_BYTES` in `limits.go`). Mongo has its own request guard (`mongo_guard.go`).
- External databases (`internal/extdb`) validate addresses against a policy that blocks link local and cloud metadata ranges unless `APP_EXTERNAL_DB_ALLOW_LINK_LOCAL` is set, and never write to the remote.
- AWS cloud connections (`internal/cloudconn/aws.go`) support role assumption with an external id, secret by reference, Vault AWS, OIDC federation and static keys (static keys refused unless `APP_CLOUD_ALLOW_STATIC_KEYS`). Sessions last `APP_CLOUD_AWS_SESSION_SECONDS` (900 default) and carry the Levelrail user into CloudTrail through `sessionName`. Grantable permission tiers live in one registry (`aws_capabilities.go`).
- `internal/awsx` is an in house SigV4 client that adds no SDK service modules. It has JSON 1.1 and Query protocols, retries, a response byte cap, and a redacting `Secret` type. CloudWatch reads go through `internal/awsx/cloudwatch` with a cache because reads are billed. `JSON()` hard codes `application/x-amz-json-1.1`; DynamoDB speaks JSON 1.0, so this needs a small change.
- Step up grants (`internal/api/step_up.go`) are scoped by session, action (`reveal`, `export`, `manage`) and a scope string.

Verified on this machine on 2026-11-25 with a throwaway container that was removed afterwards:

- Image `amazon/dynamodb-local` on Docker Hub. Newest plain tag `3.3.2` (2026-10-15), also `latest`. Every tag listed back to `1.21.0` is a multi arch OCI index with `linux/amd64` and `linux/arm64` (plus attestation entries). Tags `1.2x` are a separate older line still receiving patches (`1.25.1` in 2024-11).
- Image size about 239 MB, user `dynamodblocal` (uid 1000), workdir `/home/dynamodblocal`, entrypoint `java`, default command `-jar DynamoDBLocal.jar -inMemory`, exposes `8000/tcp`, no `HEALTHCHECK`, no declared volume. OpenJDK 17 and `/usr/bin/curl` are present in the image.
- `-sharedDb -inMemory` started in a few seconds and idled at about 157 MiB resident.
- An unsigned GET on `/` returns HTTP 400 `MissingAuthenticationToken`. A request with any well formed SigV4 `Authorization` header and dummy key is accepted (`ListTables` returned `{"TableNames":[]}`). There is no real authentication.
- `-dbPath` on a fresh named volume fails: the volume is owned by root, the server logs `SQLiteException: unable to open database file` and then holds client requests open with no reply. After a one shot `chown 1000:1000` on the volume (same image, `--user 0 --entrypoint chown`), it creates `shared-local-instance.db` and serves requests.
- Table names shorter than 3 characters are rejected, as in the real service.

Not verified locally: arm64 behaviour under load, memory under real data, PartiQL coverage of the local engine, and whether `ItemCount` and `TableSizeBytes` are computed live by the local engine.

## Decision

Recommendation: add `dynamodb` as a managed engine that runs the unmodified `amazon/dynamodb-local` image with `-sharedDb` and disk persistence by default, explored by exec of `curl` inside the container with dummy credentials; and add DynamoDB as a hosted connection that rides on an existing AWS cloud connection, called from the control plane through `internal/awsx` (not a new SDK service module), read only first, Query by key as the default read, Scan only with an explicit confirm and hard caps. One explorer UI serves both through a single `ddbviewer` request model so the guard is written once.

### 1. Local engine

| Item | Decision |
| --- | --- |
| Engine id | `dynamodb` (`store.EngineDynamoDB`) |
| Image | `amazon/dynamodb-local`, unmodified, pulled by tag, digest recorded at create like other engines |
| Version | `latest` keyword goes through the existing resolver. `catalog.json` gets a `dynamodb` entry with no `lines` (no published end of life dates) and curated `versions` on majors 2 and 3, so `supportedPrefixes` yields majors 2 and 3, the picker returns `3.3.2` today, the old 1.x line is never picked, and a future major 4 shows up as `SkippedNewer` |
| Command | `-Xmx<heap> -jar DynamoDBLocal.jar -sharedDb -dbPath /home/dynamodblocal/data` for disk, `-sharedDb -inMemory` for memory mode. Heap and container memory limit come from settings so the JVM default (a quarter of host memory when unlimited) never applies |
| `-sharedDb` | Always on. Without it the engine keeps one file per access key and region, so two apps with different dummy keys silently see different tables |
| Persistence | Named volume at `/home/dynamodblocal/data`. The reconciler runs a one shot init container from the same image (`--user 0 --entrypoint chown`, `1000:1000`) before first start and after a restore. No image changes |
| Memory mode | Allowed for preview environments and tests (`spec.storage: memory`). Data is lost on every restart; the dashboard says so on the database page |
| Port | 8000 on the private network. No host port by default. Public access follows ADR 082; a public bind on all interfaces is refused for this engine because the engine has no authentication. Loopback bind is allowed |
| Health | Exec `curl -s -o /dev/null -w %{http_code} http://127.0.0.1:8000/` and treat 400 as healthy, then a signed `ListTables` as ready. A request timeout is mandatory because a broken data directory hangs requests instead of failing them |
| Delay flags | `-delayTransientStatuses` off by default, settable for apps that want to test waiting on `CREATING` |
| Credentials | None exist. The control plane signs with dummy keys. Nothing secret is stored |

Upgrades: the local engine is upgraded by changing the image tag. The data file is SQLite and has no documented forward or backward guarantee, so the upgrade path takes a volume snapshot first (as other engines do), offers image revert only within the same major (`image_revert` true, `auto_max: minor`), and for a major change offers the JSON export and import path.

### 2. Hosted connection (real AWS)

- A hosted DynamoDB connection is `{name, cloud_connection_id, region, table_filter?}`. It holds no keys of its own. Credentials come from the AWS cloud connection (role assumption with external id preferred, then secret by reference, Vault AWS, OIDC federation; static keys only when the operator already allowed them).
- Region is validated against the shape of commercial and GovCloud regions and against `ListRegions` of the connection. The endpoint is derived from the region (`dynamodb.<region>.amazonaws.com`); there is no endpoint override field for hosted connections. This keeps the SSRF surface to one validated region string. China partition endpoints are out of scope for the first version.
- Calls go through `internal/awsx` with a new `ContentType` on `Service` (JSON 1.0, target prefix `DynamoDB_20120810`). Requests use the shared response byte cap and retry policy. `ReturnConsumedCapacity: TOTAL` is set on every read so the explorer shows capacity used per page.
- New capability tiers in `aws_capabilities.go`: `dynamodb-read` (ListTables, DescribeTable, DescribeTimeToLive, DescribeContinuousBackups, ListBackups, Query, Scan, GetItem, PartiQLSelect, cloudwatch GetMetricData) and `dynamodb-write` (PutItem, UpdateItem, DeleteItem, PartiQLInsert, PartiQLUpdate, PartiQLDelete, CreateTable, UpdateTable, CreateBackup, UpdateContinuousBackups, ExportTableToPointInTime). DeleteTable and DeleteBackup stay out of both tiers and are never offered in the first version. The permission check flow (`iam:SimulatePrincipalPolicy`) reports which tier the role really has.
- Cost warning: every Scan reads every item it passes, filtered or not, and is billed for it. The dashboard shows a cost line on Scan and on PartiQL statements that become a scan, using the capacity mode from DescribeTable.

### 3. Explorer

One request model in a new `internal/ddbviewer` package, two transports: exec plus curl for local, `awsx` for hosted. The guard runs before the transport is chosen.

| Feature | Behaviour |
| --- | --- |
| Tables list | ListTables with paging, optional `table_filter` prefix for hosted |
| Describe | Key schema, attribute definitions, GSI and LSI (keys, projection, status), TTL attribute and status, capacity mode and provisioned units, stream status, ItemCount and TableSizeBytes labelled as an estimate (AWS refreshes them about every six hours), PITR status for hosted |
| Browse by Query | Default read. Picks the table or an index, requires the partition key value, optional sort key condition, eventually consistent by default, page size capped, `Limit` and `ExclusiveStartKey` paging |
| Browse by Scan | Needs an explicit confirm per request that names the table and shows the cost line. Page item cap, one page per request, and a per user cap of scanned items per window. Parallel scan segments are not offered |
| Item viewer | JSON viewer with a toggle between DynamoDB JSON (`{"S": "x"}`) and plain JSON. Conversion is lossless one way only: plain JSON cannot express sets or binary, so the editor refuses to save a plain JSON edit of an item that holds sets or binary and asks for DynamoDB JSON |
| Item editor | Locked by default. Write unlock per ADR 082 (`db:write`, step up `manage` scoped to the database, typed confirm of the table name). PutItem with a condition on the original key and version attribute when present, so a stale edit fails instead of overwriting. Delete is one item at a time |
| PartiQL console | ExecuteStatement only, read only by default. The guard classifies the first keyword: SELECT is a read; INSERT, UPDATE, DELETE need write unlock. A SELECT whose WHERE lacks an equality on the partition key of the target table or index is classed as a scan and needs the Scan confirm. BatchExecuteStatement and ExecuteTransaction are not exposed. Statement byte cap reuses `APP_DB_QUERY_MAX_SQL_BYTES` |
| Create table and index | Form builds a CreateTable or UpdateTable (GSI add) request, shows the JSON preview and an estimated cost note for provisioned mode, runs only after `db:ddl` and a typed confirm. Index removal and table deletion are not offered |
| Export | JSON lines in DynamoDB JSON, from Query or Scan with the same confirm, capped by item count and bytes, `db:export` plus the `export` step up |
| Import | JSON lines in DynamoDB JSON or plain JSON, PutItem in batches of 25 with backoff on throttling, capped by item count and bytes, `db:import`, preview of the first items and the count before run, no overwrite unless chosen |

### 4. How apps get connection config

Local engine: on link, the app receives non secret env values.

| Env | Value |
| --- | --- |
| `AWS_ENDPOINT_URL_DYNAMODB` | `http://<container>:8000` |
| `AWS_REGION` | the database's region setting, default seeded from env |
| `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` | dummy letters and digits only (the local engine rejects other characters since 2.0) |
| `DYNAMODB_ENDPOINT` | same as the first row, for code that reads its own variable |

Recent SDKs read the service specific endpoint variable on their own. Older SDKs need the override in code, which the Connect dialog shows per language:

- Spring Boot with AWS SDK v2: a `DynamoDbClient` bean with `endpointOverride(URI.create(System.getenv("DYNAMODB_ENDPOINT")))`, `region(Region.of(...))` and a static credentials provider only in the local profile.
- Spring Cloud AWS: `spring.cloud.aws.dynamodb.endpoint=${DYNAMODB_ENDPOINT}`, `spring.cloud.aws.region.static=${AWS_REGION}`, and the dummy keys under `spring.cloud.aws.credentials` in the local profile only.
- Node (AWS SDK v3): `new DynamoDBClient({ region, endpoint: process.env.DYNAMODB_ENDPOINT })`.
- Python: `boto3.client("dynamodb", endpoint_url=os.environ.get("DYNAMODB_ENDPOINT"))`.
- Go (SDK v2): `dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) { o.BaseEndpoint = aws.String(ep) })` when the endpoint is set.

Hosted connection: the app receives `AWS_REGION` and table names only. Levelrail never hands the control plane's AWS session to an app. App credentials stay the app's concern (node instance role, its own secret reference). The Connect dialog says this in one sentence and links the IAM snippet for the app role.

### 5. Control plane and credentials

- Local: exec only, dummy keys, no credentials in the control plane because none exist. The request body goes to `curl --data-binary @-` on stdin, never on argv, so it does not show in process lists or exec audit of the container runtime.
- Hosted: the control plane must call AWS itself, so it holds short lived session credentials by nature. Controls: role assumption first with external id; sessions of `APP_CLOUD_AWS_SESSION_SECONDS`; credentials live in memory only, never in the store, logs, API responses or the browser (the `Secret` type redacts); the long lived secret behind a secret reference stays in the secrets store encrypted at rest as today; rotation follows the cloud connection (rotating there takes effect on the next session); scope is the capability tier on the role, plus optional `table_filter` enforced in Levelrail; every write, DDL, export and import writes an audit row, and CloudTrail carries `levelrail-<user>` as the session name.
- Why `awsx` and not `service/dynamodb` from the SDK: the repo pattern is the in house client for new AWS services (CloudWatch, ECR, RDS and others), it avoids a large module and a `go.mod` change, and the needed surface is about 15 operations. The cost is hand written request and response types for the attribute value union; a fake server test in `awsxtest` covers them.

### 6. Fit with other decisions

- ADR 082: actions map as `db:read` (Query, Scan, GetItem, PartiQL SELECT, describe), `db:write` (PutItem, UpdateItem, DeleteItem, PartiQL DML), `db:ddl` (CreateTable, GSI add), `db:export`, `db:import`, `db:admin` (on demand backup, PITR toggle, S3 export). `database:<name>/table/<t>` is evaluated by table scoped routes and by PartiQL after the target table is parsed. Honest limits: for hosted, Levelrail policy can only narrow what the AWS role allows, never widen it, and every user acts as the same role; per user identity exists only in CloudTrail session names, not in AWS permissions. Mapping Levelrail documents into AWS IAM (for example leading key conditions) is not done. For local, there is no database identity at all (ADR 082 layer L5 is empty): any client on the network that reaches port 8000 can do anything, so the network is the only boundary.
- ADR 081 branches: not supported. The local engine has no snapshot or clone primitive and its SQLite file format is undocumented; the hosted service has restore to a new table, which is slow, billed and creates resources outside Levelrail's lifecycle. The supported substitute is "copy by export and import" between a source and a new local database, under the same caps. A file level clone of the local volume while stopped is a candidate for later, gated on a round trip test across versions.
- ADR 083 sessions: hosted needs none (the control plane keeps HTTP connections warm). Local exec of curl pays the per exec cost ADR 083 measured; when 083 lands, the local transport uses a persistent session and runs one curl per frame inside it. Until then each explorer request is one exec.
- Monitoring: hosted reads `AWS/DynamoDB` metrics (consumed read and write capacity, throttled requests, successful request latency, user and system errors) through the cached CloudWatch reader, per table. Local has no engine metrics; container CPU and memory only.
- Backups: hosted shows on demand backups (list, create), PITR status and toggle, and export to S3 (requires PITR and an S3 bucket under the existing S3 capability). Restore is not offered in the first version. Local uses volume snapshots like other engines plus the JSON export.
- IaC and spec: `databases[].engine: dynamodb` with `version`, `storage: disk|memory`, `memory_limit`, `heap`, optional `tables[]` seed definitions (keys, GSIs, billing mode) applied idempotently after ready. Hosted connections appear as `hosted_databases[]` with `provider: aws-dynamodb`, `cloud_connection`, `region`, `table_filter`; tables of a hosted connection are never created from spec in the first version.
- CLI and API: `levelrail databases create --engine dynamodb`, `levelrail dynamodb tables|describe|query|scan --confirm-scan|get|put|partiql` working on both kinds. API under `/api/v1/databases/{name}/dynamodb/...` and `/api/v1/hosted-databases/{name}/dynamodb/...` sharing handlers.
- Dashboard: database page tabs Tables, Items, PartiQL, Metrics (hosted only), Backups, Connect. The Items tab opens on Query; Scan is a secondary button with the cost line.

### 7. Settings

All are runtime settings that apply live, seeded from env on first boot.

| Env seed | Default | Meaning |
| --- | --- | --- |
| `APP_DYNAMODB_PAGE_ITEMS` | 50 | items per Query or Scan page |
| `APP_DYNAMODB_PAGE_ITEMS_MAX` | 500 | upper bound a caller may ask for |
| `APP_DYNAMODB_SCAN_ITEMS_PER_HOUR` | 5000 | scanned items per user per database per hour |
| `APP_DYNAMODB_EXPORT_MAX_ITEMS` | 100000 | export cap |
| `APP_DYNAMODB_EXPORT_MAX_BYTES` | 268435456 | export byte cap |
| `APP_DYNAMODB_IMPORT_MAX_ITEMS` | 100000 | import cap |
| `APP_DYNAMODB_IMPORT_MAX_BYTES` | 268435456 | import byte cap |
| `APP_DYNAMODB_REQUEST_TIMEOUT` | 10s | per call, local and hosted |
| `APP_DYNAMODB_LOCAL_HEAP_MB` | 256 | `-Xmx` for the local engine |
| `APP_DYNAMODB_LOCAL_MEMORY_MB` | 512 | container memory limit |
| `APP_DYNAMODB_DEFAULT_REGION` | us-east-1 | region given to apps of a local engine |
| `APP_DYNAMODB_METRICS_CACHE_SECONDS` | 300 | CloudWatch cache for table metrics |

## Alternatives rejected

- Run the control plane against the local engine over the network instead of exec: the control plane cannot reach private networks on remote nodes, and exec is the model every other engine uses.
- Bake a helper (for example an AWS CLI) into a custom image: breaks the unmodified public image rule and adds an image to maintain. The image already ships curl.
- Add `aws-sdk-go-v2/service/dynamodb`: larger dependency and a `go.mod` change for 15 operations; `awsx` is the existing pattern.
- Endpoint override on hosted connections (to allow compatible services): reopens SSRF and blurs the cost and credential story. Compatible endpoints can come later as their own connection type.
- Scan as the default browse: cheapest to build, most expensive to run on real tables.
- Disk mode with `--user 0` on the main container: runs the engine as root forever to fix a one time ownership problem.
- Branches through hosted restore to a new table: billed, slow, and creates AWS resources outside the Levelrail lifecycle.

## Security review checklist (lr-reviewer, every slice marked SEC)

1. Every route calls `dbauthz.Resolve` and is in the route table test.
2. No hosted credential reaches a response, log line, audit row, error text or the browser; errors from AWS are classified (`classifyAWS`) before they leave.
3. Hosted endpoint is built only from a validated region; no field accepts a URL.
4. Local transport passes request bodies on stdin, never argv; the target is always `127.0.0.1:8000` inside the container.
5. Scan and scan class PartiQL need the confirm flag on the request itself, not a stored preference; caps are enforced server side.
6. Write unlock needs `db:write`, a fresh scoped step up and a typed confirm; tokens cannot hold a write unlock unless the token has `root`.
7. Item edits use condition expressions so a stale edit fails.
8. Local engine cannot be published on all interfaces; loopback bind only.
9. Export and import stream with byte caps and never buffer a whole table in memory.
10. Capability tiers contain no delete actions.

## Failure modes

| Failure | Behaviour |
| --- | --- |
| Data directory not writable | Requests hang. Health uses a timeout, the reconciler re runs the chown init once, then marks the database failed with the reason |
| Container killed by memory limit | Restart policy as other engines; memory mode loses data and the page says so |
| JVM heap too small | Engine errors on large batches; heap is a live setting with a restart |
| AWS throttling | `awsx` retries with backoff, then the explorer shows the throttled error and the capacity used |
| Session expired mid export | Refresh once, resume from the last key; second failure stops with a partial file marked incomplete |
| Role lacks a tier | Permission check names the missing tier before the user clicks |
| ItemCount looks wrong | Shown as an estimate with its update caveat |
| Image tag missing upstream | Resolver falls back to the cached or catalog version |

## What cannot be verified locally

- Real AWS behaviour: throttling, capacity billing per page, PITR export to S3, CloudWatch metric names and latency, permission tiers against a real role. Needs the AWS test account with the budget guard.
- arm64 performance and memory of the local engine with real data.
- PartiQL feature differences between the local engine and the service.
- SDK versions that read `AWS_ENDPOINT_URL_DYNAMODB` on their own; the snippets assume the explicit override.

## Testing plan

- Unit: guard classification (Query, Scan, PartiQL keyword and key detection), DynamoDB JSON and plain JSON conversion with sets, binary and numbers as strings, caps, region validation, catalog pick for `dynamodb` (picks 3.x, rejects `latest`, reports a future 4.x).
- Fake server: `awsxtest` with JSON 1.0 content type for every hosted operation, including throttling and expired credentials.
- Live local: one test behind the existing live tag that starts the engine on disk, creates a table, writes, restarts, reads back, then removes everything.
- Real host: one manual run against a small on demand table in the test account, recorded in the release checklist.

## Build slices

| # | Slice | Files (owner) | Depends | Review |
| --- | --- | --- | --- | --- |
| 1 | `awsx` JSON 1.0 support and DynamoDB service definition | `internal/awsx/client.go`, `internal/awsx/dynamodb/` (new), `internal/awsx/awsxtest/server.go` | none | no |
| 2 | Engine constant, image, port, catalog entry, resolver test | `internal/store/database.go`, `internal/reconcile/database/address.go`, `internal/dbupgrade/catalog.json`, `latest_test.go` | none | no |
| 3 | Reconciler: chown init, command flags, memory and heap, health with timeout, memory mode | `internal/reconcile/database/dynamodb.go` (new), `controller.go` | 2 | SEC |
| 4 | Request model, guard, JSON conversion | `internal/ddbviewer/` (new) | none | SEC |
| 5 | Local transport by exec and curl on stdin | `internal/ddbviewer/local.go` | 3, 4 | SEC |
| 6 | Hosted connection: store, validation, capability tiers, permission check | `internal/store/hosted_databases.go`, migration, `internal/cloudconn/aws_capabilities.go`, `internal/api/hosted_databases.go` | 1 | SEC |
| 7 | Hosted transport through `awsx` with session credentials | `internal/ddbviewer/hosted.go` | 1, 4, 6 | SEC |
| 8 | API routes for read paths, resolver wiring, route table test | `internal/api/database_viewer_dynamodb.go`, `routes_*.go` | 5, 7, ADR 082 slice 1 | SEC |
| 9 | Write unlock, item edit, PartiQL DML, create table and GSI | `internal/api/database_viewer_dynamodb_write.go`, `internal/ddbviewer/write.go` | 8 | SEC |
| 10 | Export and import with caps | `internal/ddbviewer/transfer.go`, `internal/api/database_viewer_dynamodb_transfer.go` | 8 | SEC |
| 11 | Dashboard: tabs, Query first browse, Scan confirm, item viewer and editor, PartiQL, Connect dialog, locales | `web/src/routes/databases/$name/dynamodb*.tsx`, `web/src/components/databases/dynamodb/`, locale files | 8 | no |
| 12 | CLI, spec fields, metrics and backups for hosted | `cmd/levelrail-cli/dynamodb*.go`, `internal/apiclient`, `internal/spec/spec.go`, `validate.go`, `internal/api/hosted_dynamodb_ops.go` | 8 | no |

Order: 1, 2 and 4 in parallel; then 3 and 6; then 5 and 7; then 8; then 9, 10, 11 and 12 (11 and 12 do not share files, but 11 touches locale files and must not run alongside any other locale slice).

## Consequences

- One more managed engine with no authentication, so its safety rests on the private network default and the loopback only rule.
- The control plane holds short lived AWS sessions for hosted DynamoDB, as it already does for other AWS features.
- A JVM engine raises the memory floor of a dev stack by about 160 MiB idle.
- The guard is written once for both kinds, which keeps local and hosted behaviour the same for users.

## Open questions

1. Default storage for a new local database: disk (proposed) or memory, and should preview environments default to memory?
2. Is a per user scanned item budget enough, or should Scan on hosted tables also need the `manage` step up?
3. Should hosted connections allow writes at all in the first release, or stay read only until a real host run is recorded?
4. Should the local exec transport wait for ADR 083 sessions, given about 600 ms per request without them?
5. China partition and compatible endpoints: later connection types, or never?
