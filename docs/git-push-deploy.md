---
title: "Deploy with git push"
description: Push a branch to a git remote on your own server and it builds and deploys that exact commit. A beta feature, off by default, with a per-app token.
"og:url": "https://docs.levelrail.com/git-push-deploy"
---

Add your server as a git remote, push, and the app builds and deploys the commit you pushed. There is no git provider, webhook or SSH key involved: the control plane speaks git over HTTPS on the address you already use for the dashboard.

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

On the **Source** tab the card shows the remote, a copy button, the deploy branch, the last push and the rotate and revoke buttons.

<Tip>
To keep the token out of `.git/config`, add the remote without it and let a git credential helper ask once. The user name is `git`, the password is the token.
</Tip>

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
