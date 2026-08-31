# Changelog

All notable changes to Levelrail are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
No tagged release exists yet, so everything below is unreleased. Once the
first version is tagged, `.github/workflows/release.yml` injects that tag
into `internal/version.Version`, and this file gains its first dated
version heading.

## Unreleased

### Added

- Version surface: the running build version (`internal/version.Version`,
  injected via `-ldflags` at release build time) is now checkable through
  `GET /api/v1/updates`, the Settings > Updates dashboard page, and
  `levelrail-cli version`.
- Migration and integration tooling: guided migration from Coolify,
  Dokploy, and CapRover, plus first-class Bitbucket Cloud and GitHub
  Enterprise Server support alongside GitHub.com.
- Operational safety nets: proactive alerting for certificate expiry,
  node disk space, node patch status, node CPU/memory usage, and repeated
  scheduled task failures; deploy failure and crashloop diagnosis; backup
  integrity verification with auto-verify after scheduled backups.
- Delivery workflow features: protected environments with a
  confirm-to-deploy gate, deploy comparison view, image promotion across
  environments, webhook delivery history with replay, audit log CSV
  export, and downloadable log export.
- Resource guidance: deterministic right-sizing recommendations for both
  apps and databases.
- Feature flags with live evaluation via existing API tokens, wired into
  the CLI, dashboard, and MCP tools.
- General per-actor API rate limiting.
- Guided first-run onboarding flow for zero-app instances.
- Expanded `levelrail-cli`: shell completion (bash, zsh, fish), a
  `doctor` preflight command, and CSV/JSON support across more commands.
- Additional MCP tools covering feature flags, doctor, webhook
  deliveries, backup verification, deploy comparison, and notification
  deliveries.

### Fixed

- A TTL fallback sweep now catches preview environments left behind by a
  missed pull-request-closed webhook, across all supported git providers.
- Persisted build logs now feed the crashloop/deploy diagnose flow for
  build-phase failures, not just runtime failures.
- GitHub App installation status is now verified live against GitHub
  rather than trusted from a possibly stale local row.

This list summarizes recent work rather than enumerating every merged
PR; see `git log` for the full, authoritative history.
