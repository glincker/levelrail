---
description: The dashboard's "What's new" panel shows recent release notes after an upgrade, sourced from the control plane's own CHANGELOG.md.
---

# What's new panel

A sparkle icon in the dashboard header, next to the notifications bell, opens a short list of recent releases: version, date, and the bullet points from that release.

## Where the content comes from

The only source is the repository's root `CHANGELOG.md`, maintained automatically by release-please on every release. The control plane parses it once at startup and serves the most recent entries from `GET /api/v1/changelog`. Nothing is written a second time for this panel.

The official Docker image always includes the file. A bare binary install without a `CHANGELOG.md` next to it shows an empty panel instead of failing. Point the control plane at a different file with `APP_CHANGELOG_FILE`.

## Unread indicator

The badge counts how many releases are newer than the last one you saw when you opened the panel. That last-seen version is stored only in your browser (`localStorage`) and opening the panel marks everything as read. A different browser or cleared storage shows every entry as unread again.

## CLI

`levelrail-cli changelog [--limit N]` prints the same entries in the terminal, as JSON with `--json` or human-readable otherwise.
