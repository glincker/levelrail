---
description: Per pull request preview environments on your own servers, with their own domain and DNS record, safe defaults for data and secrets, a TTL and cap, idle sleep, and a PR comment that links back.
---

# Preview environments per pull request

Open a pull request and Levelrail builds its head commit, deploys it as `<app>-pr-<n>` on its own domain, and posts one comment on the pull request with the URL. Push again and the same preview updates. Close or merge the pull request and everything the preview created is removed, including its DNS record.

This page covers the lifecycle and the safety defaults. For the git hosts that support previews and the status reporting each one gets, see [Preview environments](preview-environments.md) and [Git integrations](git-integrations.md).

## Turn it on

Connect a git source to the app, then enable previews on the app's Previews page (or `levelrail-cli apps previews enable <app>`). Turn on `pr-status` if you want the pull request comment and commit status. Previews are off until you do both.

## What a preview gets

| Property | Default |
| --- | --- |
| Name | `<app>-pr-<n>` |
| Domain | `<app>-pr-<n>.<apps base domain>` when an apps base domain is set, otherwise `pr-<n>.<app>.<primary domain>`, otherwise host and port only |
| DNS | A record is created when automatic DNS is on, and removed on teardown. A wildcard record for the base domain already covers the one-label host, so nothing is written then |
| Resources | `256Mi` memory and `0.25` CPU, never more than the app itself has |
| Idle | Sleeps after 30 idle minutes and wakes on the next request |
| Search engines | Hidden: `X-Robots-Tag` and a deny-all `robots.txt` |
| Database | None: no database credentials reach the preview |
| Fork pull requests | Wait for approval and get no environment variables |

Platform defaults come from `APP_PREVIEW_MEMORY`, `APP_PREVIEW_CPU` and `APP_PREVIEW_IDLE_SLEEP_MINUTES` (0 turns idle sleep off). Every one of them can be overridden per app.

## Environment variables per preview

A preview inherits the app's plain environment, minus database connection variables (see below). Use [preview-only overrides and branch overrides](environments.md) to give a preview its own values: they are applied last, so they always win.

## Database strategy

| Strategy | What the preview talks to |
| --- | --- |
| `none` (default) | Nothing. Variables that look like database connections (`DATABASE_URL`, `*_DB_*`, `REDIS_URL`, `POSTGRES_*` and similar) and the app's attachment variable are dropped |
| `fresh` | An empty database of the same engine and version as the app's attached database, created for the preview and deleted with it |
| `seed` | A new database restored from the latest succeeded backup of the named seed database, in the background. Without a backup or a restore runner the preview keeps an empty database and says so |
| `shared` | The production database. Opt in only for read-mostly apps you trust: every preview can read and change production data |

Databases declared in `app.yaml` with `ephemeralInPreviews` or `isolatedInPreviews` keep working as before and take precedence for multi-service previews.

The name matching for `none` is a heuristic. If a credential has an unusual name, set it to an empty value with a preview-only override.

## Lifecycle

- **Cap.** Each app keeps at most the platform cap of live previews (`APP_PREVIEW_ENV_MAX_PER_APP`), or its own number under Previews. At the cap the oldest preview is evicted, or the new pull request is skipped, depending on the app's policy.
- **TTL.** A preview with no new pushes for the TTL (default 7 days, `APP_PREVIEW_TTL`, per app in hours) is removed even if the close event never arrived. **Extend** adds time to one preview from the Previews page or `levelrail-cli previews extend <app> <pr> --hours 48`.
- **Cleanup.** Teardown removes the preview's services, its databases, and the DNS record Levelrail created. If any step fails the row stays, marked failed with the reason, and the next teardown or sweep retries only what is left. Images and volumes are reclaimed by the normal image and orphaned volume cleanup, since the preview app no longer exists.
- **State.** Each preview has a status (`deploying`, `active`, `failed`, `awaiting_approval`, `limit_reached`) and a reason, shown in the list and the pull request comment.

## Feedback on the pull request

One comment per pull request is created and then edited in place: building, ready (with the URL, the commit, a link to the build logs and when the preview is cleaned up), failed (with the reason), removed. A failing build also marks the commit status failed.

## Forks

A pull request from a fork never deploys on its own, and never receives the app's environment variables, secrets or overrides. Three controls, in order of risk:

1. Default: the pull request waits. A maintainer reviews it and approves a one-time deploy from the Previews page or `levelrail-cli apps previews approve <app> <pr> --yes`. The preview runs without the app's environment.
2. `--share-secrets` on approve (or the checkbox in the dialog) gives that one deploy the app's environment, secrets and overrides.
3. `allow-forks` deploys fork pull requests without approval, still without environment. `fork-secrets` is what lets them have it: leave it off unless every contributor is trusted.

A new push from the fork needs a new approval.

## Basic auth gate

Turn on the gate to put one username and password in front of every preview of an app. The password is stored encrypted with the app's other secrets and copied to each preview domain when it is deployed. It needs the control plane master key.

## CLI

```
levelrail-cli previews list [app] [flags]
levelrail-cli previews extend <app> <pr> [--hours 24] [flags]
levelrail-cli previews delete <app> <pr> [flags]
levelrail-cli previews settings <app> [flags]
levelrail-cli previews approve <app> <pr> --yes [--share-secrets]
```

`previews` is the same command as `apps previews`. `settings` shows the policy, or changes it when you give flags: `--max-previews`, `--ttl-hours`, `--memory`, `--cpu`, `--idle-sleep-minutes`, `--database none|shared|fresh|seed`, `--seed-database`, `--allow-forks`, `--fork-secrets`, `--gate --gate-username --gate-password`, `--allow-indexing`.

## API

| Route | Does |
| --- | --- |
| `GET /api/v1/apps/{name}/previews` | List an app's previews |
| `POST /api/v1/apps/{name}/previews/{number}/extend` | Add hours to one preview's expiry |
| `POST /api/v1/apps/{name}/previews/{number}/teardown` | Remove one preview now |
| `POST /api/v1/apps/{name}/previews/{number}/approve` | Deploy a held fork pull request once (`share_secrets` optional) |
| `GET`, `PUT /api/v1/apps/{name}/preview-policy` | Read or change the policy |

## What this does not do

Preview databases are not migrated or reset between pushes, a `seed` restore is not waited on before the app starts, and previews run on the same node as the control plane's other workloads unless the app is placed elsewhere.
