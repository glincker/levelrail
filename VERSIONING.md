# Versioning

Levelrail uses semantic versioning, computed from commit types by release-please. Nobody picks a number by hand. The full rules for contributors are in [RELEASING.md](RELEASING.md).

## Patch-only releases (current setting)

Release-please runs with `versioning: always-bump-patch`, so every release is the next patch number whatever the commit types, until the owner changes the setting in `.github/release-please/release-please-config.json`. The table below describes the intended policy for when that setting is lifted. A minor or major number still needs a deliberate change by the owner.

## How a version is chosen

| Change | Bump |
| --- | --- |
| Fix, performance or security fix | patch |
| New capability, flag, API field, template or beta resource | minor |
| Deprecation of a stable item (it still works) | minor |
| Removal or incompatible change of a stable item | major |
| Change to something marked `x-stability: beta` | minor or patch, never major |

The release takes the highest bump among its commits. `Release-As:` is a repair tool used only by the owner to fix a wrong computed version. Docs, chore, CI, test and refactor commits alone produce no release.

## What counts as breaking

The stable surface is the HTTP API, CLI flags and exit codes, stable IaC fields, config keys, the Terraform provider and SDK contracts, and migrations that need operator action. Adding a field, route, flag or variable is not breaking. Removing or renaming one, or changing its meaning, is.

- **HTTP API (v1 and v2):** routes, request and response fields and status codes. Removing or renaming one is breaking. See the [API reference](docs/api-reference.md).
- **CLI:** command names, flag names, exit codes and `--json` fields. See the [CLI reference](docs/cli-reference.md).
- **Terraform provider:** resource and attribute names and meaning. The provider is built from source today and has no separate release number. See [Terraform provider](docs/terraform-provider.md).
- **Agent protocol:** the agent reports its version when it connects, and `APP_AGENT_MIN_VERSION` flags older agents. No wire compatibility promise is documented. See [Multi-node](docs/multi-node.md#agent-version).
- **Database schema:** migrations after the baseline must be additive, enforced by `go test ./internal/store`. The SQLite schema itself is internal, use the API. See [Upgrade compatibility](docs/upgrade-compatibility.md).
- **Config:** documented `APP_*` variables. See [Environment variables](docs/environment-variables.md).

Beta and experimental features are outside these promises. See [Feature status](docs/feature-status.md) and [Experimental features](docs/experimental-features.md).

## Support window

The [support policy](docs/support-policy.md) is a draft and is not in force. It proposes security fixes for the latest minor, high and critical fixes for the previous minor, and a deprecation window of at least two minor releases. Until it is confirmed, treat the latest release as the only supported one. The tracker below leaves the status column blank for versions the policy does not classify.

## Upgrading and rolling back

- Upgrade from the previous minor's latest patch. Skipping minors is best effort.
- Every upgrade takes a restore point first. A failed health check restores the old binary and, when the schema changed, the old database.
- A binary older than the database schema refuses to start and leaves the database unchanged. A downgrade through `self-upgrade --to` is refused.

Pages: [Upgrade safely](docs/upgrade-safely.md), [Upgrade compatibility](docs/upgrade-compatibility.md), [Upgrade history](docs/upgrade-history.md), [Rollback](docs/rollback.md).

## Components

The control plane, the CLI and the agent are built from this repository and released under one version number and one tag. The agent is upgraded by hand on each node. The Terraform provider lives in `integrations/terraform-provider` and is not published separately. The `kit` module is versioned on its own by the same rules.

## Release cadence

No fixed cadence is documented. Releases happen when the release branch is merged and the release PR is merged, see [RELEASING.md](RELEASING.md).

## Version tracker

Generated from `CHANGELOG.md` by `node scripts/gen-versions-table.mjs`. Do not edit by hand.

<!-- versions:begin -->
| Version | Date | Status | Notes |
| --- | --- | --- | --- |
| 1.5.7 | 2026-11-21 | Latest | [Release notes](https://github.com/glincker/levelrail/releases/tag/v1.5.7) |
| 1.5.6 | 2026-11-18 |  | [Release notes](https://github.com/glincker/levelrail/releases/tag/v1.5.6) |
| 1.5.5 | 2026-11-18 |  | [Release notes](https://github.com/glincker/levelrail/releases/tag/v1.5.5) |
| 1.5.4 | 2026-11-18 |  | [Release notes](https://github.com/glincker/levelrail/releases/tag/v1.5.4) |
| 1.5.3 | 2026-11-18 |  | [Release notes](https://github.com/glincker/levelrail/releases/tag/v1.5.3) |
| 1.5.2 | 2026-11-18 |  | [Release notes](https://github.com/glincker/levelrail/releases/tag/v1.5.2) |
| 1.5.1 | 2026-11-18 |  | [Release notes](https://github.com/glincker/levelrail/releases/tag/v1.5.1) |
| 1.5.0 | 2026-11-17 |  | [Release notes](https://github.com/glincker/levelrail/releases/tag/v1.5.0) |
| 1.4.1 | 2026-11-17 |  | [Release notes](https://github.com/glincker/levelrail/releases/tag/v1.4.1) |
| 1.4.0 | 2026-11-17 |  | [Release notes](https://github.com/glincker/levelrail/releases/tag/v1.4.0) |
| 1.3.2 | 2026-11-16 |  | [Release notes](https://github.com/glincker/levelrail/releases/tag/v1.3.2) |
| 1.3.1 | 2026-11-16 |  | [Release notes](https://github.com/glincker/levelrail/releases/tag/v1.3.1) |
| 1.3.0 | 2026-11-15 |  | [Release notes](https://github.com/glincker/levelrail/releases/tag/v1.3.0) |
| 1.2.0 | 2026-11-15 |  | [Release notes](https://github.com/glincker/levelrail/releases/tag/v1.2.0) |
| 1.1.2 | 2026-11-14 |  | [Release notes](https://github.com/glincker/levelrail/releases/tag/v1.1.2) |
| 1.1.1 | 2026-11-14 |  | [Release notes](https://github.com/glincker/levelrail/releases/tag/v1.1.1) |
| 1.1.0 | 2026-11-14 |  | [Release notes](https://github.com/glincker/levelrail/releases/tag/v1.1.0) |
| 1.0.3 | 2026-11-13 |  | [Release notes](https://github.com/glincker/levelrail/releases/tag/v1.0.3) |
| 1.0.2 | 2026-11-13 |  | [Release notes](https://github.com/glincker/levelrail/releases/tag/v1.0.2) |
| 1.0.1 | 2026-11-12 |  | [Release notes](https://github.com/glincker/levelrail/releases/tag/v1.0.1) |
| 0.9.6 | 2026-11-10 |  | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.9.6) |
| 0.9.5 | 2026-11-08 |  | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.9.5) |
| 0.9.3 | 2026-11-07 |  | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.9.3) |
| 0.9.2 | 2026-11-06 |  | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.9.2) |
| 0.9.0 | 2026-11-02 |  | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.9.0) |
| 0.8.8 | 2026-10-30 |  | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.8.8) |
| 0.8.7 | 2026-10-29 |  | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.8.7) |
| 0.8.6 | 2026-10-27 |  | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.8.6) |
| 0.8.5 | 2026-10-26 |  | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.8.5) |
| 0.8.4 | 2026-10-24 |  | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.8.4) |
| 0.8.3 | 2026-10-23 |  | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.8.3) |
| 0.8.2 | 2026-10-22 |  | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.8.2) |
| 0.8.1 | 2026-10-22 |  | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.8.1) |
| 0.8.0 | 2026-10-21 |  | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.8.0) |
| 0.7.0 | 2026-10-20 |  | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.7.0) |
| 0.6.0 | 2026-10-17 |  | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.6.0) |
| 0.5.1 | 2026-10-16 |  | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.5.1) |
| 0.5.0 | 2026-10-16 |  | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.5.0) |
| 0.4.0 | 2026-10-15 |  | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.4.0) |
| 0.3.0 | 2026-10-12 |  | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.3.0) |
| 0.2.2 | 2026-10-12 |  | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.2.2) |
| 0.2.1 | 2026-10-12 |  | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.2.1) |
| 0.2.0 | 2026-10-12 |  | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.2.0) |
| 0.2.0-beta.20 | 2026-10-11 | Pre-release | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.2.0-beta.20) |
| 0.2.0-beta.19 | 2026-10-11 | Pre-release | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.2.0-beta.19) |
| 0.2.0-beta.18 | 2026-10-05 | Pre-release | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.2.0-beta.18) |
| 0.2.0-beta.17 | 2026-10-05 | Pre-release | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.2.0-beta.17) |
| 0.2.0-beta.16 | 2026-10-05 | Pre-release | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.2.0-beta.16) |
| 0.2.0-beta.15 | 2026-10-05 | Pre-release | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.2.0-beta.15) |
| 0.2.0-beta.14 | 2026-09-23 | Pre-release | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.2.0-beta.14) |
| 0.2.0-beta.13 | 2026-09-23 | Pre-release | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.2.0-beta.13) |
| 0.2.0-beta.12 | 2026-09-23 | Pre-release | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.2.0-beta.12) |
| 0.2.0-beta.11 | 2026-09-23 | Pre-release | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.2.0-beta.11) |
| 0.2.0-beta.10 | 2026-09-23 | Pre-release | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.2.0-beta.10) |
| 0.2.0-beta.9 | 2026-09-23 | Pre-release | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.2.0-beta.9) |
| 0.2.0-beta.8 | 2026-09-23 | Pre-release | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.2.0-beta.8) |
| 0.2.0-beta.7 | 2026-09-23 | Pre-release | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.2.0-beta.7) |
| 0.2.0-beta.6 | 2026-09-23 | Pre-release | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.2.0-beta.6) |
| 0.2.0-beta.5 | 2026-09-21 | Pre-release | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.2.0-beta.5) |
| 0.2.0-beta.4 | 2026-09-20 | Pre-release | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.2.0-beta.4) |
| 0.2.0-beta.3 | 2026-09-19 | Pre-release | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.2.0-beta.3) |
| 0.2.0-beta.2 | 2026-09-18 | Pre-release | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.2.0-beta.2) |
| 0.2.0-beta.1 | 2026-09-13 | Pre-release | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.2.0-beta.1) |
| 0.2.0-beta | 2026-09-12 | Pre-release | [Release notes](https://github.com/glincker/levelrail/releases/tag/v0.2.0-beta) |
<!-- versions:end -->
