---
title: "Preview environments per pull request on your own servers"
description: "Levelrail deploys a preview environment for each pull request on GitHub, GitLab, Bitbucket and Gitea and tears it down when the pull request closes."
"og:url": "https://docs.levelrail.com/preview-environments"
---

**Every pull request gets its own running copy**

Open or update a pull request and Levelrail deploys a preview. Close it and the preview is torn down, whether or not it merged.

[Set up git integrations](/git-integrations) | [View on GitHub](https://github.com/glincker/levelrail)

```bash
curl -fsSL https://levelrail.com/install.sh | sudo sh
```

## How previews work

<CardGroup cols={3}>
  <Card title="Opened or updated">
    A new pull request or a new push to it deploys or redeploys a preview copy automatically.
  </Card>
  <Card title="Closed">
    Closing a pull request tears the preview down regardless of whether it merged.
  </Card>
  <Card title="Status back to your git host">
    Each preview gets a commit status and an updated pull request comment. On a GitHub source with status reporting on (the default, off with `APP_GIT_STATUS_ENABLED=false` or the app's `report_status` setting), it also shows as a deployment in the `preview` environment.
  </Card>
  <Card title="Disposable databases">
    An app can provision a full, disposable database for every pull request preview that is destroyed with the preview.
  </Card>
  <Card title="Multi-service previews">
    A multi-service app fans a preview out the same way a real deploy does, never through a second, drifted code path.
  </Card>
  <Card title="Four git providers">
    GitHub, GitLab, Bitbucket and Gitea sources all support previews.
  </Card>
</CardGroup>

## The pull request comment

Each preview keeps one comment on its pull request, edited in place as the preview moves from building to ready, failed or removed. It shows:

- **Status**, and the preview **URL** once it is ready.
- The **commit** it was built from and the time it was **last updated**.
- A **build logs** link to the dashboard, and the **deploy time** of the last build.
- The **cleanup** time if nothing changes, and how many **env overrides** apply.

The comment never contains an env var name or value, so secrets set for a pull request cannot leak into it.

## Static sites

If the app is a static site (`build.type: static`), the preview publishes the pull request's head commit as its own static site named `<app>-pr-<number>`, with no container. It is built the same way as a production deploy and served on the same preview URL a container preview would get. The live site, its domain and its rollback history are not touched, and the same status checks and pull request comment are posted.

- Closing or merging the pull request, the stale sweep and a manual **Tear down** remove the preview's files, route and site record. Only that preview's own directory is deleted.
- Without a base domain or primary domain the preview has no host name, so it is built but not reachable.
- The basic auth gate is not applied to static previews, and the preview is not given a no-index header. The preview comment notes it when the gate is on. Do not put private content in a static preview.
- Database strategies, disposable databases and idle sleep do not apply, since a static site has no runtime.

## Env overrides for one pull request

Besides the app-wide `preview-env`, branch-pattern `branch-env` and per-context [deploy contexts](/deploy-contexts) overrides, you can set values for a single pull request, for example to point one preview at a staging API. They win over every other source for that pull request and apply on its next deploy, so push a commit after setting them.

<Tabs>
<Tab title="UI">

Open the app, then **Previews**, and use **Env overrides** on the pull request's row. Tick **Store encrypted** for a secret.

</Tab>
<Tab title="CLI">

```bash
levelrail-cli apps pr-env set web FEATURE_FLAG --pr 42 --value on
levelrail-cli apps pr-env set web API_KEY --pr 42 --value sk-test-123 --secret
levelrail-cli apps pr-env list web --pr 42
levelrail-cli apps pr-env clear web prenv_abc123 --pr 42
```

</Tab>
<Tab title="API">

`GET`, `POST` on `/api/v1/apps/{name}/previews/{number}/env` and `DELETE /api/v1/apps/{name}/previews/{number}/env/{id}`. A secret is stored encrypted and its value is never returned.

</Tab>
</Tabs>

Fork pull requests only receive overrides when you approved sharing the app's secrets with that fork.

## What cleanup removes

When a pull request closes or merges, Levelrail removes everything the preview created:

- the preview app and its containers (or, for a static site, its published files), and the DNS record,
- every disposable database and its data volumes,
- the preview's own named volumes (a volume that another service still mounts is kept),
- the pull request's env overrides and their encrypted values.

Cleanup is safe to repeat. A closed event delivered twice is ignored the second time. If a step fails, such as a volume that is still in use, the preview is marked failed with the reason and the next close event, manual **Tear down**, or the stale sweep finishes the job. A pull request reopened later starts from a fresh preview with no old overrides.

## Branch deploys

A branch deploy gives a branch its own stable URL, separate from pull request previews. It is off by default. Choose which branches get one on the app's Previews page (Branch deploys), or from the CLI:

```bash
levelrail-cli apps branch-deploys set web --mode patterns --branch staging --branch "feature/*"
levelrail-cli apps branch-deploys list web
levelrail-cli apps branch-deploys delete web staging
```

- Modes: `off`, `all`, or `patterns` (exact names or globs where `*` matches any characters, including `/`). The production branch never gets one.
- The URL comes from the branch name and stays the same across pushes: `<app>-br-<branch>` under the apps base domain, or `br-<branch>.<app>.<primary-domain>` without one. A branch name with characters outside lowercase letters, digits and dashes gets a short hash suffix, so `feature/x` and `feature-x` never share a URL. Names are capped to one DNS label.
- Each push to the branch rebuilds it. Branch-scoped env overrides (`apps branch-env`) and the app's preview policy (resource limits, basic auth gate, search engine hiding, idle sleep) apply.
- It is removed when the branch is deleted (the provider's push event for the deletion) and after it sees no push for the idle TTL, `APP_BRANCH_DEPLOY_TTL` (default 336h). `APP_BRANCH_DEPLOY_MAX_PER_APP` caps live branch deploys per app (default 10); a push past the cap is ignored with a note.
- Works with GitHub, GitLab, Gitea and Bitbucket push webhooks. It does not provision preview databases. Per-context env and build settings are covered in [deploy contexts](/deploy-contexts).

## Frequently asked questions

<AccordionGroup>
  <Accordion title="Which git hosts support previews?">
    GitHub, GitLab, Bitbucket and Gitea.
  </Accordion>
  <Accordion title="Are previews on by default?">
    Previews are gated on a per-app preview setting, so you turn them on where you want them.
  </Accordion>
  <Accordion title="What about secrets in previews?">
    Previews use the app's configuration, so review which environment values a preview should receive before enabling it for untrusted forks.
  </Accordion>
  <Accordion title="Is this the same as deploy thumbnails?">
    No. Deploy previews, the thumbnails in deploy history, are a separate feature from per-pull-request preview environments.
  </Accordion>
</AccordionGroup>

## Review changes on a real URL

Connect a repository and open a pull request.

[Set up git integrations](/git-integrations) | [View on GitHub](https://github.com/glincker/levelrail)
