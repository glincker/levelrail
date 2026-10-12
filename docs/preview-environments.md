---
title: "Preview environments per pull request on your own servers"
description: "Levelrail deploys a preview environment for each pull request on GitHub, GitLab, Bitbucket and Gitea and tears it down when the pull request closes."
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
    On GitHub, each preview shows as a deployment in the preview environment that moves from in progress to success or failure.
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
