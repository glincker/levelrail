# ADR 082: Database access, roles, IAM, SSO readiness, network and firewall model

Status: Proposed

## Context

The database explorer is growing write paths (row edits, imports, DDL, document and key editing, DynamoDB items) and more engines. Every one of those must answer the same questions: who is asking, may they touch this database, as which database identity does the statement run, how is it recorded, and who can reach the database over the network at all. Today the answers are spread over handlers. This ADR fixes one model for access and one for the network, and every later slice builds on it.

What exists on `release/v1.5.11`, read in code:

Identity and abilities

- Abilities are flat strings (`internal/authengine/abilities.go`): `read`, `read:sensitive`, `write`, `write:sensitive`, `deploy`, `root`, plus token only `signin:approve` and `secrets:reveal`. Sessions and API tokens both resolve to a principal and a list of abilities (`callerPrincipal`, `internal/api/auth.go`). A pinned session resolves as a token principal with its pinned abilities.
- Stored roles (the `access-roles` experimental feature, `internal/api/roles_store.go`) carry abilities and a visibility (`all` or `granted`) and sync abilities onto users (`SyncRoleAbilities`). Environment grants per user exist (`SetUserEnvironmentGrants`).
- SSO: OIDC sign in reads a groups claim (`internal/authengine/oauth_claims.go`, capped at 200 groups) and maps it to a stored role through `store.SSOGroupMapping{Group, RoleID, Priority}` (`resolveSSORole`, `internal/api/sso_glue.go`, `oauthDesiredRole` in `sso_oauth_policy.go`). SAML and SCIM code exist (`sso_saml.go`, `scim.go`); whether the SAML path uses the same mapping table was not checked for this ADR.
- Session rules per team (`internal/accesspolicy`): idle timeout, max lifetime, IP allow lists (an address must be in every team's list), required MFA, session caps.

Resource policy

- IAM style documents (`internal/api/iam.go`): statements with `Effect` Allow or Deny, `Action` = ability names (validated by `isKnownAbility`), `Resource` = `app:<name>`, `database:<name>` or `*`, trailing `*` as a prefix wildcard. Evaluation: an explicit Deny anywhere wins, then base abilities, then an explicit Allow may grant a resource scoped ability the caller lacks globally (`authorizeResource`).
- `requireAbilityForResource` (`auth.go`) runs the decision through `authorizeResourceInEnvironment` (`visibility.go`): restricted callers see a 404 for resources outside their environments, team scoped callers are checked against every owner with the team role capping abilities (`capAbilities`) and protected environment rules (`protectedDenied`). `requireAbilityDecided` writes one `audit_log` row for every gated route above `read`.
- `internal/policy` is deploy guardrails (registries, memory, CPU, mounts, labels), not access policy. It is not part of this model.

Step up

- Step up grants (`internal/api/step_up.go`) are short lived, keyed by a hash of the session token plus action plus a scope string, so a grant is useless from another session or scope. Actions today: `reveal`, `export`, `manage`. Database password reveal uses scope `database-credentials:<name>` with Root plus fresh step up and a per minute cap (`APP_DB_REVEAL_PER_MINUTE`).

Database identity

- The explorer runs the engine client inside the database container (`internal/dbviewer`): Postgres `psql` over the local socket as the superuser inside `BEGIN READ ONLY`, MySQL and MariaDB as `root` inside `START TRANSACTION READ ONLY`. The control plane never reads database passwords for the explorer. Reads need `read:sensitive`, writes need `root` plus a typed confirm, no step up for queries. Writes get an audit row with a statement fingerprint; reads are logged to slog and to per user query history.
- `internal/dbaccess` manages Postgres roles only (`Postgres.Create`, `Rotate`, `SetLogin`, `Drop`, `DropTemp`), presets `read_only`, `read_write`, `owner` (`sql.go`), temporary credentials with TTL bounds from env (`APP_DB_TEMP_TTL_MIN_MINUTES`, `_MAX_`, `_DEFAULT_`, sweep `APP_DB_TEMP_SWEEP_SECONDS`) and a sweeper that drops `tmp_` roles. `TempConnLimit = 5` is a constant. User routes refuse engines other than Postgres (`database_access.go`).
- Redis family databases are passwordless by design; Memcached has no auth and may only publish on loopback.

Network

- Default private: no host port, reachable on the Docker network or mesh. Network scope `platform`, `project`, `environment` (`internal/dbaccess/scope.go`).
- Public access publishes a host port with bind `127.0.0.1` (default) or all interfaces (`database_public_access.go`, gated `write:sensitive`).
- Allowed source rules (preview, apply, remove, make private) are enforced in the `DOCKER-USER` iptables chain by `internal/exposure`. That chain is not visible on non Linux hosts and does not exist on nftables only engines; the code reports this as a reason, it does not enforce. Rules have no expiry.
- `internal/reconcile/firewall` converges a `firewall_rules` table onto ufw on the local node only; remote node rules are skipped and logged. AWS security group calls exist in `internal/provision/aws_network.go` for provisioned nodes.
- TLS: require TLS for Postgres (rewrites catch all rules to `hostssl`, local socket untouched, `internal/dbaccess/tls.go`) and Redis. Other engines have none.
- Tunnel: websocket relay, Root, control plane node only.
- External databases: the address policy in `internal/extdb/conn.go` refuses loopback, unspecified, multicast and link local or cloud metadata addresses (opt in `APP_EXTERNAL_DB_ALLOW_LINK_LOCAL`); private ranges (RFC1918, ULA) are allowed. External database routes use the same `database:<name>` resource string as managed databases.

Note on references: the task brief points at "ADR 043 cloud firewall". In this repo `adr/043` is concurrent reconcile; there is no cloud firewall ADR on this branch. This ADR therefore states the cloud firewall part itself and keeps it small. ADR 081 (database branches) lives on another branch; this ADR relies only on its summary: a branch is a normal managed database plus one `database_branches` row with a parent, created by dump and restore into an unreachable container.

## Decision

Recommendation in one paragraph: every database route goes through one resolver, `dbauthz.Resolve`, that returns abilities, the policy decision, the database preset to run as and the limits. The explorer stops running as the engine superuser: reads run as a per database read only login, writes and DDL run as a short lived per user brokered login created on demand and dropped on expiry or logout, and the superuser stays for platform operations only. Presets are data per engine. SSO lands on stored roles, and stored roles map to a maximum database preset per database, so the explorer never looks at the login source. The network model has four exposure levels, allowed sources with expiry, TLS per engine with verify modes, and an explicit private range setting for external databases with a deny default on new installs.

### 1. Layered access and one resolver

Layers, evaluated in this order. A failure at any layer stops the request.

| Layer | Question | Source of truth |
| --- | --- | --- |
| L1 identity | Who is calling, session or token, which principal | `callerPrincipal`, session rules (`internal/accesspolicy`) |
| L2 ability | Does the caller hold the base ability for the route | abilities on the user, role or token |
| L3 resource policy | Allow or Deny on `database:<name>` for the database action, with team, project and environment scope | `iam.go` documents, `authorizeResourceInEnvironment`, team caps, protected environments |
| L4 step up and unlock | Is a fresh, scoped step up present for writes, exports, break glass | step up grants with scope `database:<name>` |
| L5 database identity | Which preset and which login does the statement run as | preset registry plus broker |

Database actions. Abilities stay the route gate (L2, unchanged middleware). Inside the resolver a second, finer action is evaluated against the same policy documents. New action names are added to the known set in `iam.go`:

| Action | Meaning | Default when no policy mentions it |
| --- | --- | --- |
| `db:read` | schema, rows, read queries, key and document reads | holds `read:sensitive` |
| `db:write` | insert, update, delete, key set, document replace | holds `root` (today's rule) |
| `db:ddl` | create, alter, drop, index changes | holds `root` |
| `db:export` | file export, dumps, MCP read tools returning rows | holds `read:sensitive` and a fresh `export` step up |
| `db:import` | file import into a table or collection | holds `root` |
| `db:admin` | users, presets, sessions and locks kill, engine settings | holds `root` |
| `db:breakglass` | owner preset with step up and expiry | holds `root`, never a token |

An explicit Deny on a db action wins over everything, as today. An explicit Allow can grant `db:write` to a non root user on one database, which is how "this team writes to staging but only reads production" is expressed without new roles.

The resolver:

```go
// internal/dbauthz
func Resolve(ctx context.Context, in Request) (Decision, error)

type Request struct {
    Caller   Caller   // principal type and id, abilities, session id hash, source IP
    Database string   // managed or external name
    Branch   string   // set when the database is a branch (ADR 081)
    Action   Action   // db:read, db:write, ...
    Engine   string
}

type Decision struct {
    Allowed    bool
    Hidden     bool          // answer 404, not 403
    Reason     string        // stable code, also the API error code
    Preset     string        // read_only, read_write, schema_admin, owner
    Login      LoginMode     // shared_readonly, brokered, breakglass
    StepUpNeed string        // action name when a fresh step up is missing
    Limits     Limits        // rows, bytes, timeout, concurrency, rate
    AuditMode  AuditMode     // fingerprint only, or full text when the setting allows
}
```

Every explorer route (SQL, document, key value, DynamoDB, ClickHouse, search engines), every export and import route, branch routes and the MCP database tools call `Resolve` and nothing else for authz. Handlers no longer compare abilities themselves. A table driven test lists every database route pattern and fails when a route is registered without a resolver call (the same idea as the param route allowlist).

Resource names:

- `database:<name>` for managed and external databases. External databases share this namespace today; the resolver adds `database-kind:external` as an extra resource so a policy can target either kind without a rename.
- Branches resolve as `database:<branch>` and also pass `database:<parent>/branches` as an extra resource. Decision: branches inherit both Allow and Deny written on `database:<parent>/branches`; they do not inherit statements on `database:<parent>` itself, so a production Allow never leaks to every branch by accident. ADR 081 rule kept: a full data branch of a parent needs `db:admin` on the parent; `db:write` may only create schema only or masked branches.
- `database:<name>/collection/<c>` and `database:<name>/table/<schema>.<t>` are accepted by the parser and the policy simulator, evaluated only by routes that act on one collection or table. They are not evaluated for free form queries (a SQL statement can touch any table, the database grants decide there).
- Prefix patterns: a policy author writing `database:prod*` also matches `database:production2`. Docs and the simulator warn and suggest `database:prod` plus an explicit list or a separator (`database:prod-*`). The match function itself is not changed in this ADR.

### 2. Database identity per engine

Three login modes.

| Mode | Used for | Identity |
| --- | --- | --- |
| shared read only | reads by anyone allowed `db:read` | one platform role per database, `lr_ro`, created at provision or first use |
| brokered | writes, DDL, imports, and reads when the per user setting is on | one login per Levelrail user per database, `lr_u_<short user hash>`, preset from the decision |
| break glass | `db:breakglass` | brokered login with the `owner` preset, short expiry |

The engine superuser stays for platform operations only: provisioning, backups, restores, upgrades, user management by `internal/dbaccess`. The explorer never runs as the superuser after M02. A stopgap like `SET ROLE` inside a superuser session is rejected (see Alternatives).

Brokered login lifecycle: created on the first statement that needs it, recorded in `database_access_users` with `kind=brokered`, `levelrail_user_id`, `preset`, `expires_at`. Expiry is the unlock window (owner decision 5, default 15 minutes, live setting `databases.explorer.unlock_minutes`, seeded from `APP_DB_EXPLORER_UNLOCK_MINUTES`). Dropped by the existing sweeper on expiry, on session revoke (a hook from `sessionStore.revokeAll` and logout), on role change, on SCIM deprovision and on database delete. A drop that fails is retried by the sweeper and shown in the access panel; it never blocks logout.

Engine by engine, with honest limits:

| Engine | Read only login | Brokered login | Attribution in the engine | Limits |
| --- | --- | --- | --- | --- |
| Postgres | role with `pg_read_all_data` (14 and later) or per schema `SELECT` grants plus default privileges; `default_transaction_read_only=on` on the role | `CREATE ROLE ... LOGIN CONNECTION LIMIT n` with no password; exec `psql -U <role>` over the local socket | role name in `pg_stat_activity.usename`, `application_name=levelrail:<user id>:<request id>` | Relies on the image's local socket trust; a role with no password cannot log in over TCP. `VALID UNTIL` does not apply to socket trust, so the sweeper is the expiry. Postgres 13 has no `pg_read_all_data`; grants per schema then. |
| MySQL 8 | `lr_ro@localhost` with `SELECT, SHOW VIEW` | `lr_u_x@localhost` with a random password held only in control plane memory for the unlock window, passed via exec env, never stored or logged | user in processlist and `performance_schema` | Needs a password (no socket auth in the official image). A control plane restart loses the password, the sweeper drops the user and the next statement recreates it. Host `localhost` keeps TCP logins out. |
| MariaDB | same as MySQL | same as MySQL; `unix_socket` exists for root only in the image, not reused | user in processlist | Same restart rule. |
| MongoDB | user `lr_ro` with `read` on the target database | user `lr_u_x` with `read`, `readWrite` or `dbAdmin` per preset, password in memory as above; exec `mongosh` with `appName=levelrail:<user id>` | `currentOp` shows user and appName; profiler entries carry `user` | Community edition has no audit log, so the Levelrail audit is the only statement record. Users have no expiry, the sweeper is the expiry. |
| DocumentDB (open source, Postgres based) | VERIFY | VERIFY | VERIFY | User model not read yet; explorer stays on the shared read only login until verified (lane B14 and B15). |
| AWS DocumentDB | stored credential | not brokered in 1.5.11 | none beyond the stored user | Hosted, Levelrail cannot create users without an admin credential the owner chooses to store. |
| Redis, Valkey, KeyDB | ACL user `lr_ro` with `+@read -@dangerous ~*` | ACL user `lr_u_x` with preset command and key patterns, password in memory | `CLIENT SETNAME levelrail:<user id>`, ACL user in `CLIENT LIST` | The default user stays passwordless for apps, so ACL users are attribution and command limits inside the explorer, not a network boundary. Network isolation stays the boundary. KeyDB ACL support VERIFY. |
| Dragonfly | VERIFY ACL support | VERIFY | `CLIENT SETNAME` | Explorer stays on fixed read commands built by code until verified. |
| Memcached | none | none | none | No auth exists; stats only, loopback publish only. |
| DynamoDB Local | not applicable | not applicable | Levelrail audit only | DynamoDB Local accepts any credentials. Levelrail evaluates an IAM like document (actions `dynamodb:GetItem`, `Query`, `Scan`, `PutItem`, `DeleteItem`, `UpdateItem`, `PartiQL*`; resources `table/<t>`, `table/<t>/index/<i>`) in the explorer proxy. This is enforcement in Levelrail only; apps on the network are not restricted. |
| AWS DynamoDB | connection role | `sts:AssumeRole` with a session policy narrowed to the preset and `SourceIdentity=levelrail:<user id>`, duration = unlock window | CloudTrail records the source identity | Needs a role the owner creates with a trust policy and `sts:SetSourceIdentity`; without it the explorer is read only on the stored credential. |
| ClickHouse | SQL user with `readonly=1` | SQL user, password in memory | `system.query_log.user` | After lane G05. |
| External SQL (Postgres, MySQL, MariaDB) | stored credential | optional broker mode only when the stored credential can create roles; off by default | stored user only | Without broker mode every statement runs as the stored user and attribution lives in the Levelrail audit only. Shown in the UI. |

### 3. Role presets as data

`internal/dbaccess/presets.yaml` holds presets per engine: `read_only`, `read_write`, `schema_admin`, `owner`, each with the engine statements (grant templates with validated identifiers, never string concatenation of user input), the capability it unlocks in the explorer and whether it can be brokered. The existing Postgres presets move into this file without behavior change. The Access UI renders presets from `GET /databases/{name}/access/presets`, not from engine lists.

Mapping Levelrail to presets:

1. The decision's ceiling comes from the db action: `db:read` gives `read_only`, `db:write` gives `read_write`, `db:ddl` gives `schema_admin`, `db:breakglass` gives `owner`.
2. A per database binding table `database_role_bindings(database, levelrail_role_id, max_preset)` can lower the ceiling for members of a stored role. It never raises it above what the db action allows.
3. The effective preset is the lower of the two. Absent a binding, the ceiling applies.

Per column or per collection masking by role is not part of this ADR's build: masking ships for branches (ADR 081, dump stream). Explorer masking by role needs either engine views per role or a result rewriter; both are open questions below.

### 4. SSO readiness, service accounts, break glass

- The group to role table already exists (`SSOGroupMapping`). This ADR adds nothing to it; the database side reads stored roles. Chain: IdP group, stored role (priority wins), abilities plus `database_role_bindings`. The mapping can be stored and tested without a provider configured: a "simulate groups" box in settings runs `resolveSSORole` and then the resolver for a chosen database.
- The explorer and the resolver read abilities and role id from the session only. They never branch on password, passkey, OIDC or SAML. A test signs in through each path and asserts the same decision.
- Role changes at the IdP take effect at the next sign in (OIDC) or SCIM update; on change the broker drops that user's brokered logins.
- Service accounts and CI tokens: tokens resolve like users. Tokens cannot hold a write unlock through interactive step up; `db:write`, `db:ddl` and `db:import` for a token need an explicit Allow policy on that database and a token expiry no longer than the setting `databases.tokens.max_write_days` (seed `APP_DB_TOKEN_MAX_WRITE_DAYS`, default 7). Without both, a token is capped at `read_only`. Tokens never get `db:breakglass`. Pinned sessions keep their pinned abilities.
- Break glass: `POST /databases/{name}/explorer/break-glass` needs `root`, a fresh step up (passkey preferred, password accepted), a reason string, a duration up to `databases.breakglass.max_minutes` (seed `APP_DB_BREAKGLASS_MAX_MINUTES`, default 30). It creates a brokered `owner` login, notifies every root user, flags every audit row in the window, and cannot be extended, only reissued with a new step up.

### 5. Audit model

A dedicated table, not `audit_log`, because reads are audited too and the volume differs.

`database_statement_audit`: `ts`, `database`, `parent_database` (branches), `engine`, `principal_type`, `principal_id`, `levelrail_role_id`, `db_login`, `preset`, `login_mode`, `action`, `fingerprint` (normalized statement hash), `statement_text` (only when the setting allows, default off, capped), `rows`, `bytes`, `duration_ms`, `outcome`, `error_class`, `source_ip`, `user_agent_hash`, `session_hash`, `step_up_grant_hash`, `request_id`.

- Every statement through the explorer, every export and import, every break glass event writes one row. Exports and imports also write an `audit_log` row so the instance audit viewer shows them.
- Retention: live setting `databases.audit.retention_days`, seed `APP_DB_AUDIT_RETENTION_DAYS`, default 90; batched sweep like metrics retention. A size guard `APP_DB_AUDIT_MAX_ROWS` keeps the newest rows when the cap is hit and raises a doctor warning.
- Writes are batched off the request path with a bounded queue; a full queue fails the statement closed for writes and DDL and drops with a counter for reads (reads must not stop because audit is slow, but the drop is visible).
- Query history stays per user and keeps raw SQL as today; it is a convenience, not the audit.
- Engine side: `application_name`, `appName` and `CLIENT SETNAME` carry the user and request id so an engine log line can be joined back to the audit row.

### 6. Network model

Exposure levels, computed and shown in one place (N01):

| Level | Meaning | Requirements |
| --- | --- | --- |
| private | no host port; reachable from attached apps per network scope | default |
| project | private plus scope `project` or `environment` limits which apps may attach | placed in a project |
| loopback | host port bound to `127.0.0.1` (laptop case) | none extra |
| public | host port on all interfaces | allowed sources non empty AND TLS required, unless the owner flips `databases.public.allow_open` (default off) with step up |

Allowed sources:

- Per database and per branch, entries are CIDR, "this machine" (the caller's source IP as the control plane sees it, behind the trusted proxy setting), another app, another project. App and project entries resolve to their container networks, not IPs.
- Each entry has an optional `expires_at`; a sweeper removes expired entries and re-applies the chain. Default expiry for "this machine" entries: `databases.network.temp_source_hours`, seed `APP_DB_TEMP_SOURCE_HOURS`, default 8.
- Enforcement stays in `internal/exposure` (`DOCKER-USER`). Where enforcement is not possible (non Linux host, nftables only engine) the level `public` is refused, not shown as protected.
- Brokered logins may carry a per user allow list later (Postgres `pg_hba` per role); not built now because brokered logins are socket only.

TLS for every engine:

| Engine | Server TLS | Status |
| --- | --- | --- |
| Postgres, Redis | yes | exists |
| MySQL, MariaDB | `require_secure_transport`, mounted cert | N02, VERIFY image paths |
| MongoDB | `--tlsMode requireTLS` | N02 |
| Valkey, KeyDB, Dragonfly | TLS port flags | N02, VERIFY image builds include TLS |
| ClickHouse | TLS port config | after G05 |
| DynamoDB Local | none | loopback only, never public |

Certificates come from one per instance CA (existing self signed path extended), downloadable as a CA bundle. Client verify modes shown in snippets and accepted for external databases: `disable`, `require`, `verify-ca`, `verify-full`; external databases default to `verify-full` when the provider has a public CA, `require` otherwise, and a doctor check flags `disable` on anything not on loopback.

Endpoint or SNI routing (one public port, routed by server name) is later. Postgres 17 direct TLS makes it possible for Postgres; MySQL's protocol starts in plain text, so it would need a protocol aware proxy. Not in 1.5.11.

Host and cloud firewall:

- The host firewall controller is local node only today. N04 adds drift display: for each published database, the expected rule versus what ufw and `DOCKER-USER` hold, per node where readable. Remote node apply waits for the agent RPC that the controller already names.
- Cloud firewall (security groups for AWS nodes, security lists for Oracle nodes): read only drift first ("port 5432 is open to 0.0.0.0/0 in the cloud firewall but the database is loopback"), apply later behind a setting with step up. No automatic cloud changes in 1.5.11.

Outbound rules for external and hosted databases:

- Keep the current address policy (no loopback, no metadata, link local opt in).
- Private ranges become a setting `databases.external.private_ranges` with values `deny`, `allow_listed`, `allow`. Recommendation: `deny` on new installs; on upgrade, existing installs get `allow_listed` with the private CIDRs of external databases they already have, so nothing breaks and nothing new opens. The allow list is a live setting with step up to change.
- Container names on the platform network count as private.
- The address checked by the control plane is pinned: the helper container receives the resolved IP plus the hostname for TLS verification, so a second DNS answer cannot redirect it.
- Credentials are only ever sent to a saved external database origin; a test probe with typed credentials is not stored and not retried elsewhere.

Connection and rate limits (all live settings with env seeds):

| Setting | Seed env | Default |
| --- | --- | --- |
| brokered login connection limit | `APP_DB_BROKERED_CONN_LIMIT` (replaces the `TempConnLimit` constant) | 5 |
| explorer statements per user per database per minute | `APP_DB_EXPLORER_PER_MINUTE` | 120 |
| concurrent explorer statements per user | `APP_DB_EXPLORER_CONCURRENCY` | 2 |
| engine max connections | engine config, per database | engine default |
| exports per user per hour | `APP_DB_EXPORT_PER_HOUR` | 20 |

Doctor checks (N07): public without TLS; public without allowed sources; allowed sources not enforceable on this host; external database with TLS `disable` off loopback; default or never rotated credentials; certificate expiry within `APP_DB_TLS_EXPIRY_WARN_DAYS` (default 21); brokered logins past expiry still present; cloud firewall wider than the exposure level.

### 7. Branches and hosted databases inherit the model

- Branches (ADR 081) are databases, so every route and the resolver apply unchanged. Access inheritance is only through `database:<parent>/branches` statements (section 1). Network: a branch starts private whatever the parent's exposure; allowed sources and published ports are not copied; TLS required state is copied. Roles: brokered and `tmp_` logins are excluded from the dump, and presets are re-created from the registry on the branch. Protected branches refuse break glass `owner` DDL that drops objects without a second typed confirm.
- External and hosted databases use the same resolver with `database-kind:external`; brokered logins only in broker mode; audit rows identical; outbound policy applies to every connection the helper makes.

### 8. Threat model

| Threat | Control in this model |
| --- | --- |
| Viewer or read only user writes data | `db:write` gate, read only login, read only transaction, statement guard |
| Statement escapes its role to the superuser | explorer never runs as the superuser; brokered and read only logins are real logins, not `SET ROLE` |
| Team member reaches another team's database | team caps and owner checks in the resolver, 404 for hidden resources |
| Policy wildcard grants more than intended | simulator warning, explicit Deny wins, deny tests per route |
| Stolen CI token writes to production | token write needs explicit Allow plus bounded expiry, never break glass |
| Stale access after offboarding | brokered logins dropped on logout, role change, SCIM deprovision, expiry |
| Bulk data exfiltration through export or MCP | `db:export` with step up, row and byte caps, rate limits, audit row per export |
| Audit gaps or tampering | dedicated table, write path fails closed for writes, retention and size guard visible in doctor |
| Database exposed to the internet | public level requires allowed sources and TLS; refused where not enforceable; doctor and cloud drift checks |
| SSRF through external database host | address policy, private range setting, pinned resolved address |
| Credentials leaking to the browser or logs | brokered passwords in memory only, exec env not logged, no password in URLs shown |
| Runaway query or lock | timeouts and caps from env, concurrency and rate limits, cancel route (A04) |
| Break glass abuse | root plus step up, reason, short expiry, notification, flagged audit |

Security review checklist for every explorer slice (lr-reviewer):

1. The route calls `dbauthz.Resolve` and nothing else for authz; the route is in the route table test.
2. Deny tests exist for a team scoped caller, a viewer, a token without Allow, an expired step up, and a hidden database (404).
3. The statement runs as the preset login from the decision, never as the superuser.
4. Identifiers are validated and quoted by code; values are literals or parameters; user text reaches the engine only through the guard.
5. Results are capped by the env limits; exports stream with caps.
6. One audit row per statement with the fields above; failures audited too.
7. No password, token or full connection string in responses, logs, errors or audit text.
8. Write paths need the unlock and typed confirm where the plan says so.
9. Network facing changes (publish, allowed sources, TLS) go through the exposure model and write an `audit_log` row.
10. CSRF and origin checks hold for every mutating route (existing middleware, verified by a test for new routes).

## Alternatives rejected

- Keep the shared superuser and rely on the read only transaction and the guard. Rejected: no attribution in the engine, any guard gap is a superuser gap, and writes cannot be narrowed per user.
- `SET ROLE` to a narrower role inside the superuser session. Rejected: in a read only transaction `RESET ROLE` is still allowed, so the boundary depends on the guard catching it. A real login closes that.
- Long lived per user database accounts with stored passwords. Rejected: credential sprawl, stored secrets per user, offboarding debt.
- A new action vocabulary replacing abilities. Rejected for now: too large a change; db actions sit beside abilities inside the resolver only.
- A database proxy in the control plane (wire protocol aware) for all engines. Rejected for 1.5.11: large, per engine protocol work, and it would put credentials in the control plane. The exec model keeps them in the container.
- Mapping IdP groups straight to database presets. Rejected: two mapping tables to keep in sync; groups map to stored roles, roles map to presets.

## Failure modes

- Broker cannot create a login (engine down, out of connections): the statement fails with a clear reason; reads keep working through the read only login if it exists.
- Control plane restart during an unlock: MySQL, MariaDB, MongoDB and Redis passwords are lost; the sweeper drops the logins and the user unlocks again with step up. Postgres socket logins survive and are dropped at expiry.
- Sweeper lag: an expired brokered login lives until the next sweep (default 30 s); doctor reports logins older than expiry plus two sweeps.
- Audit queue full: writes fail closed, reads drop rows with a counter and a doctor warning.
- Exposure cannot be enforced (macOS VM runtime, nftables): public refused, loopback allowed, panel says why.
- Private range migration misses an external database whose host resolves later: probe fails with the address policy reason and a one click "add to allow list" (root, step up).
- IdP group claim missing or renamed: users fall back to the policy default role; the simulator shows group like claims as today.

## What cannot be verified locally

- MySQL, MariaDB, MongoDB, Valkey, KeyDB, Dragonfly TLS flags and cert paths in the official images, and arm64 builds (real host proofs, lane H04).
- ACL support in KeyDB and Dragonfly.
- The open source DocumentDB user model.
- AWS DynamoDB source identity in CloudTrail and the trust policy shape (needs an AWS account and spend approval).
- Cloud firewall drift on real Oracle and AWS nodes.
- `DOCKER-USER` enforcement on a multi node install and on Podman with nftables.
- Behavior of SAML sign in with group mappings (not read for this ADR).
- Audit write volume on a busy database (needs a load run on a real host).

## Testing plan

- Unit: resolver table tests (ability x policy Allow and Deny x team role x environment x token x step up age x branch inheritance), preset registry validation, private range setting, pinned address.
- Route table test: every `/api/v1/databases/{name}/...` and `/api/v1/external-databases/{name}/...` route either calls the resolver or is on a short reviewed exception list.
- Live engine tests behind the existing live test tag: brokered login create, run, drop for Postgres, MySQL, MariaDB, MongoDB, Redis; `pg_stat_activity` shows the user's role and application name.
- Session tests: logout, role change and SCIM deprovision drop brokered logins.
- Audit tests: one row per statement, failed statements included, retention sweep.
- Network: exposure level computation per state; public refused without allowed sources or TLS; expired allowed source removed.
- Real host (vps2, oci1 arm64): TLS per engine, `DOCKER-USER` enforcement, cloud drift read.

## Build slices (lanes M and N)

Order follows dependencies. SEC means Sonnet plus an lr-reviewer pass.

| # | Slice | Lane | Files (owner) | Settings | Review |
| --- | --- | --- | --- | --- | --- |
| 1 | Resolver and db actions: `dbauthz.Resolve`, db action names in the known set, route table test, deny tests; wire the existing explorer routes | M01, M04 | `internal/dbauthz/*` (new), `internal/api/iam.go`, `internal/api/database_viewer*.go` | none new | SEC |
| 2 | Resource names: branches, kind extra resource, collection and table forms in the parser, simulator warnings for prefix patterns | M04 | `internal/api/iam.go`, `internal/api/iam_simulator*.go`, docs IAM page | none | SEC |
| 3 | Preset registry as data; Postgres presets moved without change; `GET /access/presets`; Access UI from presets | M03, G01 | `internal/dbaccess/presets.yaml`, `internal/dbaccess/presets.go`, `internal/api/database_access.go`, web access tab | none | SEC |
| 4 | Statement audit table, batched writer, retention, audit viewer filter by database | M05, A02 | `migrations/*_database_statement_audit.sql`, `internal/store/database_statement_audit.go`, `internal/api/database_viewer_query.go` | `APP_DB_AUDIT_RETENTION_DAYS`, `APP_DB_AUDIT_MAX_ROWS` | SEC |
| 5 | Postgres read only login and brokered logins, unlock window, sweeper and session revoke hooks | M02, A03 | `internal/dbaccess/broker*.go`, `internal/dbaccess/sweeper.go`, `internal/dbviewer/run.go`, `internal/api/auth.go` (revoke hook only) | `APP_DB_EXPLORER_UNLOCK_MINUTES`, `APP_DB_BROKERED_CONN_LIMIT` | SEC |
| 6 | Brokered logins for MySQL, MariaDB, MongoDB (after B02), Redis family ACL (after C07) | M02, B09, C07 | `internal/dbaccess/broker_mysql.go`, `broker_mongo.go`, `broker_redis.go`, `internal/dbviewer/*` | same as 5 | SEC |
| 7 | SSO readiness: `database_role_bindings`, simulate groups, effective role tests across login sources, token write rules | M06 | `migrations/*_database_role_bindings.sql`, `internal/store/database_role_bindings.go`, `internal/api/database_role_bindings.go`, web settings | `APP_DB_TOKEN_MAX_WRITE_DAYS` | SEC |
| 8 | Break glass route, notification, flagged audit | M00 follow up | `internal/api/database_breakglass.go`, `internal/api/step_up.go` (new action) | `APP_DB_BREAKGLASS_MAX_MINUTES` | SEC |
| 9 | Exposure level computation and the Networking panel with self test | N01 | `internal/api/database_exposure.go`, `internal/exposure/*` (read only use), web network tab | none | no |
| 10 | Allowed sources with expiry, "this machine", refuse public where not enforceable, firewall and cloud drift read | N03, N04 | `migrations/*_database_source_expiry.sql`, `internal/api/database_network_rules.go`, `internal/reconcile/firewall/*`, `internal/provision/aws_network.go` (read only) | `APP_DB_TEMP_SOURCE_HOURS` | SEC |
| 11 | TLS for MySQL, MariaDB, MongoDB, Valkey family; CA bundle download; verify modes | N02 | `internal/reconcile/database/tls.go`, `internal/dbaccess/tls.go`, `internal/api/database_tls*.go`, web snippets | `APP_DB_TLS_EXPIRY_WARN_DAYS` | SEC |
| 12 | Outbound private range setting with upgrade migration, pinned address, limits as settings, doctor checks, docs matrix | N05, N06, N07, M07, N08 | `internal/extdb/conn.go`, `internal/extdb/helper.go`, `internal/api/settings_*.go`, `internal/doctor/*`, docs | `APP_DB_EXPLORER_PER_MINUTE`, `APP_DB_EXPLORER_CONCURRENCY`, `APP_DB_EXPORT_PER_HOUR` | SEC |

Slices 1 and 4 come first; 5 depends on 1, 3 and 4; 6 depends on 5 and on lanes B and C; 9 can run in parallel with 1. Slices touching `internal/api/iam.go` (1, 2) and `internal/dbaccess/sweeper.go` (5, 6) are sequenced, not parallel.

## Consequences

- One place decides database access; reviewing a new explorer route means checking one call and its tests.
- Statements become attributable to a Levelrail user in the engine for Postgres, MySQL, MariaDB, MongoDB and the Redis family, and in the audit for all engines.
- More moving parts: brokered logins, a sweeper with more kinds, an audit table that grows. Settings and doctor checks keep them visible.
- New installs refuse private range external databases until the owner allows them; that is a visible behavior change and needs a release note.
- Writes stay `root` by default; finer grants come from policies, so small installs see no change.

## Open questions

1. Should `db:write` default to `write:sensitive` instead of `root` once brokered logins and the unlock ship? Default here: keep `root`.
2. Private range default for new installs: `deny` (proposed) or `allow_listed` with an empty list (same effect, different wording in the UI)?
3. Explorer masking by role: engine views per role, a result rewriter, or branches only for now (default: branches only)?
4. Should statement text be stored in the audit by default, given it can contain data values (default: off, fingerprint only)?
5. Per user brokered logins for reads too (more attribution, more logins), or shared read only login for reads (default: shared, setting to switch)?
6. Cloud firewall apply: who may change a security group from Levelrail, and is it ever automatic (default: never automatic, read only drift in 1.5.11)?
7. Does the SAML path use the same group mapping table, and should SCIM group changes drop brokered logins immediately (default: yes once confirmed)?
