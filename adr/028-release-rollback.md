# ADR 028: Roll back a release from the host, never from the control plane

Status: Accepted

Date: 2026-10-09

## Context

Operators asked to return to an earlier release from Settings > Updates. Two
facts shape any answer:

- Migrations are forward-only (ADR 007). An older binary refuses to start on a
  newer schema ("database is at version N, this binary supports up to M"). So
  replacing the binary is safe only when the target supports a schema at least
  as new as the database. Otherwise the only way back is restoring a database
  backup, which permanently loses everything written since.
- The control plane never upgrades itself, and the dashboard says so. A process
  that can replace its own binary and restart is a root-level code execution
  path behind an API token.

## Decision

Rollback is planned in the product and applied on the host.

- **Plan (read-only, API, CLI, dashboard, MCP):** `GET /api/v1/updates/releases`
  lists the last 5 releases per channel with a per-release verdict, and
  `GET /api/v1/updates/rollback-plan?version=` runs the existing preflight
  checks plus the schema verdict. Verdicts are `binary_only`, `forward`,
  `restore_required` and `unknown`. A release's schema version comes from a
  retained binary's recorded metadata, else from the `release-manifest.json`
  asset the release workflow now publishes (the output of
  `levelrail version --json`). Releases with neither report `unknown`; it is
  never guessed.
- **Apply (host only):** `sudo levelrail rollback --to <version>`, run by an
  operator on the server. It requires typed confirmation of the version, takes
  a fresh control plane backup, re-verifies the retained binary against its
  recorded SHA-256, stops the service, swaps the binary, starts it, and waits
  for health and readiness. If the target does not become healthy within the
  timeout, the previous binary (and database, when one was restored) is put
  back automatically and the command says so. Every step is written to the
  audit log.
- **Schema guard:** a binary-only rollback across a newer schema is refused.
  The only path is `--restore-backup <name> --confirm-data-loss <name>`, which
  shows the backup's timestamp and schema and requires the backup name typed
  back. An `unknown` verdict is refused unless `--accept-unknown-schema` is
  passed (the older binary refuses a newer schema without touching data, and
  automatic recovery covers the rest).
- **Retention:** `install.sh` upgrades keep the last 3 release binaries in
  `<install dir>/levelrail.releases/` with a SHA-256 and schema sidecar, so a
  rollback to a retained release is instant and works offline.
  `install.sh retain` downloads and verifies (SHA-256 and cosign signature) any
  other release into the same directory; `rollback` then applies it. Download
  and signature verification stay in the one installer code path that already
  does it.
- **MCP is read-only:** `list_releases` and `get_rollback_plan` only. A test
  asserts that no tool applies an upgrade or rollback.

## Rejected alternatives

- **A privileged helper or in-process swap behind an admin button.** Needs root
  or systemd control from the control plane process, so a stolen admin token
  becomes arbitrary code as root, and it removes the stated property that the
  control plane never upgrades itself. A button also cannot honestly show the
  data-loss consequence of a restore.
- **A separate always-on updater daemon.** Same trust problem plus a new
  component to ship, secure and keep alive across the very upgrades it
  performs.
- **Downgrade migrations.** Contradicts ADR 007 (forward-only by design) and
  multiplies the test matrix; a bad down migration destroys data silently.
- **Always restoring the pre-upgrade backup.** Loses data even when the schema
  did not change. The binary-only path is the common case and costs nothing.
- **Guessing schema compatibility from version numbers.** Releases do not map
  to schema versions monotonically across channels; an `unknown` verdict is
  honest, a wrong `binary_only` is not.

## Consequences

- Rolling back needs shell access to the host. That is the same bar as
  installing and upgrading, and it is the point.
- Releases published before this ADR have no manifest. They show `unknown`
  until a retained copy records its schema (any release installed through
  `install.sh` after this change does).
- Retaining 3 binaries costs about 3 x the binary size on disk.
