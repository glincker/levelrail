# ADR 087: Database credentials from vaults and secret managers

Status: Proposed

## Context

Some teams do not want Levelrail to hold database credentials. Their rule is that every credential lives in a vault (HashiCorp Vault or a compatible server such as OpenBao, AWS Secrets Manager, Google Secret Manager, Azure Key Vault, 1Password, Doppler, Infisical) and apps read it from there with their own identity. Today Levelrail can read app env values from a vault, but every database credential it creates or adopts is stored in its own encrypted store. This ADR decides how databases fit the vault model for managed local databases, hosted databases and dynamic credentials.

What exists in the code today (read on `work/stage`):

- `internal/secretsource`: the single reference model `secret://<conn>/<path>#<key>`, a `Provider` interface (`Name`, `Capabilities`, `Resolve`), an optional `Renewer`, a `Value` type whose format verbs print a mask, a `Redactor`, a `FakeProvider`, and reference limits seeded from `APP_SECRETREF_MAX_*` and overridden by runtime settings. Read only: there is no write, version list, rotate or revoke.
- `internal/secretconn`: named secret connections, adapters from the Vault resolver and from the Doppler, Infisical and 1Password providers to `secretsource.Provider`, `Probe` and `ProbePath`, and a per connection `Policy` (allowed path prefixes, allowed environments, delegation only when both are set). `CheckResolve` and `CheckUnvettedRef` are the gate for every reference resolve.
- `internal/vault`: HashiCorp Vault KV v2 reads, token and AppRole auth (`store.VaultAuthMethodToken`, `store.VaultAuthMethodAppRole`), session caching and renewal, a short value cache (`KeyVaultCacheTTL`), a dial time address guard (`guard.go`: link local, metadata hosts, unspecified and multicast always blocked; loopback and private blocked unless `KeyVaultAllowPrivate`; no cross host redirects), a credential grace window for rotating the connection credential, a capabilities probe on `data/` and `metadata/` paths, and an AWS secrets engine broker with lease revoke (`awscreds.go`) plus Vault AWS IAM login (`awslogin.go`). The database secrets engine is not used anywhere.
- `internal/secrets`: envelope encryption with per app DEKs under a master key, DEK rotation, and an optional master key wrap through a Vault Transit key (`transit.go`, selected by `APP_KMS_PROVIDER=vault-transit` in `cmd/levelrail/kms.go`). No cloud KMS wrapper exists yet.
- `internal/awsx/secrets`: a signed client for Secrets Manager (`ListSecrets`, `GetSecretValue`, `PutSecretValue`) and SSM parameters. ADR 085 plans `secretsmanager:<arn>` references for adopted RDS passwords; not built.
- Databases: the reconciler (`internal/reconcile/database/controller.go`) passes the engine root credential as container env (`POSTGRES_PASSWORD`, `MYSQL_ROOT_PASSWORD` and the engine equivalents), read from `internal/secrets` under `database.PasswordSecretKey`. `internal/dbrotate` rotates that owner password with set, verify, rollback and a pending key, then restarts dependent apps. `internal/dbaccess` creates roles from presets, rotates them, and issues temporary brokered logins with a sweeper (ADR 082). `internal/api/database_credentials_reveal.go` reveals credentials to root with a fresh step up, rate limited and audited. Hosted records (`internal/extdb`, `store.ExternalDatabase`) keep their password in `internal/secrets` under `external-database/<name>` and pass every host through `Conn.Validate` and `CheckResolved`.
- Apps get database env through attachments (`DATABASE_URL` by default, `internal/api/apps_database.go`) and connection bindings (`<NAME>_DATABASE_URL`, `internal/api/apps_connections.go`). The value is resolved by Levelrail and written into the container env.

Older branches `work/int-vault-client`, `work/secret-connections`, `work/secret-refs`, `work/secretsource`, `work/vault-private-default` and `work/vault-templates` are behind `work/stage` for these packages (their diff only deletes code that later landed). Nothing needs to be salvaged from them. The user doc `docs/vault-secret-references.md` still says Levelrail does no caching; the resolver now has a short cache, so the doc needs a one line fix in the docs slice.

## Decision

Recommendation: add a credentials source to every database record, with three values, `levelrail` (today, the default), `vault_kv` (static credentials pushed to or read from a KV style store) and `vault_dynamic` (short lived credentials issued by a vault database engine). Extend the existing `secretsource` provider model with an optional write and lease side instead of building a second client. Apps receive a reference by default when the source is a vault, not a value. Ship static KV for HashiCorp Vault and AWS Secrets Manager first, dynamic credentials for HashiCorp Vault second, and leave the root credential sealing options as opt in.

### 1. What a vault integration means per database kind

#### a. Managed local databases

The container needs the engine root credential at first start, so Levelrail still generates it. The change is who else ever sees it.

- App credentials: every attached app gets its own database user (the ADR 082 presets, `dbaccess.Create`) instead of the owner login. With source `vault_kv`, Levelrail writes that user's password and a full connection URL to `levelrail/<project>/<database>/<user>` in the connection's mount and stores only a reference row: connection, path, version, fingerprint, last rotated. The local encrypted copy is deleted after a verified write. Rotation (`dbaccess.Rotate`, `dbrotate` for the owner) generates a new password, sets it in the engine, verifies a login, writes a new vault version, and only then retires the old one. Apps reading the vault themselves pick up the new version on their own schedule; the old password stays valid for a grace window (setting below) so they do not break mid rotation.
- Operators: the explorer uses brokered logins (ADR 082), so no operator ever needs the owner credential. Reveal of the owner credential stays root plus step up and, with a vault source, reveals the reference and a link to the vault path, not the value.
- The root credential at rest, three levels, the operator picks one:
  1. Default: stored in `internal/secrets` as today. With `APP_KMS_PROVIDER=vault-transit` the master key is already wrapped by the vault, so a stolen control plane database is not enough. This is the recommendation for most installs.
  2. Root in the vault only: Levelrail writes the root credential to `levelrail/<project>/<database>/_root` and keeps no copy. At every container create the reconciler resolves it (an init step) and passes it as env as today. Honest limits: the value still exists in the container env, in `docker inspect` output for anyone with Docker socket access on that node, and in the engine's own data directory as a hash. A vault outage blocks container recreate (restart after a crash, upgrade, move) for that database, which is a real availability cost. The reconciler therefore refuses this mode unless the operator confirms it in the switch wizard.
  3. A cloud KMS wrapper for the master key (AWS KMS first) as a new `secrets.KeyWrapper` next to Transit. Same protection class as level 1 for installs that have a cloud KMS but no vault. Later slice.
- Not offered: rotating the root credential out of existence after bootstrap. Postgres and MySQL images reuse the env only on first init, but the reconciler, `dbrotate` and the pooler still need an admin path, and a database with no admin login cannot be repaired by Levelrail.

#### b. Hosted and external databases

The credential lives only in the vault. The `ExternalDatabase` record holds a reference (`secret://<conn>/<path>#<key>` or the ADR 085 `secretsmanager:<arn>` form, normalized to the same `Ref`). Levelrail resolves it at use: explorer session start, health probes, backups, schema reads. Values are cached in memory only, keyed by reference and version, for `databases.credentials.cache_seconds`; never written to disk. A 401 or auth failure from the database drops the cache entry and retries once with a fresh read, which covers rotation done outside Levelrail. Adoption of an RDS instance with a managed master secret uses the ARN directly, which is what ADR 085 already planned. The resolve goes through `extdb.Conn.Validate` and `CheckResolved` before the credential is sent, unchanged.

#### c. Dynamic credentials

With source `vault_dynamic`, the vault is the broker. Levelrail configures nothing in the engine itself; the operator (or a guided wizard that prints the commands) sets up a Vault database secrets engine connection and roles that match the ADR 082 presets, named `levelrail-<database>-<preset>`.

- Explorer: each brokered login of ADR 082 becomes a `creds/<role>` read with a TTL of `databases.credentials.dynamic_ttl_minutes`. The lease id is kept with the explorer session; closing the session or the sweeper revokes it. Break glass reads the `owner` preset role with the same step up, reason and duration rules as ADR 082, and revokes on expiry.
- Apps: the default is that the app reads `creds/<role>` itself with its own vault auth. Levelrail can optionally issue a credential per deployment (one lease per container create, renewed by the control plane at `databases.credentials.renew_percent` of the TTL, revoked on container removal). This option is off by default because a control plane outage then stops renewals and the app loses its database when the lease ends; the UI says so.
- Probes and backups use their own short lease per run, revoked at the end.
- Only HashiCorp Vault (and compatible) supports this in the first cut. AWS Secrets Manager has rotation lambdas, not per request credentials, so it is static only. RDS IAM auth tokens (ADR 085) are the AWS equivalent and stay a separate later path.

### 2. Provider adapter interface

Extend `internal/secretsource` with optional interfaces, discovered through `Capabilities`, so existing read only providers keep working:

- `Provider` (exists): `Resolve`.
- `Writer`: `Write(ctx, ref, fields map[string]Value) (version string, err error)`, check and set on the expected version where the backend supports it.
- `Versioner`: `Versions(ctx, ref) ([]VersionInfo, error)` returning version, created time and state, never values.
- `Rotator`: `Rotate(ctx, ref)` for backends with native rotation (Secrets Manager `RotateSecret`); otherwise Levelrail rotates and writes.
- `Health`: `Probe(ctx)` and `ProbePath(ctx, path, wantWrite bool)` (exists in `secretconn`, gains the write check through Vault `capabilities-self` on `data/` and `metadata/`).
- `Renewer` (exists) and `Revoker`: `Revoke(ctx, lease)`.
- `Issuer`: `Issue(ctx, role string, ttl) (Value, Lease, error)` for dynamic credentials.

First providers:

1. HashiCorp Vault KV v2: read (exists), write with CAS, versions from `metadata/`, delete of old versions not used (Levelrail never destroys versions; the vault's own `max_versions` applies).
2. HashiCorp Vault database engine: `Issuer`, `Renewer`, `Revoker`. Reuses the lease handling pattern of `vault.AWSBroker`.
3. AWS Secrets Manager: read and write through `internal/awsx/secrets`, versions through stage labels (`AWSCURRENT`, `AWSPENDING`, `AWSPREVIOUS`), signed by the cloud account (ADR 085), scoped to an ARN or name prefix.
4. Generic external reference: any existing `secretconn` provider (Doppler, Infisical, 1Password) as read only. Databases can use them for hosted credentials but not for push or rotation; the UI shows "read only source".

Google Secret Manager and Azure Key Vault follow the same interface later; no slice here.

Auth methods: Vault token, AppRole (both exist), Kubernetes style JWT auth (`auth/<mount>/login` with a projected token or an operator supplied JWT file), and AWS IAM (exists through `awslogin.go`). Levelrail never asks for and refuses a root token: on token auth, connection test calls `auth/token/lookup-self` and rejects a token whose policies include `root` or that has no TTL, with a message that names the minimum policy. A sample least privilege policy is printed in the wizard: `create`, `update`, `read` on `<mount>/data/levelrail/<project>/*`, `read` on `<mount>/metadata/levelrail/<project>/*`, `read` on `<db-mount>/creds/levelrail-*`, `update` on `sys/leases/renew` and `sys/leases/revoke`.

Connection settings (address, namespace, mount, role id, CA bundle) stay in the existing `secret_connections` rows; the credential (token, secret id, JWT file path) stays sealed in `internal/secrets` as today. A database never stores a vault credential of its own; it names a connection.

Paths: `levelrail/<project>/<database>/<user>` with fields `username`, `password`, `host`, `port`, `database`, `url`. `_root` is reserved for the root only mode. Project and database names are already restricted to `[a-z0-9-]`; user names go through `dbaccess.ValidateRoleName`. The path is built by Levelrail, never typed by a user, and must pass the connection's `Policy.PathAllowed`. Vault namespaces come from the connection.

TLS and reach: every provider call uses the `vault/guard.go` dialer (or the `awsx` client for AWS). HTTPS is required except to loopback. Private addresses follow the existing `KeyVaultAllowPrivate` setting, which is admin set per install; link local and metadata ranges are always blocked. The database endpoint returned inside a secret (the `host` field) is not trusted: it goes through `extdb.Conn.Validate` and `CheckResolved` like a typed host.

Failure behavior:

- Vault down, apps reading the vault themselves: unaffected. Levelrail is not in their path.
- Vault down, Levelrail injecting values: existing containers keep running; a container recreate fails with a clear reason and the reconciler retries with backoff. Rotation is blocked (it cannot write the new version) and reports `vault_unreachable` without touching the engine.
- Explorer, probes, backups for hosted or dynamic sources: degraded. Cached values are used until `databases.credentials.stale_grace_seconds` after their cache expiry, then the explorer shows "credentials source unreachable" and offers break glass.
- Break glass when the vault is down: for managed databases only, root with a fresh step up can use the local admin path (exec over the container socket, which needs no password) for `databases.breakglass.max_minutes`. Hosted databases have no local path; Levelrail says so rather than pretend.

Audit: every resolve, write, version list, issue, renew and revoke writes an `audit_log` row with action, connection, path, version, lease id hash, actor, request id and the result class. Values never appear; a value fingerprint (HMAC of the value with a per install key, first 12 hex) is stored so an operator can tell two versions apart. Explorer use of a dynamic credential also writes the ADR 082 `database_statement_audit` row with `login_mode=vault_dynamic`.

IAM abilities (ADR 082 style): `db:credentials:read_source` (see source, status, reference), `db:credentials:manage` (switch source, rotate, test), `db:credentials:reveal` (root, step up), `secrets:providers:manage` (add and test connections, root). AWS: the database read tier gains `secretsmanager:GetSecretValue` and `DescribeSecret` on `rds!*` and `levelrail/*`, and a write tier adds `PutSecretValue`, `CreateSecret` on `levelrail/*` only.

Step up: revealing a value (owner or app user) and switching a source both need a fresh step up, as `database_credentials_reveal.go` does now.

### 3. How apps consume

- Reference mode (default for vault sources): the app env gets `VAULT_ADDR`, `VAULT_NAMESPACE` when set, and `<NAME>_DATABASE_SECRET_PATH` (for example `secret/data/levelrail/shop/orders/orders_app`). No password in the container env. The app authenticates with its own method (AppRole, Kubernetes, AWS IAM). For Secrets Manager the env gets `<NAME>_DATABASE_SECRET_ARN` and `AWS_REGION`.
- Value mode (opt in per attachment): Levelrail resolves the reference at container create and injects `DATABASE_URL` as today. This keeps apps that cannot talk to a vault working, at the cost that the value is in the container env.
- Agent or CSI style sidecar: out of scope. The design leaves room: the reference env names are the same ones a sidecar template would consume, and a later slice can add a sidecar option to the app spec that mounts a rendered file. Not built, not promised.
- Connect dialog (`web/src/components/databases/ConnectDialog.tsx`): with a vault source it shows short snippets with the real path filled in for Spring Cloud Vault (`spring.cloud.vault.kv` with the backend and path), Node (`node-vault` read), Python (`hvac` `secrets.kv.v2.read_secret_version`), Go (`github.com/hashicorp/vault/api` `KVv2(...).Get`), and the AWS SDK `GetSecretValue` for Secrets Manager. Snippets contain no value.

### 4. UI

- Database page: a "Credentials source" card with the source (Levelrail managed, Vault, AWS Secrets Manager, read only source), connection name, path or ARN, version, last rotated, last successful read, status (ok, degraded, unreachable, permission missing), and actions: test connection (read and, when needed, write capability), rotate now, open in vault (link built from the address and path), switch source.
- Switch source wizard: pick a connection, preview the paths that will be written and the attachments that change from value to reference, a dry run that tests write capability on each path without writing a value (Vault `capabilities-self`; Secrets Manager `DescribeSecret` plus a simulated policy check where the account allows it), then apply. Apply writes, verifies a read back by fingerprint, switches the reference rows, and only then deletes local copies. Root only mode is a separate confirmed toggle inside the wizard.
- Create flow: a step "Where do credentials live?" with Levelrail managed preselected, and Vault or Secrets Manager shown only when a connection exists (otherwise a link to add one).
- Explorer status line: "credentials from Vault (orders, v7)" or "credentials from Levelrail", plus the lease time left for dynamic sessions.

### 5. Data model, API, CLI, IaC, migration

Data model (names only): `database_credential_sources` (database, kind, connection, mount, base path, mode, root_mode, created, updated), `database_credential_refs` (database, principal, ref, version, fingerprint, last_rotated, last_read, state), `database_credential_leases` (database, session or container id, lease id hash, role, expires, revoked). Lease ids are sealed through `internal/secrets`.

API:

- `GET /api/v1/databases/{name}/credentials/source`, `PUT` (switch, with `dry_run`), `POST .../credentials/test`, `POST .../credentials/rotate` (principal optional), `GET .../credentials/versions`.
- `POST /api/v1/secrets/providers`, `POST /api/v1/secrets/providers/{conn}/test` on the existing secret connections routes.

CLI: `levelrail databases credentials source <db> [--set vault --connection <c>] [--dry-run]`, `levelrail databases credentials rotate <db> [--user <u>]`, `levelrail databases credentials test <db>`, `levelrail secrets providers add|test|list`.

Spec and IaC: database spec gains `credentials: { source: vault_kv, connection: corp-vault, mode: reference, root: levelrail }`; attachments gain `credentials_mode: reference | value`. Values never appear in specs; drift shows a source change as a planned change with the dry run result.

Migration: none by default. Every existing database reads as `source: levelrail`, `mode: value`, and nothing changes until an operator switches.

### 6. Settings (runtime settings, env only seeds defaults)

| Setting | Seed env | Default |
| --- | --- | --- |
| `databases.credentials.cache_seconds` | `APP_DB_CRED_CACHE_SECONDS` | 60 |
| `databases.credentials.stale_grace_seconds` | `APP_DB_CRED_STALE_GRACE_SECONDS` | 300 |
| `databases.credentials.dynamic_ttl_minutes` | `APP_DB_CRED_DYNAMIC_TTL_MINUTES` | 30 |
| `databases.credentials.renew_percent` | `APP_DB_CRED_RENEW_PERCENT` | 70 |
| `databases.credentials.rotation_grace_minutes` | `APP_DB_CRED_ROTATION_GRACE_MINUTES` | 60 |
| `databases.credentials.write_retries` | `APP_DB_CRED_WRITE_RETRIES` | 3 |
| `databases.credentials.vault_enabled` (flag) | `APP_DB_CRED_VAULT_ENABLED` | off |

Vault timeout, private range and session renewal reuse the existing `KeySecretProviderTimeout`, `KeyVaultAllowPrivate` and `KeyVaultTokenRenewAfter`.

## Alternatives rejected

- A separate vault client for databases: duplicates the guard, session and redaction code in `internal/vault` and `secretsource`. Extending the interface keeps one audited path.
- Always injecting values from the vault into app env: easy, but defeats the reason teams use a vault (the value ends up in the container env and `docker inspect`). Kept only as opt in.
- Storing the root credential only in the vault by default: makes every container recreate depend on vault reachability. Offered as opt in with the cost stated.
- Levelrail configuring the Vault database engine with its own privileged token: would need a highly privileged Vault policy held by Levelrail. Rejected; the operator configures the engine, Levelrail only reads `creds/`.
- Letting users type a vault path for a database: invites path traversal and confused deputy reads. Paths are built from validated names.

## Security

Threat model:

- Vault token theft from the control plane: the credential is sealed under the master key (Transit wrapped when configured), scoped by the least privilege policy to `levelrail/<project>/*` and `creds/levelrail-*`, short TTL with renewal, root tokens refused. A thief gets database credentials for Levelrail managed paths only, which they could already get from a compromised control plane today.
- Confused deputy through references: a user with `db:credentials:manage` on database A tries to point A at another team's path. Prevented by built paths, `Policy.PathAllowed` and `CheckResolve` per environment, and rejection of `..`, `%2e`, empty segments and the `data/` or `metadata/` prefix in any segment (the `secretconn.cleanPath` rules). Hosted references typed by a user go through the same prefix allow list of the connection.
- TOCTOU on rotation: set in engine, verify, write new version with CAS on the expected version, then retire. A concurrent outside write makes CAS fail; Levelrail rolls the engine back to the old password (existing `dbrotate.rollback`) and marks `reference_update_failed`. The per database lock in `dbrotate` already serializes rotations.
- Leaked values in logs and audit: `secretsource.Value` masks format verbs, provider errors go through `Redactor.MaskError` and `awsx/secrets.Redact`, audit stores fingerprints only, `extdb.Conn.Scrub` strips the password from engine errors. Test: a guard test that resolves a fake secret and greps every log and audit line for the value.
- Secret contents as an SSRF vector: the `host` inside a secret goes through `extdb` checks.
- Lease leakage: leases are revoked on session close, container removal and by a sweeper; the sweeper also revokes leases whose session no longer exists.

Security review checklist (for lr-reviewer): root token refusal tested; path builder fuzzed; CAS used on every write; no value in any error, log, audit row or API response except reveal; reveal needs root plus fresh step up; dial guard used for every provider call; private range setting respected; lease revoke on every exit path; switch wizard deletes local copies only after verified read back; spec export contains no values.

## Failure modes

- Write succeeds in the vault, local reference update fails: the next sweep reads the latest version, compares fingerprints and repairs the reference row.
- Engine accepts the new password, vault write fails after retries: engine rolled back to the old password, state `reference_update_failed`, alert raised.
- Vault returns a version older than the reference (restore from backup on the vault side): Levelrail refuses to downgrade silently and shows "vault version behind Levelrail", with an action to accept.
- Lease renew fails: the session or container gets a warning at 90 percent of TTL; the explorer reissues; app per deployment leases fall back to a redeploy.
- Connection credential rotated outside Levelrail: existing grace window in `vault/grace.go`, then the source shows "permission missing".
- Secrets Manager rotation lambda changes the password: next auth failure drops the cache and re reads `AWSCURRENT`.

## What cannot be verified locally

- Real HashiCorp Vault Enterprise namespaces, performance standbys and their redirect behavior; the dev server has no namespaces.
- Vault Kubernetes auth with a real cluster's projected service account token issuer.
- AWS Secrets Manager stage labels with a real rotation lambda, IAM policy simulation, and RDS managed master secrets. Needs the AWS test account under its budget guard.
- Behavior under real network partitions between control plane, nodes and vault.
- Whether common frameworks (Spring Cloud Vault in particular) pick up a new KV version without a restart in the versions users run; the snippets document the library call, not a verified refresh behavior.

## Testing plan

- Unit: path builder and traversal fuzz, CAS conflict path, rotation state machine with `FakeProvider` extended for `Writer`, `Issuer`, `Revoker`, root token refusal, redaction guard test over logs and audit.
- Integration with a local Vault dev server (or OpenBao) in a container: KV v2 write, versions, database engine against a local Postgres, lease renew and revoke, Vault down during rotation and during explorer use.
- Real host proof on the test hosts: one managed Postgres switched to Vault reference mode, an app reading through AppRole, a rotation, and a vault outage while the app keeps serving.
- AWS proof in the test account: adopted RDS with a managed master secret, read through the ARN, explorer session, no write.
- UI: Credentials source card states and the switch wizard dry run, at two widths.

## Slices

Order matters; slices 1 to 3 share `internal/secretsource` and run in sequence. Flag `databases.credentials.vault_enabled` stays off until slice 6 is proven. [R] means lr-reviewer required.

| # | Slice | Files (owner) | Target |
| --- | --- | --- | --- |
| 1 | Optional interfaces `Writer`, `Versioner`, `Revoker`, `Issuer`; `FakeProvider` support | `internal/secretsource/source.go`, `fake.go` | 1.5.11 |
| 2 | Vault KV v2 write with CAS, versions, write capability probe, root token refusal [R] | `internal/vault/resolver.go`, `probe.go`, new `internal/vault/write.go`, `internal/secretconn/adapters.go` | 1.5.11 |
| 3 | Path builder and validation, settings keys and env seeds | new `internal/dbcreds/paths.go`, `internal/runtimesettings` keys file | 1.5.11 |
| 4 | Data model and store for sources, refs, leases | new `internal/store/database_credential_sources.go`, migration | 1.5.11 |
| 5 | Hosted databases read through a reference (explorer, probe, backup) with cache and stale grace [R] | `internal/extdb/resolve.go`, new `internal/dbcreds/resolve.go` | 1.5.11 |
| 6 | Managed app users pushed to KV, rotation through vault with rollback [R] | `internal/dbaccess/manager.go` callers, `internal/dbrotate/service.go`, new `internal/dbcreds/push.go` | 1.5.11 |
| 7 | API and CLI: source, test, rotate, versions; provider add and test [R] | new `internal/api/database_credentials_source.go`, routes file, `cmd/levelrail-cli` databases commands | 1.5.11 |
| 8 | Reference mode env for attachments and bindings | `internal/api/apps_database.go`, `internal/api/apps_connections.go`, reconciler env build | later |
| 9 | UI: Credentials source card, switch wizard with dry run, create step, explorer status line, Connect snippets | `web/src/components/databases/` new `CredentialsSourceCard.tsx`, `ConnectDialog.tsx`, `CreateDatabaseFields.tsx`, locales | later |
| 10 | AWS Secrets Manager provider: read, write, stage labels, IAM tier statements [R] | new `internal/secretconn/awssm.go`, `internal/awsx/secrets/secrets.go`, `internal/cloudconn/aws_capabilities.go` | later |
| 11 | Vault database engine: explorer brokered logins and break glass as leases, sweeper revoke [R] | new `internal/vault/dbengine.go`, `internal/dbaccess/sweeper.go`, explorer broker | later |
| 12 | Vault Kubernetes style JWT auth method [R] | `internal/vault/session.go`, `internal/store/vault_settings.go` | later |
| 13 | Root only mode for managed databases with confirm gate [R] | `internal/reconcile/database/controller.go`, `engine_env.go`, `internal/dbcreds` | later |
| 14 | AWS KMS master key wrapper, IaC spec fields, docs (including the caching line fix in `docs/vault-secret-references.md`) | `internal/secrets/` new `awskms.go`, `cmd/levelrail/kms.go`, `internal/spec`, `docs/` | later |

## Consequences

- Teams with a vault rule can run databases on Levelrail without Levelrail holding app credentials, and with reference mode without values in container env.
- Managed databases still have a root credential Levelrail can reach; this ADR reduces who sees it, it does not remove it. The UI and docs say that plainly.
- More moving parts in rotation: a vault write sits between engine change and commit, and a vault outage blocks rotation.
- The `secretsource` interface grows from read only to read and write, so every new provider needs write and CAS tests.
- Dynamic credentials depend on operator configured Vault roles; Levelrail can guide but not verify the role's grants match the preset until it issues one and reads its grants.

## Open questions

1. Should reference mode become the default for new attachments once a vault connection exists, or stay opt in per attachment?
2. Per deployment dynamic leases for apps: worth building at all, given the control plane renewal dependency, or leave apps to read `creds/` themselves?
3. Is root only mode wanted by any real user, given that it makes container recreate depend on the vault? If not, drop slice 13.
4. Which of Google Secret Manager and Azure Key Vault comes after Secrets Manager, and is it needed before 1k stars?
5. Should the fingerprint HMAC key be per install or per project, and should fingerprints be exportable for external audit joins?
6. For hosted databases adopted with a typed password today, should Levelrail offer a one click move of that password into the vault?
