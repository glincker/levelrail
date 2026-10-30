---
title: "Promotions and release notes"
description: Promote the same image from one environment to another without a rebuild, see what a promotion or a deploy changes (commits, env var keys, image digest), and generate release notes from the commits between two deploys.
"og:url": "https://docs.levelrail.com/promotions-and-releases"
---

A promotion moves a known-good image from one app to a sibling app in another environment of the same project. It reuses the image the source app is running, so nothing is rebuilt and the content you tested is the content you ship.

## Promote an image

Preview first. The preview shows the image on each side (with its digest when one is known), the config differences, the env var keys that would be added, removed or changed (never values), and anything that blocks the promotion:

```bash
levelrail-cli apps promote web-staging --to env_production --preview
levelrail-cli apps promote web-staging --to env_production --target web-prod
```

- Add `--include-env` to also apply the added and removed plain env keys. Secret values, domains, ports, volumes and node placement always stay the target's own.
- A promotion into a protected environment needs `--confirm` and becomes a pending approval that a different person approves. See [Environments](/environments).
- A source that is unhealthy or whose last deploy failed is refused unless you pass `--force`.
- In the dashboard, use Promote on the source app's Deploys tab.

## See what a deploy changed

Compare two deploys, or one deploy against what is live now:

```bash
levelrail-cli apps deploys compare web --from dep_abc123 --to dep_def456
```

Besides the config fields, the comparison lists:

- **Image digest** on each side, and a changed digest as its own line.
- **Env var keys** added, removed or changed. Secret and database-backed values are never shown.
- **Commits** between the two deploys, newest first, read from the app's connected git source. When the range cannot be listed (no git source, or a commit older than the provider's recent window), the response says why instead of guessing.

The same view is in the dashboard under an app's deploy history, using Compare.

## Release bindings (APP_RELEASE_BINDINGS)

A release is an immutable snapshot of a succeeded deploy: its image pinned by digest plus the environment neutral config (health, replicas, strategy, resources, labels). Never env values, domains, ports or volumes. Each app has a binding that names the release it runs, with a history of every move. The server reads `APP_RELEASE_BINDINGS` once at start.

| Mode | What happens |
| --- | --- |
| `off` (default) | Nothing is recorded. Promote copies the image tag and rollback redeploys a snapshot, as before. `releases list` is empty. |
| `shadow` | Releases and bindings are recorded next to each succeeded deploy that has a digest. No deploy decision reads them, so it is safe to leave on and compare. |
| `on` | Promote and rollback go through the binding. Promote ships the source's release by digest, an approval stores the release id so a tag that moved cannot change what is approved, and `--release ID` works on `apps promote` and `apps rollback`. |

In `on` mode a failed apply puts the binding back where it was, and only while no newer move happened. Every move writes a `release_binding.retargeted` audit entry. An app with no digest recorded keeps the tag path.

Digest checks run on approval. If the registry needs credentials the server does not hold, or answers 401, 403 or a masked "not found", the check is inconclusive and the approval goes ahead. Only a definite "manifest unknown" blocks it. `APP_APPROVE_DIGEST_CHECK_TIMEOUT` bounds the lookup. Moving back from `on` to `shadow` or `off` is safe: stored releases stay, and pending approvals that carry a release id still apply that exact image.

## Generate release notes

Release notes group the commits between two deploys by conventional-commit type: breaking changes first, then features, fixes, performance, refactors, docs, tests and other changes.

```bash
levelrail-cli apps deploys release-notes web --from dep_abc123 --to dep_def456
levelrail-cli apps deploys release-notes web --from dep_abc123 --json
```

Leave `--to` out to end at the newest deploy that recorded a commit. The output is markdown you can paste into a release or a status update:

```
## web: 3f2a9c1 to 8b7d0e4

### Features

- **api**: add promotion preview (8b7d0e4)

### Fixes

- handle empty env on promote (5c1e2aa)
```

In the dashboard, open a comparison and use Generate release notes, then Copy markdown. The HTTP route is `GET /api/v1/apps/{name}/deploys/release-notes?from=ID&to=ID`.

## Push release notes to notifications

Notes are also pushed, not only pulled. When a deploy succeeds in a production environment, or a promotion completes, the same generator output is attached to the deploy-succeeded notification. It is on by default for those deploys and has no per-app switch.

- Generic webhook channels get an optional `release_notes` field (see [Event webhooks](/event-webhooks)).
- Slack, Mattermost, Rocket.Chat, Webex, Google Chat, Teams, email and Resend get the full markdown after the summary line.
- Discord, Telegram, ntfy, Gotify and Lark get the first 3 lines only. Paging and push channels stay unchanged.
- `RELEASE_NOTES_MAX_BYTES` (default 4000) caps the text, `RELEASE_NOTES_PREVIEW_BYTES` (default 300) caps the short-channel excerpt, `RELEASE_NOTES_TIMEOUT_SECONDS` (default 5) bounds generation.
- If generation fails or the range cannot be listed, the notification goes out without notes. A promotion only gets notes when the target app has deploys that recorded a git commit.

## Example flow

1. Deploy to staging and let it settle.
2. `apps promote web-staging --to env_production --preview` and read the env key changes.
3. `apps promote web-staging --to env_production --confirm`, then approve it as a second person.
4. `apps deploys release-notes web-prod --from <last prod deploy>` and paste the result.
