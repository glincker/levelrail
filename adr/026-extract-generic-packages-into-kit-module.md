# ADR 026: Extract generic packages into a nested kit module

Status: Accepted

Date: 2026-10-05

## Context

- Several packages under `internal/` (SSRF-safe HTTP client, cron parsing, path globs, health probes, slow-query parsers, disk forecasting, stack detection, and others) have no dependency on the platform.
- `internal/` makes them importable by nobody outside this module, so they cannot be reused or discovered.

## Decision

- Move them into a nested Go module at `kit/` (`github.com/GLINCKER/levelrail/kit`) that depends on the standard library only.
- The main module consumes it through a `replace` directive, so local builds always use the working tree.
- Tag the kit independently as `kit/vX.Y.Z`. It stays at v0.x: the API may change between minor versions.
- Keep it in this repository. Splitting out would add cross-repo PRs and releases for every change that touches both sides.

## Rejected alternatives

- Move to `pkg/` in the root module: consumers would inherit the whole module graph (Docker, Caddy, BuildKit, SQLite) and a version tied to platform releases.
- Separate repository now: cross-repo release overhead before the APIs have settled.
- Leave in `internal/`: no reuse.

## Consequences

- CI needs a lane that vets and tests the nested module, since `./...` from the root does not cross a module boundary.
- The brand rule applies to `kit/`: no product name in source.
- The kit can later move to its own repository with `git filter-repo` plus an import path rename.
- Product-specific packages such as `sshprovision` and `objectstore` intentionally stay in `internal/`.
