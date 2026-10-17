---
title: "Deploying through git host outages"
description: What still deploys when GitHub, GitLab or your own git server is down, how the source cache works, and how to deploy from it.
---

A deploy should not stop because someone else's server is down. The control plane keeps a bounded local mirror of every repository it fetches, so most deploys never need the git host again.

## What needs the git host

| Action | Needs the git host? |
| --- | --- |
| Roll back to an earlier release | No. Rollback redeploys a pinned image. See [Rollback pins](/deploy-safety#rollback-pins). |
| Redeploy an image, change env, domains or resources | No. |
| Redeploy a commit that was deployed before | No. A full commit hash found in the cache is used without any network call. |
| Rebuild a known commit after a base image changed | No, same reason. |
| Deploy a new commit or a branch tip | Yes. A branch is refreshed first so it is never served stale. |

## The source cache

After every successful fetch the control plane keeps a bare mirror under `<data dir>/source-cache`. Each deploy checks its tree out of that mirror into a throwaway directory, so the mirror itself is never modified by a build.

The mirror is bounded and pruned in a fixed order: anything unused for longer than the age cap first, then the least recently used until the total fits the size cap. A mirror with a build in progress is never removed.

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_SOURCE_CACHE_MAX_BYTES` | `5368709120` (5 GiB) | Total size cap for all mirrors. |
| `APP_SOURCE_CACHE_MAX_AGE` | `720h` | Mirrors unused for longer than this are removed. |
| `APP_GIT_FETCH_TIMEOUT` | `10m` | How long one fetch attempt may take. |
| `APP_GIT_FETCH_RETRIES` | `2` | Retries after the first attempt, only for network and server errors. |
| `APP_GIT_FETCH_BACKOFF` | `2s` | First wait between retries, doubled each time. |
| `APP_GIT_RECOVERY_INTERVAL` | `30s` | How often a failing host is probed for recovery. |
| `APP_GIT_REPLAY_WINDOW` | `24h` | How old a push may be and still be replayed after recovery. |

Authentication and not-found errors are never retried: a retry cannot fix them.

## When the host is down

A deploy that needs a commit the cache does not hold fails fast, after the bounded retries, with:

```text
git host unreachable, commit not cached
```

The failure carries the code `git_host_unreachable` and a next step (see [Deploy failures](/deploy-failures#git_host_unreachable)). The dashboard shows a **Git host unreachable** item in Attention for as long as the host fails, naming the apps whose pushes are waiting.

### Deploy from cache

Rebuild the last known good commit without contacting the host:

```bash
levelrail-cli apps deploy web --from-cache
levelrail-cli apps deploy web --from-cache --commit 1a2b3c4
```

Without `--commit` it uses the commit of the app's last successful source deploy. The same action appears as **Deploy from cache** on a failed deploy in the deploy history when the cause is the git host. The API is `POST /api/v1/apps/{name}/deploys/from-cache`. It goes through the normal queue, freeze windows and approvals. If the commit is not in the cache it fails with `source_not_cached`.

### Pushes while the host is down

The webhook receiver keeps accepting pushes. The delivery is stored in the app's webhook delivery history like any other, and the failed attempt records why. When the host answers a probe again, the newest delivery of each waiting app is replayed once, as long as it is still inside the replay window and no newer delivery has arrived since. You can also replay any delivery by hand from the delivery history.

Replay after recovery is kept in memory. If the control plane restarts during an outage, replay the delivery from the history instead.

## Limits

- A checkout from the cache has a real `.git` at the requested commit with full history and tags, but no remote and no branches.
- Single repository mode (`APP_GIT_REPO_URL`) uses the same cache and fast failure, but does not replay pushes automatically.
- Submodule contents are not fetched into the cache.
