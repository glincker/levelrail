---
title: "Deploy with git push"
description: Push a branch to a git remote on your own server and it builds and deploys that exact commit. A beta feature, off by default, with a per-app token.
"og:url": "https://docs.levelrail.com/git-push-deploy"
---

If you are used to deploying with a plain `git push`, this is that flow: add your server as a git remote, push, and the app builds and deploys the commit you pushed. There is no git provider, webhook or SSH key involved: the control plane speaks git over HTTPS on the address you already use for the dashboard.

<Warning>
Beta. It is off by default, it only accepts pushes, and it has not run in production for long. Read [Limits](#limits) before relying on it.
</Warning>

## Turn it on

1. Open **Settings, Beta features** and switch on **Git push deploy** (or add `git-push` to `APP_EXPERIMENTAL`). It applies at once, no restart.
2. Create the app first. An empty app made with `levelrail-cli apps create` or the dashboard is enough.
3. Enable the remote for that app, from the CLI or the app's **Source** tab.

```bash
levelrail-cli git remote add my-app
```

It prints the exact line to run in your repository. The token is part of that line and is shown once.

```bash
git remote add levelrail https://git:gp_...@cp.example.com/api/v1/git/my-app.git
git push levelrail main
```

Add `--apply` (to `git remote add` or `git remote rotate`) to let the CLI set the remote in the current repository, or in `--dir PATH`, for you. It runs `git remote add`, or `git remote set-url` when a remote with that name already points at this app's address. If the name is taken by a remote that points somewhere else, it stops and changes nothing. By default the token is kept out of `.git/config` (see the tip below). Pass `--store-token-in-config` to write the token URL into `.git/config` instead; it is then stored unencrypted in the repository config and anyone who can read that repository can read it. If git or the repository is missing, the command prints the line above so the one-time token is not lost. `--apply` is unit tested with fakes (`cmd/levelrail-cli/git_remote_test.go`) and has not run against a live control plane.

On the **Source** tab the card shows the remote, a copy button, the deploy branch, the last push and the rotate and revoke buttons.

<Tip>
To keep the token out of `.git/config`, add the remote without it and let a git credential helper ask once. The user name is `git`, the password is the token.
</Tip>

With `--apply` the token stays out of `.git/config` by default (`--credential-helper` is still accepted and does nothing extra). The remote URL is written with the user name only, and a fixed git credential helper for this server reads the token from the `LEVELRAIL_GIT_TOKEN` environment variable at push time. The one-time token is printed once so you can export it, and the command refuses a server URL whose host differs from the API address. `rotate --apply` also replaces any remote URL an older version wrote with the token inline. Without `--apply` nothing changes: the command prints the add line. This is unit tested with fakes and has not pushed to a live git server.

## What a push does

A push to the deploy branch (`main` by default) is received, stored, built and deployed. The build output comes back to your terminal as `remote:` lines, so you see what the dashboard sees.

```text
remote: Received commit 3f2a9c1 on main, starting the build.
remote: ...build output...
remote: Deploy succeeded: 3f2a9c1 is rolling out.
```

- The image is tagged by the pushed commit, and the deploy shows in the app's deploy history with the commit, message and author. Rollback works as for any other deploy.
- If the build fails, git shows `[remote rejected] ... deploy failed`, the previous release keeps running, and the branch on the server does not move. After fixing the build, run the same `git push` again.
- A Dockerfile at the repository root is built as a Dockerfile, anything else with Railpack. If the app has a connected [git source](/git-integrations) its build settings are used instead.
- With no `app.yaml`, a first push of a repo that has a `Procfile` deploys each process type as a service. An `app.json` next to it supplies healthchecks, `formation` replica counts and `scripts.dokku` pre and post deploy hooks. Any other `app.json` key is reported as a warning in the deploy result, never dropped silently. An app that already deployed keeps its single service. To run several process types by hand, describe them as services in `app.yaml`; `levelrail-cli import config` turns a `Procfile` into a first draft. See [Buildpack apps](/importing-apps#buildpack-apps).
- One deploy per app at a time. A push during a running deploy or a deploy freeze is refused with a message.

Other branches, tags, deletes and pushes of several branches at once are rejected with a message that names the deploy branch. Change it with:

```bash
levelrail-cli git remote branch my-app release
```

## Tokens

- One token per app, shown once, stored only as a SHA-256 hash.
- The token can do one thing: push to that app. It cannot read the app, list anything or call the API.
- `levelrail-cli git remote rotate my-app` issues a new token and the old one stops working at once. `revoke` turns the remote off.
- Wrong or missing tokens always answer the same way, requests are rate limited, and repeated bad tokens back off. Every push outcome is written to the audit log.
- Creating, rotating, changing the branch and revoking need the `write_sensitive` ability on the app.

## GitHub App access for stored sources

Git sources set by a team scoped user no longer clone with the connected GitHub App (`app_auth` is false). Multi-team admins should review stored git sources after upgrading: existing sources keep App access.

## Submodules and LFS

Builds skip git submodules and LFS objects unless you turn them on per app. The toggles apply to push deploys only; preview and branch deploys (including pull requests from forks) never fetch submodules or LFS objects.

```sh
levelrail apps git-source settings my-app --submodules --lfs
```

The same toggles are on the app's Source tab and in `PUT /api/v1/apps/{name}/git-source/deploy-settings` (`fetch_submodules`, `fetch_lfs`). Changing them needs the same access as other git source settings.

- Submodule URLs must be `https://`, or a `./` or `../` path relative to the app's repository that stays on its host. `ssh`, `git://`, `file://`, and URLs with a user name or password are refused, and the build fails with the module name.
- A submodule path must stay inside the checkout: absolute paths, `..`, `.git` and symlinked directories are refused.
- The app's git credentials go only to the repository's own host. A submodule on another host is fetched anonymously unless an admin lists that host in `git.submodules_credential_hosts`. A private submodule without access fails with a clear `submodule needs credentials` error that names the host.
- LFS objects download only from the repository's host. If your git server redirects downloads to a storage host, list it in `git.lfs_hosts`; the app's credentials are never sent there. Each object is checked against its pointer's size and hash.
- Submodule and LFS hosts must resolve to public addresses. Loopback, private, link-local, CGNAT, multicast and unspecified addresses are refused at connect time; an admin can allow a self-hosted git server's network in `git.submodules_allowed_cidrs`. Any network failure is reported as `submodule host unreachable` with the host name; the cause is in the server log.
- Runtime settings cap each build: `git.submodules_max_depth` (nesting), `git.submodules_max_count`, `git.submodules_max_mb` (checkout plus submodules plus LFS) and `git.submodules_timeout`. They apply to the next build.

## Limits

| Limit | Default | Setting |
| --- | --- | --- |
| Size of one push | 256 MiB | `APP_GIT_PUSH_MAX_BYTES` |
| Stored copy per app | 1 GiB, then it is reset and the push asks to retry | `APP_GIT_PUSH_QUOTA_BYTES` |
| Wait for the deploy | 30 minutes (the build keeps going after) | `APP_GIT_PUSH_TIMEOUT` |
| Requests per minute, per address and app | 30 | `APP_GIT_PUSH_RATE_PER_MINUTE` |

What it does not do:

- It is not a git host. There is no repository browser, history view or pull request, and `git clone` and `git fetch` are refused.
- No Git LFS, no SSH, no Compose apps, no per-branch environments.
- Anyone with the token can deploy that app. Treat it like a deploy key, rotate it when someone leaves.
- It needs the source cache directory, which every normal install has. Without it the endpoints answer 501.
- The control plane must be reachable over HTTPS from where you push. Over plain HTTP the token travels in clear text, so use the HTTPS dashboard address.

## Troubleshooting

| You see | Meaning |
| --- | --- |
| `Authentication failed` | Wrong, rotated or revoked token, or the token belongs to another app. |
| `branch not allowed, this app deploys main` | You pushed another branch. Push `main` or change the deploy branch. |
| `deploy failed, the previous release is still running` | The build failed. The lines above show why. Fix it and push again. |
| `HTTP 413` | The push is larger than the limit. Remove large files or raise `APP_GIT_PUSH_MAX_BYTES`. |
| `HTTP 404` | Git push deploy is switched off in Beta features. |
| `HTTP 429` | Too many requests or bad tokens. Wait for the time in `Retry-After`. |
| `another push for this app is in progress` | Wait for the running push to finish. |

The API is `GET`, `POST`, `PUT` and `DELETE` on `/api/v1/apps/{name}/git-remote`, listed in the [API reference](/api-reference). The design and the alternatives that were rejected are in ADR 040.
