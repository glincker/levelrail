---
title: "Support policy (draft)"
description: Draft compatibility and support policy for a future stable line. Not yet in force.
"og:url": "https://docs.levelrail.com/support-policy"
---

**Draft, not yet in force.** Levelrail is pre-1.0 and nothing on this page is promised today. It states what the project intends to commit to when a stable line is cut, so the terms can be reviewed first. Until then, read [Feature status](/feature-status) for what is proven and [Roadmap](/roadmap) for what is open.

## What would be covered

Within one stable line (for example 1.x), these do not change in a breaking way:

| Surface | Covered | Where it is documented |
| --- | --- | --- |
| REST API v1 | Routes, request fields, response fields and status codes | [API reference](/api-reference) |
| REST API v2 | Resource shape, resource version and list and watch | [API v2](/api-v2) |
| CLI | Command names, flag names, exit codes and `--json` output fields | [CLI reference](/cli-reference), [CLI output](/cli-output) |
| App spec | Documented fields and their meaning | [App spec reference](/app-spec-reference) |
| Environment variables | Names and meaning of documented `APP_*` variables | [Environment variables](/environment-variables) |
| On-disk upgrade path | Upgrading from the previous minor's latest patch, keeping data | [Upgrade compatibility](/upgrade-compatibility), [Upgrade safely](/upgrade-safely) |

Adding a field, route, flag or variable is not a breaking change. Removing or renaming one, or changing its meaning, is.

## What would not be covered

- Experimental and beta features, including everything behind `APP_EXPERIMENTAL`. See [Experimental features](/experimental-features). They can change or be removed in any release, with a note in the changelog.
- Fields whose name starts with `x-`. They are for tooling and may change at any time.
- Undocumented routes, flags, variables and database tables. The SQLite schema is internal. Use the API.
- Multi-node and WireGuard mesh behavior while they are labelled beta in [Feature status](/feature-status).
- Dashboard layout, copy and keyboard shortcuts.
- Generated pages such as the template catalog. Templates can be added, changed or retired.

## Deprecation window

A covered item that is deprecated stays for at least two minor releases after the release that deprecates it. During that time:

1. The item still works as before.
2. Using it produces a visible warning: a response header for the API, a line on stderr for the CLI, a log line at start for a variable or spec field.
3. The docs page for the item carries a deprecation note naming the replacement and the first release that may remove it.
4. The changelog lists it under the deprecating release.

A removal ships only in a minor release after the window, never in a patch.

## Security fixes

Proposed, to be confirmed before this policy takes effect:

- The latest stable minor gets security fixes as patch releases.
- The previous minor gets fixes for high and critical issues while it is inside its two-minor deprecation window.
- Older lines get no fixes. Upgrade.
- Fixes are announced in the changelog and the [advisory feed](/advisory-feed). Releases stay signed.

Reporting a vulnerability is covered in the repository security policy.

## Upgrade path guarantee

These behaviors already exist and are tested by the upgrade harness described in [Upgrade safely](/upgrade-safely#upgrading-real-releases-with-real-data). The policy would make them a promise:

- Migrations only go forward. After the baseline release, new migrations are additive (`go test ./internal/store` enforces it).
- A binary older than the database schema refuses to start, exits with status 78 and leaves the database unchanged. A downgrade through `self-upgrade --to` is refused too.
- Every upgrade takes a restore point first (database copy, binary, unit file). A failed health check puts the old binary and, when the schema changed, the old database back automatically.
- Upgrading is supported from the previous minor's latest patch. Skipping minors is best effort.

Not yet proven, so not promised: systemd and agent reconnection after an upgrade, and an upgrade on a live production host.

## Open questions

- Which release becomes the first stable line, and what evidence is required first (see the graduation criteria in [Experimental features](/experimental-features#graduation-criteria-proposed)).
- Whether multi-node joins the covered surface at 1.0 or later.
- The exact warning mechanism for each surface.
