---
title: "Preview environments"
description: Run a copy of an app for each pull request, with its own URL, caps, optional database, and automatic teardown when the pull request closes.
"og:url": "https://docs.levelrail.com/preview-environments"
---

A preview environment is a running copy of an app for one pull request. Levelrail deploys it when the pull request opens or gets new commits, and tears it down when the pull request closes or merges.

Previews are off until you turn them on for an app. Each app opts in on its own.

## Turn it on

Connect the app to a git repository first (see [git integrations](/git-integrations)). Then:

```sh
levelrail-cli apps previews enable web
levelrail-cli apps previews list web
levelrail-cli apps previews disable web
```

`list` with no app name shows previews across every app you can read.

## Lifecycle

- **Opened or reopened** deploys a preview. A new push to the pull request redeploys it.
- **Closed or merged** tears it down, whether or not it merged.
- **Teardown** removes the preview app, its containers, its disposable databases, and its env overrides.
- **Stale sweep** tears down previews that have had no updates for their TTL. The default is 7 days. Set it per app with `--ttl-hours`, or for the whole control plane with the runtime setting `deploy.preview_ttl_hours` (`levelrail-cli settings runtime set deploy.preview_ttl_hours 72`, applies on the next sweep; 0 keeps the startup default) or `APP_PREVIEW_TTL` (a Go duration such as `168h`). A per-app value wins. The sweeper runs every hour by default, set by `APP_PREVIEW_SWEEP_INTERVAL`.
- A preview stuck in deploying longer than `APP_PREVIEW_STUCK_AFTER` (default 30 minutes) is marked failed by the startup sweep.

Manual controls:

```sh
levelrail-cli apps previews teardown web 42
levelrail-cli apps previews extend web 42 --hours 48
levelrail-cli apps previews sweep
```

`levelrail-cli apps previews open web 42` opens the preview's URL in your browser (`--print` prints it instead).

`teardown` removes one pull request's preview now. `extend` keeps one preview past its TTL. `sweep` removes every stale preview across all apps.

## Limits

Two caps bound how many previews run at once:

- `APP_PREVIEW_ENV_MAX_PER_APP` (default 10) caps live previews per app.
- `APP_PREVIEW_ENV_MAX_TOTAL` (default 50) caps live previews on the control plane.

A value of 0 turns that cap off. An app can set its own per-app cap with `--max-previews`, where 0 means the platform cap applies.

When a cap is full, the app's `on_limit` decides what happens:

```sh
levelrail-cli apps previews limits web --on-limit evict_oldest   # default: remove the oldest live preview
levelrail-cli apps previews limits web --on-limit reject         # refuse the new preview
levelrail-cli apps previews limits                               # platform-wide usage and caps
```

A preview that the cap rejects is recorded with the status `limit_reached` and the reason.

## Fork pull requests

By default a pull request from a fork is held. It does not deploy until someone approves it. Approve a held pull request once with:

```sh
levelrail-cli apps previews approve web 42 --yes
```

Add `--share-secrets` to give that one deploy the app's environment variables and secrets. Without it, a fork preview gets no app environment.

To skip the hold for forks, set `--allow-forks`. Forks then deploy without approval but still get no app environment. Only `--fork-secrets` gives fork previews the app's environment and secrets, so turn it on only if you accept that a fork's code runs with them:

```sh
levelrail-cli apps previews settings web --allow-forks
levelrail-cli apps previews settings web --fork-secrets
```

## Database strategy

Each preview can get a database. The default is `none`, which gives the preview no database.

```sh
levelrail-cli apps previews settings web --database fresh
levelrail-cli apps previews settings web --database seed --seed-database web-db
```

| Strategy | What the preview gets |
| --- | --- |
| `none` | No database. This is the default. |
| `shared` | The app's production database. Writes from the preview land in production, so use this only when that is what you want. |
| `fresh` | An empty database of the same engine and version, created for this preview and removed with it. |
| `seed` | A fresh database seeded from the latest backup of `--seed-database`, or of the app's own database when no name is given. Seeding stops after `APP_PREVIEW_SEED_TIMEOUT` (default 30 minutes). |

The strategy applies to single-service apps. A multi-service app, or an app whose `app.yaml` declares `databases:`, uses that file instead. Mark a database with `ephemeralInPreviews: true` there to give every preview its own copy.

## Resources, idle sleep, and visibility

- Each preview gets 256Mi of memory and 0.25 CPU by default. Change them with `--memory` and `--cpu`.
- A preview sleeps after 30 minutes without requests. Change it with `--idle-sleep-minutes`, or for the control plane with `APP_PREVIEW_IDLE_SLEEP_MINUTES`.
- Search engines do not index preview URLs by default. Add `--allow-indexing` to allow it.
- Put basic auth in front of every preview of an app with `--gate`, plus `--gate-username` and `--gate-password`. The password is write-only: it is stored, not shown again. The previews list shows a badge per preview with a domain: Basic auth on or Public, read from that preview's own `gated` flag. Against an older server without the flag, it falls back to the app setting (including Gate needs a password when the gate is on and no password is stored). The previews list API and the CLI table also carry a per-preview `gated` flag (GATED column), true when that preview's own domain currently sits behind basic auth or a sign-in gate. It is a flag only, never the credentials, and read-only users see it too.

```sh
levelrail-cli apps previews settings web --memory 512Mi --cpu 0.5 --idle-sleep-minutes 60
levelrail-cli apps previews settings web --gate --gate-username reviewer --gate-password "$PREVIEW_GATE_PASSWORD"
```

Run `levelrail-cli apps previews settings web` with no policy flag to see the current values.

## Pull request feedback

Previews can report back to GitHub: a commit status on each deploy, and one pull request comment that is updated in place. This is off by default and opts in per app:

```sh
levelrail-cli apps previews pr-status enable web
levelrail-cli apps previews pr-status disable web
```

The control plane can switch commit statuses and deployments off for every app with `APP_GIT_STATUS_ENABLED=false`.

## Environment overrides for one pull request

Set a value for one pull request only, for example to point one preview at a staging API. The override takes precedence over the app's environment for that pull request, and applies on its next deploy.

```sh
levelrail-cli apps pr-env set web FEATURE_FLAG --pr 42 --value on
levelrail-cli apps pr-env set web API_KEY --pr 42 --value sk-test-123 --secret
levelrail-cli apps pr-env list web --pr 42
levelrail-cli apps pr-env clear web <id> --pr 42
```

A secret is stored encrypted and its value is not returned by `list`. Use `--secret` for anything sensitive.

## Branch deploys

A branch deploy gives a branch a stable URL of its own. It is separate from pull request previews and rebuilds on every push to the branch.

```sh
levelrail-cli apps branch-deploys set web --mode patterns --branch staging --branch "feature/*"
levelrail-cli apps branch-deploys list web
levelrail-cli apps branch-deploys delete web staging
```

- `--mode` is `off`, `all`, or `patterns`. Only `all` and `patterns` give branches a deploy. In `patterns`, a pattern is an exact branch name or a glob where `*` matches any characters, including `/`. The production branch never gets one.
- A branch deploy is removed when the branch is deleted, or after 14 days with no push. `APP_BRANCH_DEPLOY_TTL` changes the idle time.
- `APP_BRANCH_DEPLOY_MAX_PER_APP` (default 10) caps live branch deploys per app. A push past the cap is ignored with a note.
- Branch-scoped env overrides are set with `levelrail-cli apps branch-env`.

## Migration step and prebuilt images

Two optional settings per app, both off by default. They are set through `PUT /api/v1/apps/{name}/preview-policy` (`migrate_command`, `migrate_timeout_seconds`, `image_tag_template`).

- **Migration step.** `migrate_command` (for example `npm run migrate`) runs once through `sh -c` inside the preview's container, after the database strategy has been applied and before the preview is marked active. The container carries the preview's environment. If the command exits non-zero or times out, the preview is marked failed and the masked output tail is the failure reason, shown in the preview list and the pull request comment. The timeout is `migrate_timeout_seconds`, else `APP_PREVIEW_MIGRATE_TIMEOUT` (default 10 minutes). The wait for the container to be running is `APP_PREVIEW_MIGRATE_WAIT` (default 2 minutes). It applies to single-service previews, not static or multi-service ones. With the `seed` strategy the restore finishes first (bounded by `APP_PREVIEW_SEED_TIMEOUT`), the command then runs in a container started after the database env was attached, and a seed that cannot run (no runner, no backup, low disk, restore error) fails the preview instead of migrating an empty database. If the migration or the seed fails, the preview's own database is removed and the live app and its database are not touched. With the `fresh` strategy the command waits until the new PostgreSQL database accepts connections (`pg_isready` inside its container, bounded by the migrate timeout) and the preview fails if it never does. `fresh` with another engine fails, since readiness cannot be checked. The `shared` strategy (it would migrate the production database) and `databases:` in `app.yaml` fail with a message saying so. The `seed` and `fresh` paths are built and tested with a fake runner, not proven against a live database. Set it from the CLI with `apps previews settings <app> --migrate-command ... --migrate-timeout ...`.
- **Prebuilt image.** `image_tag_template` deploys a registry image for the pull request with no build, for example `ghcr.io/org/app:pr-{number}`. Placeholders are `{number}`, `{sha}`, `{short_sha}` and `{branch}`, and at least one is required. Set it with `--image-tag-template`. Your CI must push that tag before the pull request event arrives, or the deploy fails. It applies to single-service previews.

## Static site previews

When an app's build type is static (single service), a preview publishes the pull request's head commit as its own static site named `<app>-pr-<number>`. The live site, its route and its release history are not touched. Closing the pull request, the stale sweep and `apps previews teardown` remove the preview's site row, route and files. The basic auth gate (`--gate`) is created and enforced on static previews the same way as on container previews. If the gate cannot be created (no username, no stored password, no master key), the preview is not published and is marked failed with the reason, rather than served publicly. A forward-auth gate on the preview's domain is not enforced on static routes, so the preview is refused with a clear error instead. Built and tested, not proven on a real host.

## Pull request comment commands

Comment commands are off until you turn them on per app, and `APP_PR_COMMENT_COMMANDS_DISABLED=true` switches them off for the whole control plane:

```sh
levelrail-cli apps set web --pr-comment-commands on
levelrail-cli apps set web --pr-comment-commands-forks on   # also accept them on fork pull requests
```

A command is a line at the start of a pull request comment, outside code fences, with nothing after the verb. Exactly one command per comment, otherwise nothing runs.

| Comment | Effect |
| --- | --- |
| `/levelrail status` | Read-only status reply. |
| `/levelrail redeploy` | Redeploys the app. |
| `/levelrail rollback` | Rolls back to the earlier deploy. |
| `/levelrail preview redeploy` | Redeploys this pull request's preview (previews must be enabled). |
| `/levelrail help` | Lists the commands. |

The commenter must have `write` or `admin` permission on the repository, checked against GitHub for each command and refused if the lookup fails. Comments from bots and GitHub Apps are ignored, each comment id runs once, and per-pull-request and per-user rate limits apply (`APP_PR_COMMENT_COMMANDS_PER_PR_PER_MINUTE`, `APP_PR_COMMENT_COMMANDS_PER_USER_PER_MINUTE`). The commands are GitHub only. There is no `/destroy` or `/deploy` command: teardown stays with closing the pull request or the CLI. Built and tested, not proven on a real host.

## Not yet

- **Migration step with `shared`, `app.yaml` databases or a non-PostgreSQL `fresh` database.** `migrate_command` works with the `none` and `seed` strategies, and with `fresh` on PostgreSQL. With the others the preview fails with a message saying so.
- **Preview databases for branch deploys.** Branch deploys are app-only. The branch deploy code path never creates, seeds or removes a database, so a branch deploy uses the app's own database env. Database strategies apply to pull request previews only.
- **Comment commands outside GitHub.** GitLab, Gitea and Bitbucket pull requests do not accept comment commands.

## Preview host templates

By default a preview is served on a generated host. To choose the pattern, set a host template and the domain it renders under:

```
levelrail-cli apps previews settings web --host-template 'pr-{number}.{domain}' --wildcard-domain example.com
```

This renders `pr-12.example.com` for pull request 12. `{branch}` (reduced to one DNS label) works too. The template must contain `{number}` or `{branch}`, and the domain must be one of the app's own domains or a subdomain of one, so a template can never put a preview on a host the app does not own. The domain is checked again at deploy time; if it no longer matches, the preview falls back to the default host. Point a wildcard DNS record at the server for the domain, or let automatic DNS handle each host. An empty template clears the setting.
