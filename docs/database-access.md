---
title: "Database access and network"
description: "Manage database users, issue short-lived credentials, see who can reach a database, and control its network exposure, scope and TLS from the UI, CLI or API."
"og:url": "https://docs.levelrail.com/database-access"
---

Two tabs on every database give you the controls you would expect from a managed
database service: **Access** (who can log in, and who can manage it) and
**Network** (who can reach it, and how far).

Users and temporary credentials are available for PostgreSQL. Network scope, allowed
sources and the reachability map work for every engine.

## Access

### Database users

Create a login for a teammate or a reporting tool instead of sharing the main
password. Pick a preset:

| Preset | What it can do |
| --- | --- |
| Read only | Read every table. Writes are refused even if a client asks for them. |
| Read and write | Read and change rows. Cannot change the schema. |
| Owner | Everything on this one database, including schema. No server level rights. |

The password is generated, shown once and never stored. You can set a connection
limit and an expiry. Rotate, disable (ends open sessions) and delete are one click
each. The platform's own admin role and any superuser are protected and cannot be
changed here, and a name that collides with a system role is refused.

Your apps keep using the platform's credentials, so a user here never breaks them.

### Temporary credentials

**Get temporary credentials** mints a short-lived read-only or read-write login
for a developer or a one-off job. Lifetime is 15 minutes to 24 hours (default 1
hour). It shows the connection string once and removes the role at expiry.

Expiry is enforced twice: the role carries `VALID UNTIL` so the server refuses
logins even while the control plane is down, and a sweeper drops the role and ends
its sessions. The sweeper runs at boot and on a timer, claims each credential
atomically so it is revoked exactly once, and writes an audit entry as the
`database-access-sweeper` system actor.

### Who can manage this database

Lists the users and tokens with abilities on this database, from their own
abilities and from IAM policies, and the policies that mention it. **Grant
access** applies one of three database scoped IAM templates to a user or token,
previews the exact policy, and records an audit entry:

- `database-read-only`: read the database and its data, any change denied.
- `database-operator`: read plus day to day operations, not users, restore or the write console.
- `database-owner`: everything on this one database.

### Owner password rotation

The **Owner password** card on the Access tab rotates the password your apps use
for PostgreSQL, MySQL, MariaDB and MongoDB. Redis family engines, search engines,
ClickHouse and Memcached have no owner password the platform can change, so the
card says so instead of offering a button.

A rotation is staged so apps never point at a dead password:

1. The new password is generated and held encrypted beside the current one.
2. The database is changed over its own container (the password travels on stdin only).
3. A real login with the new password is checked. If it is refused, the old password is put back and nothing else changes.
4. The stored credential switches and the running apps linked to the database restart to pick it up. Stopped apps pick it up when they start.

The dialog previews which apps restart before you confirm, and a real rotation
asks for your password (and authenticator code). Single password engines have a
brief window between steps 2 and 4 where a restarting app reconnects, which is
why the preview lists them. If the control plane stops between steps, the
rotation is settled on the next pass (hourly by default): whichever password the
database actually holds is confirmed and the stored credential follows it.

Set **Rotate every (days)** to rotate on a schedule; 0 turns it off. The card
shows the password age, the state and the last failure reason. Audit entries are
`database.password.rotate.start`, `database.password.rotate`,
`database.password.rotate.rollback` and `database.password.policy`, and never carry
the password.

## Network

### Reachability

A plain language verdict ("Private: only these 3 apps can reach it", "Exposed to
the internet on 5432"), the internal host and address, the Docker networks it is
attached to, which apps connect (with project and environment), and whether a port
is published and to which address.

**Make private** stops publishing the port and removes any rule that guarded it.

### Allowed sources

An inbound rules editor for a published port: source IP or CIDR plus an optional
description. Rules are applied through the same [host firewall](/host-firewall) integration as the
[exposure audit](/exposure-audit): tagged, idempotent, previewed first, guarded against
locking out the platform or SSH, and removable. Drift (a rule missing from, or
extra in, the firewall) is shown inline. Rules can only be applied on the control
plane's own Linux host.

### Network scope

`Whole platform` (default), `Same project only` or `Same environment only`.
Check impact first: it lists which referencing apps would lose reachability.
Applying a scope that cuts apps off needs confirmation. Out of scope apps are
detached from the database on the next reconcile and show the reason on their
status. Setting the scope back to the whole platform reverses it.

Scope limits which apps the platform attaches to the database. Containers on
Docker's default bridge on the same host can still reach it by its bridge address;
the reachability map says so. See ADR 029.

### Require TLS

Refuses connections that are not encrypted. The connection strings this platform
hands out already ask for TLS. Clients configured with `sslmode=disable` are
refused once this is on.

## CLI

```
levelrail-cli databases users list <database>
levelrail-cli databases users create <database> <name> --preset read_only --expires-in-days 30
levelrail-cli databases users rotate|disable|enable|delete <database> <name>
levelrail-cli databases rotate-password <database> --dry-run
levelrail-cli databases rotate-password <database>
levelrail-cli databases rotate-password <database> --every-days 30
levelrail-cli databases rotate-password <database> --status
levelrail-cli databases access temp <database> --preset read_write --minutes 60
levelrail-cli databases access list|revoke|who|grant <database> ...
levelrail-cli databases network show <database>
levelrail-cli databases network allow <database> --source 203.0.113.7/32 --description laptop --confirm
levelrail-cli databases network deny <database> --source 203.0.113.7/32 --confirm
levelrail-cli databases network scope <database> --scope project --dry-run
levelrail-cli databases network tls <database> --require
```

Every command takes `--json`.

## MCP

Four read-only tools: `list_database_users`, `get_database_access`,
`get_database_reachability` and `get_database_password_rotation` (age, state,
schedule, apps a rotation would restart). None returns a secret, and there is no tool that
creates users, issues credentials or changes rules.

## Permissions and audit

Reading role names and platform access needs `read:sensitive` on the database.
Rotating the owner password needs `write:sensitive` plus a fresh step-up.
Creating, rotating, disabling, deleting, issuing credentials, granting access and
every network change need `root` on the database, which a database scoped IAM
policy can grant. Each change is written to the audit log with an action name
(`database.user.create`, `database.temp.issue`, `database.network.scope.set`, and
so on). Passwords never appear in the audit log, application logs or API list
responses, and are sent to the database as SCRAM verifiers, so even a failed
statement logged by the server carries no plaintext.

## Configuration

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_DB_TEMP_TTL_MIN_MINUTES` | 15 | Shortest temporary credential lifetime |
| `APP_DB_TEMP_TTL_MAX_MINUTES` | 1440 | Longest temporary credential lifetime |
| `APP_DB_TEMP_TTL_DEFAULT_MINUTES` | 60 | Lifetime when none is requested |
| `APP_DB_TEMP_SWEEP_SECONDS` | 30 | How often expired credentials are removed |
| `APP_DATABASE_PASSWORD_ROTATION_INTERVAL_SECONDS` | 3600 | How often scheduled and interrupted password rotations are processed |

## Troubleshooting

### User creation fails with "role already exists" or "invalid role name"

Likely cause: The username is already in use, or it conflicts with a PostgreSQL system role.

How to confirm: List existing users: `levelrail-cli databases users list <database>`. Check if the name appears. System roles (like "postgres", "pg_*") cannot be created through this interface.

Fix: Choose a different username that does not exist and does not match a PostgreSQL system role. You can also rotate an existing user's password instead of creating a new one: `levelrail-cli databases users rotate <database> <existing-name>`.

Still stuck: See [Database access](/database-access#database-users) for user creation syntax.

### Temporary credentials expired or cannot log in

Likely cause: The temporary credential has expired (lifetime is 15 minutes to 24 hours), or the role was already removed by the sweeper.

How to confirm: Temporary credentials are valid from the moment of issue until the expiry time. Check when it was issued and what lifetime you requested: `levelrail-cli databases access temp <database>` shows the expiry. If it is in the past, the credential is no longer valid.

Fix: Issue a new temporary credential: `levelrail-cli databases access temp <database> --preset read_only --minutes 60`. Each credential is single-use: issue a new one each time you need access.

Still stuck: See [Temporary credentials](/database-access#temporary-credentials) for the full process.

### "Permission denied" when using a read-only user account

Likely cause: The account role has write restrictions, or the user is attempting a schema change (which read-only users cannot do).

How to confirm: Check the user preset: `levelrail-cli databases users list <database>` shows the role. If it is "read_only", the user can only SELECT, not INSERT, UPDATE, DELETE or schema changes. Check the SQL command that failed: read-only preset refuses any DML.

Fix: Either use a "read_write" user for writes (if the user needs write access), or use an application account with full permissions for your app. Create a new user with the correct preset: `levelrail-cli databases users create <database> <new-name> --preset read_write`.

Still stuck: See [Database users](/database-access#database-users) for preset permissions.

### App lost database reachability after network scope change

Likely cause: The database scope was changed to "project only" or "environment only", and the app is in a different project or environment.

How to confirm: Check the database scope: Dashboard > Databases > <database> > Network > Network scope. Check the app's project and environment: Dashboard > Apps > <app> > Settings. If the scope does not include the app's project or environment, the app cannot reach the database.

Fix: Either change the database scope back to "Whole platform", or move the app to the same project or environment as the database: `levelrail-cli apps set-project <app> <project-id>`.

Still stuck: See [Network scope](/database-access#network-scope) for the full scoping rules and reachability impact check.

### TLS required but client refuses to use TLS

Likely cause: The database has "Require TLS" enabled, but the client (app or external connection) is configured with `sslmode=disable` or `--no-tls`.

How to confirm: On the database's Network tab, check if "Require TLS" is enabled. Check the client connection string: it should include `?sslmode=require` (PostgreSQL) or equivalent for other databases.

Fix: Remove the `--no-tls` flag or change the connection string to use TLS. For PostgreSQL, the default connection string from the platform includes `?sslmode=require`. Apps attached to the database get this automatically. If using a temporary credential or custom connection, include TLS: `psql "postgresql://user:password@host:5432/dbname?sslmode=require"`.

Still stuck: See [Require TLS](/database-access#require-tls) for TLS settings per database engine.
