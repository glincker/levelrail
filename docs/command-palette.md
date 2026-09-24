---
description: Keyboard-first command palette for navigating the dashboard and running quick app actions.
---

# Command palette

Press `Ctrl+K` (or `Cmd+K` on macOS) anywhere in the dashboard, or use the search button in the header.

## Groups

- **Recent**: the last five items you picked, shown when the search box is empty. Stored only in your browser (`localStorage`); if storage is blocked the palette still works, it just forgets.
- **Actions**: Go to Status, Go to Apps, Go to Nodes, Create app, Browse templates (both open the Apps page, where the New app button lives), and Toggle theme (cycles light, dark, system).
- **App actions**: type at least two characters of an app name to get Restart, Redeploy, Open logs for and Open deploys for that app (up to three matching apps). Restart and Redeploy show the same toasts as the app list menu.
- **Navigate**, **Settings**, **Apps**, **Databases**: jump to any page, settings section, app or database.

Matching is fuzzy: `rw` finds "Restart web". The app and database lists come from the dashboard's cached queries, so typing never triggers a fetch.

## Keys

| Key | Does |
| --- | --- |
| `Up` / `Down` | Move the selection |
| `Enter` | Run the selected item |
| `Esc` | Close |

Focus stays inside the palette while it is open and returns to the page on close.

These actions call the existing API (`POST /api/v1/apps/{name}/restart` and `POST /api/v1/apps/{name}/deploys`); the CLI equivalents are `levelrail apps restart` and `levelrail apps deploy`.

## Keyboard shortcuts

Press `?` anywhere in the dashboard (or pick **Keyboard shortcuts** in the palette) to see the full list.

| Key | Does |
| --- | --- |
| `Ctrl+K` / `Cmd+K` | Open the command palette |
| `?` | Show the shortcuts dialog |
| `/` | Focus the current page's search or filter field, if it has one |
| `Esc` | Close a dialog, or leave the search field |
| `g` then `a` | Go to Apps |
| `g` then `n` | Go to Nodes |
| `g` then `s` | Go to Status |
| `g` then `d` | Go to Domains |
| `g` then `b` | Go to Backups |
| `g` then `t` | Go to Settings |

The second key of a `g` chord must follow within 1.5 seconds. Shortcuts are ignored while you type in an input, textarea, select or editable field, while Ctrl, Cmd or Alt is held, and while any dialog is open. They are browser-side only, so there is no API or CLI equivalent.
