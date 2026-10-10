---
description: Manage database users, issue short-lived credentials, see who can reach a database, and control its network exposure, scope and TLS from the dashboard, CLI or API.
---

# Database access and network

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

## Network

### Reachability

A plain language verdict ("Private: only these 3 apps can reach it", "Exposed to
the internet on 5432"), the internal host and address, the Docker networks it is
attached to, which apps connect (with project and environment), and whether a port
is published and to which address.

**Make private** stops publishing the port and removes any rule that guarded it.

### Allowed sources

An inbound rules editor for a published port: source IP or CIDR plus an optional
description. Rules are applied through the same host firewall integration as the
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

Three read-only tools: `list_database_users`, `get_database_access` and
`get_database_reachability`. None returns a secret, and there is no tool that
creates users, issues credentials or changes rules.

## Permissions and audit

Reading role names and platform access needs `read:sensitive` on the database.
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
