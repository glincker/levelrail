---
description: The dashboard's "What's new" panel shows recent release notes after an upgrade, sourced from the control plane's own CHANGELOG.md.
---

# What's new panel

A sparkle icon in the dashboard header, next to the notifications bell, opens a short list of recent releases: version, date, and the bullet points from that release.

## Where the content comes from

There is exactly one source of truth: the repository's root `CHANGELOG.md`, maintained automatically by release-please on every release. The control plane parses it once at startup (`internal/changelog`) and serves the most recent entries from `GET /api/v1/changelog`. Nothing is hand-curated a second time for this panel.

A control plane running from the official Docker image always has this file available. A bare binary install that hasn't shipped `CHANGELOG.md` next to the binary yet (see `internal/changelog.ReadFile`'s default, `./CHANGELOG.md`, overridable with `APP_CHANGELOG_FILE`) just shows an empty panel rather than failing.

## Unread indicator

The badge counts how many releases are newer than the last one you opened the panel at. That "last seen version" is stored only in your browser (`localStorage`); opening the panel marks everything as read. It is a per-browser convenience, not account state, so a different browser or a cleared storage shows every entry as unread again.

## CLI

`levelrail-cli changelog [--limit N]` prints the same entries from the terminal, as JSON with `--json` or human-readable otherwise.
