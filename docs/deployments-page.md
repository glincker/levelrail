---
description: The cross-app Deployments page shows every deploy in one live list, with filters kept in the URL, a details drawer, redeploy and rollback actions, and keyboard shortcuts.
---

# Deployments page

The Deployments page (`/deployments`) lists every deploy across every app you can read, newest first. It updates live, so a deploy that starts or finishes appears without a refresh.

## What it shows

- **Summary strip**: deploys in progress, failure rate over 24 hours, median duration, deploys that need attention (held, or running a different image than expected), and a 14 day sparkline.
- **Building now**: queued and building deploys with a live timer and step progress. It is hidden when nothing is running.
- **The list**: one row per deploy with the commit message, status and duration, environment, app, commit, branch, age and author. The live production release carries a highlighted Production pill. Redeploys and rollbacks show "Redeploy of ..." or "Rollback to ..." instead of a commit. Failed, held, queued and superseded rows show the reason under the message.

Use **Load More** to fetch the next page. Long lists are virtualized.

## Filters

Select **Add Filter** and pick Author, Environment, Status, App, Branch or Trigger. Each choice becomes a removable pill and is stored in the page URL, so you can share a filtered view. The search box matches commit message, commit sha and app name.

Author is applied to the rows already loaded, because the server does not filter by author.

## Details drawer

Selecting a row opens a drawer with status, timing, source, the image as a `repo@sha256` chip you can copy, domains, and a preview image when one exists. For a failed deploy it shows the failing step, the error and the last log lines, with a link to the full logs.

Actions in the drawer: Redeploy, Roll back to this, Compare, Promote and Cancel. Redeploy, roll back and cancel always ask for confirmation. An action that cannot run shows why. Rolling back changes only the image; environment variables and settings are not reapplied.

## Keyboard shortcuts

| Key | Action |
| --- | --- |
| `/` | Focus search |
| `f` | Add filter |
| `j` / `k` | Next or previous deploy (the drawer follows) |
| `Enter` | Open details |
| `Esc` | Close details |
| `r` | Redeploy (asks first) |
| `b` | Roll back to the selected deploy (asks first) |
| `c` | Cancel a running deploy (asks first) |

Shortcuts are ignored while you type in a field.
