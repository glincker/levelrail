---
title: "Git reliability"
description: What the control plane caches, what it polls, what the connection and webhook statuses mean, the environment variables behind them, and how to recover when a push did not deploy.
"og:url": "https://docs.levelrail.com/git-reliability"
---

Git integrations fail quietly in a few well-known ways: a list that stops at an arbitrary page, a revoked grant that shows as an internal error, a webhook address no git host can reach, a push that never arrives. This page lists what the control plane does about each one. Setup lives in [Git integrations](/git-integrations).

## What is cached

| Data | Cached for | Refresh |
| --- | --- | --- |
| Repository list of each connection (one per GitHub installation, one per GitLab, Bitbucket or Gitea account) | `APP_GIT_REPO_CACHE_TTL`, then served stale for up to `APP_GIT_REPO_CACHE_STALE_MAX` while one background refresh runs | Refresh button in the picker, `levelrail-cli git-providers repos <provider> --refresh`, `?refresh=1` |
| Newest commits of a branch (deploy sheet) | `APP_GIT_COMMITS_CACHE_TTL` | `?refresh=1` |
| OAuth access tokens | Until a minute before they expire | Refreshed one caller at a time per connection |

Concurrent requests for one connection share a single provider call. Everything is kept per connection: the list cache key, the recorded health, the OAuth refresh lock and the rate limit notice all belong to one account, so a revoked grant or a rate limit on one account never hides or slows another. A cached list is dropped when its own connection is disconnected or reauthorized, or a GitHub installation is removed. If the provider rate limits a refresh, the saved list is still served and says when it will be checked again.

## What polls

Every enabled git source whose trigger mode is `push` is checked for a missed push. Release mode sources are not polled: they deploy on tags, which a branch head comparison cannot see.

Each check reads the remote branch head (a git `ls-remote`, no provider API budget) and compares it with the newest head a webhook or an earlier poll saw. A head nobody has deployed, queued or seen is queued like a webhook push, recorded as `Poll, webhook not received` in the deploy list, and run by the deploy queue. The poller never builds anything itself.

- A webhook and the poller never both queue one commit: both check and enqueue under one lock, deduped by commit SHA.
- The first head ever seen for a source is only remembered. It cannot be told from a rollback you did on purpose, so it is not deployed.
- A head that is older than what is already deployed or queued is ignored.
- A failing or rate limited host is backed off with a doubling delay up to `APP_GIT_POLL_MAX_BACKOFF`, and a provider connection marked rate limited is skipped until its next check time.
- Cancelling the control plane's context stops the poller cleanly.

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_GIT_POLL_ENABLED` | `true` | `false` turns polling off everywhere. |
| `APP_GIT_POLL_INTERVAL` | `5m` | Time between checks of one source. |
| `APP_GIT_POLL_JITTER` | `0.2` | Fraction of the interval added or removed at random, so sources do not poll together. |
| `APP_GIT_POLL_CONCURRENCY` | `4` | Sources checked at once. |
| `APP_GIT_POLL_MAX_BACKOFF` | `1h` | Longest wait after repeated failures. |

## Older deliveries are not redeployed

Webhook deliveries can arrive out of order. A push is ignored, with the reason in the delivery history, when it is provably older than a commit already deployed or queued: its commit is the parent of that commit, or the host's commit timestamp is earlier. When order cannot be established (a force push with no timestamps, for example) the push is processed and the deploy-time ordering guard still applies.

## Statuses

Connection health (`connection.status` of each account in `GET /api/v1/git-connections`, the worst of a provider's accounts in `GET /api/v1/git-providers`, the settings page, the System Doctor check `git_connections`). A git source whose account was disconnected shows `needs_reconnect`:

| Status | Meaning | Recovery |
| --- | --- | --- |
| `ok` | The provider accepted the last call. | None. |
| `needs_reauthorize` | The grant was revoked or expired, or the provider answered 401 or 403. | Reconnect on that account's row on the provider's settings page. |
| `revoked` | The GitHub App installation or authorization no longer exists. | Reconnect, which opens the App page to install again. |
| `rate_limited` | The provider is rate limiting. | Automatic; shows `next_check_at`. |

Webhook registration (`webhook_registration.state` on a git source): `registered`, or `failed` with the provider's error. A failed registration shows a Register again button; the CLI is `levelrail-cli apps git-source register-webhook <app>`.

Webhook address (`webhook_url_warning`): `private_address` means the address is localhost, a private or link-local range, or a local-only name; `no_public_url` means none is known. The control plane builds the address from the primary domain, then the dashboard URL, and only then from the address of the request being served. When it is not reachable the source shows a warning with a Fix action that saves a public URL. To allow a private address on purpose, for example for a LAN-only test, set `APP_GIT_WEBHOOK_ALLOW_PRIVATE=true`.

## Environment variables

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_GIT_REPO_CACHE_TTL` | `5m` | How long a repository list is served without any provider call. |
| `APP_GIT_REPO_CACHE_STALE_MAX` | `24h` | How long an expired list may still be served. |
| `APP_GIT_REPO_REFRESH_MIN` | `10s` | Shortest gap between manual refreshes of one connection. |
| `APP_GIT_REPO_LIST_MAX` | `10000` | Repositories listed per connection before the list is marked truncated. |
| `APP_GIT_RATE_LIMIT_RETRIES` | `2` | Retries of a rate limited provider call within one request. |
| `APP_GIT_RATE_LIMIT_MAX_WAIT` | `5s` | Longest `Retry-After` slept on; longer waits return a next check time instead. |
| `APP_GIT_RATE_LIMIT_BACKOFF` | `60s` | Assumed wait when the provider sends no `Retry-After`. |
| `APP_GIT_WEBHOOK_ALLOW_PRIVATE` | off | Do not warn about a private webhook address. |
| `APP_GIT_POLL_*` | see above | The poller. |

## Recovering

1. A push did not deploy: open the app's Source tab. Fix the webhook address warning, press Register again if registration failed, and check the delivery history under Webhook deliveries.
2. Nothing arrives at all: the poller deploys the new head within one interval, or deploy the commit by hand from the deploy sheet.
3. A provider shows Reconnect: press it and authorize again. Cached lists are dropped and refilled.
4. A repository list looks short or stale: press Refresh. A list marked truncated is capped at `APP_GIT_REPO_LIST_MAX`; paste a repository URL for one beyond it.
