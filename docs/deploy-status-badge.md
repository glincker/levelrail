---
description: An opt-in, per-app SVG badge showing an app's latest deploy status and when it happened, for embedding in your own project's README.
---

# Deploy status badge

Every app can serve a small public SVG badge showing its latest deploy status, the same idea as a GitHub Actions workflow badge or a Vercel deployment badge. It is off by default: a private app's deploy status is never exposed until you turn it on for that specific app.

## Turning it on

Open the app, then **Deploys**, and find the **Deploy status badge** card. Flip **Enabled**, then copy the markdown snippet shown underneath into your own README:

```markdown
![deploy status](https://your-server/api/v1/apps/web/badge.svg)
```

Replace the URL with the one the dashboard shows you: it already has your server's address and the app's real name filled in.

## What it looks like

The badge is a flat, two-color pill, rendered as real SVG on every request:

| Latest deploy attempt | Badge |
| --- | --- |
| Succeeded | Green, `success, 2h ago` |
| Failed | Red, `failed, 10m ago` |
| Running or queued | Yellow, `deploying` |
| No deploy attempts yet | Gray, `no deploys` |
| Anything else (canceled, held, superseded) | Gray, `unknown` |

## Access

`GET /api/v1/apps/{name}/badge.svg` needs no authentication: that is the point, so it works embedded in a public README on GitHub. It only ever reveals the app's own name (which you chose to put in the URL), its current deploy status, and when that deploy happened, nothing else about the app's configuration, environment, or infrastructure.

An app with the badge disabled answers the exact same way as a URL for an app that does not exist: a plain 404. There is no way to tell the two apart from the outside.

Turn it off the same way you turned it on, from the same card.
