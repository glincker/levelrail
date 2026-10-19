---
title: "Zero-downtime deploys with Docker: readiness-gated cutover and rollback"
description: "How Levelrail ships without downtime: blue-green, rolling or recreate strategies gated on real readiness probes and digest-pinned images."
"og:url": "https://docs.levelrail.com/zero-downtime-deploys"
---

**A deploy is live only when it is actually ready**

Traffic cuts over after the new container passes a real readiness probe, the previous release stays available, and four guarantees keep the wrong build from serving.

[Read deploy safety](/deploy-safety) | [View on GitHub](https://github.com/glincker/levelrail)

```bash
curl -fsSL https://levelrail.com/install.sh | sudo sh
```

## Four guarantees between a deploy and serving traffic

<CardGroup cols={3}>
  <Card title="Digest-truthful deploys">
    A deploy is pinned to the image digest it resolved, so a moved tag cannot change what serves. Rollbacks deploy the recorded pinned reference.
  </Card>
  <Card title="Stale-deploy guard">
    Automated deploys carry a per-app sequence number and commit order, so an older build still in flight cannot overwrite a newer one. Manual deploys and rollbacks are never rejected.
  </Card>
  <Card title="Freeze windows">
    Block automated deploys on a cron schedule, for example every Friday evening, and release them afterwards.
  </Card>
  <Card title="Instant rollback window">
    The previous release is held briefly after cutover, and prior images stay pinned so garbage collection cannot remove a rollback target.
  </Card>
  <Card title="Three strategies">
    Blue-green, rolling or recreate, all gated on readiness and liveness probes.
  </Card>
  <Card title="A reason for every state">
    The reconciler records a status condition with a reason after each pass, so a stuck deploy explains itself.
  </Card>
</CardGroup>

## Frequently asked questions

<AccordionGroup>
  <Accordion title="What does zero downtime require from my app?">
    A readiness endpoint that returns success only when the app can serve. Cutover waits for it.
  </Accordion>
  <Accordion title="What if the new version never becomes ready?">
    Traffic stays on the current release and the deploy is reported as failed, with logs and a reason.
  </Accordion>
  <Accordion title="Can a stale build overwrite a newer one?">
    Not for automated deploys: the stale-deploy guard skips them. Manual deploys and explicit rollbacks always run.
  </Accordion>
  <Accordion title="Does it work on one server?">
    Yes. The same cutover logic runs on a single node.
  </Accordion>
</AccordionGroup>

## Ship without holding your breath

Read the deploy safety guide or try it on a spare server.

[Read deploy safety](/deploy-safety) | [View on GitHub](https://github.com/glincker/levelrail)

## Troubleshooting

For issues with zero-downtime deploys, readiness gates, or rollbacks, see [Deploy safety](deploy-safety.md) for safety mechanisms and [Deploying apps](deploying-apps.md) for deploy troubleshooting. Check [Deploy failures](deploy-failures.md) for structured failure codes and what each one means.
