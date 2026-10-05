---
description: Keyboard-first command palette and shortcuts for navigating the dashboard and running quick app actions.
---

# Command palette

Press `Ctrl+K` (`Cmd+K` on macOS) anywhere in the dashboard, or use the search hint in the header. Type to filter. Matching is fuzzy, so `rw` finds "Restart web".

## What you can find

- **Suggested** (empty search box): the app you are looking at, apps that are failing, and a few common actions such as creating an app.
- **Recent**: the last five items you picked, kept only in your browser (`localStorage`). If storage is blocked the palette still works and just forgets.
- **Actions**: Go to Status, Apps, Alerts or Nodes, Create app, Browse templates, Import, Filter failing apps, Keyboard shortcuts, and Toggle theme (cycles light, dark, system).
- **App actions**: type at least two characters of an app name to get Restart, Redeploy, Open logs for and Open deploys for that app (up to three matching apps). Restart and Redeploy show the same toasts as the app list menu.
- **Navigate**, **Settings**, **Apps**, **Databases**, **Nodes**, **Templates**, **Domains**: jump straight to a page, a settings section, or a specific resource. Pages for [experimental features](experimental-features.md) appear only when the feature is on.

The lists come from the dashboard's cached queries, so typing never triggers a fetch.

| Key | Does |
| --- | --- |
| `Up` / `Down` | Move the selection |
| `Enter` | Run the selected item |
| `Esc` | Close |

Focus stays inside the palette while it is open and returns to the page on close. Restart and Redeploy call `POST /api/v1/apps/{name}/restart` and `POST /api/v1/apps/{name}/deploys`; the CLI equivalents are `levelrail-cli apps restart` and `levelrail-cli apps deploy`.

## Keyboard shortcuts

| Key | Does |
| --- | --- |
| `Ctrl+K` / `Cmd+K` | Open the command palette |
| Hold `l` | Open the quick navigation overlay: a tile for each destination below |
| `/` | Focus the current page's search or filter field, if it has one |
| `Esc` | Close a dialog, or leave the search field |
| `g` then `a` | Go to Apps |
| `g` then `n` | Go to Nodes |
| `g` then `s` | Go to Status |
| `g` then `d` | Go to Domains |
| `g` then `b` | Go to Backups |
| `g` then `p` | Go to Pipelines |
| `g` then `l` | Go to Load balancers (experimental, see below) |
| `g` then `m` | Go to AI models (experimental, see below) |
| `g` then `t` | Go to Settings |

The second key of a `g` chord must follow within 1.5 seconds. Shortcuts are ignored while you type in an input, textarea, select or editable field, while Ctrl, Cmd or Alt is held, and while any dialog is open. They run in the browser only, so there is no API or CLI equivalent. The load balancer and AI models shortcuts need their [experimental feature](experimental-features.md) switched on.
