---
title: "Deploy contexts: env and build settings per kind of deploy"
description: "Set env vars and static build settings separately for pull request previews, branch deploys and specific branches, and see the effective config for any deploy."
"og:url": "https://docs.levelrail.com/deploy-contexts"
---

**One model for per-deploy config**

A deploy context is the kind of deploy an app is running: `production`, `deploy-preview` (a pull request preview) or `branch-deploy` (a [branch deploy](/preview-environments#branch-deploys)). You can override env vars and static build settings for each, and for a named branch or branch pattern, without touching the app's own defaults.

## What you can set

| Setting | Meaning |
| --- | --- |
| `env` | Plain env vars for the context. Use [branch env overrides](/preview-environments#env-overrides-for-one-pull-request) when a value is a secret. |
| `build.command` | Static site build command (only for `build.type: static`). |
| `build.publish` | Static site publish directory, relative to the base directory. |
| `build.args` | Build args (Dockerfile `ARG` values, or build container variables for a static build). |

Context names: `deploy-preview`, `branch-deploy`, and `branch:<pattern>` where the pattern is an exact branch name or a glob such as `release/*`. `production` is the app's own settings, so it takes no overrides.

## Resolution order

For every value, the first match wins, highest first:

1. Branch env override (`apps branch-env`), exact pattern before glob.
2. Branch context (`branch:<pattern>`), exact pattern before glob.
3. The deploy's context (`deploy-preview` or `branch-deploy`).
4. The older app-wide preview env override (`apps preview-env`), previews only.
5. The app default.

A per-pull-request override (`apps pr-env`) still wins over all of these for that pull request. Existing branch env overrides and preview env overrides keep working unchanged.

## Set contexts

<Tabs>
<Tab title="UI">

Open the app, then **Previews**, and use **Deploy contexts**. Pick a context, fill in env vars (one `KEY=VALUE` per line) and any build settings, then save. Settings you leave empty inherit.

</Tab>
<Tab title="CLI">

```bash
levelrail-cli apps contexts set web --context deploy-preview --env API_URL=https://staging.example.com
levelrail-cli apps contexts set docs --context "branch:release/*" --build-command "npm run build:rc" --publish dist
levelrail-cli apps contexts list web
levelrail-cli apps contexts clear web --context deploy-preview
```

</Tab>
<Tab title="API">

`GET`, `PUT` and `DELETE` on `/api/v1/apps/{name}/contexts` (the context is in the body, or the `context` query parameter for delete).

</Tab>
<Tab title="app.yaml">

```yaml
services:
  web:
    build: { type: static, command: npm run build, publish: dist }
    contexts:
      deploy-preview:
        env: { API_URL: https://staging.example.com }
      "branch:release/*":
        build: { command: npm run build:rc, publish: dist }
```

The block is validated against the app spec schema. Multi-service git sources apply it on preview and branch deploys. For a single-service app, set contexts in the UI, CLI or API.

</Tab>
</Tabs>

## See the effective config

The **Effective config for this deploy** view shows, for a chosen context and branch, every env var and build setting with where its value came from. Secrets, and values whose names look sensitive, stay masked. It appears under **Previews** and on each deploy's build tab.

```bash
levelrail-cli apps contexts show web --context branch-deploy --branch release/1.2
```

## What this does not do

- Production deploys use the app's own settings only.
- Context env vars are stored as plain values, so keep secrets in branch env overrides or app secrets.
- Build overrides only change what a preview or branch deploy builds. They never touch the production build.
