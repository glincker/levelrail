---
title: "Self-hosted alternative to Cloud Run for steady workloads"
description: "When an always-on container is cheaper on your own server than on Cloud Run, when it is not, and how Levelrail fits for git or image deploys."
---

**Run the same container on a server you own**

Cloud Run is a good fit for bursty traffic. For a service that is up all day, a fixed-price server can cost less. Levelrail deploys the same image with TLS, rollback, logs and metrics. You operate the server.

[Migration guide](/migrating-from-cloud-run) | [Get started](/getting-started)

```bash
curl -fsSL https://levelrail.com/install.sh | sudo sh
```

## What carries over from Cloud Run

<CardGroup cols={3}>
  <Card title="The same container image">
    A Cloud Run service is an image that listens on a port. Deploy that image from a registry, with a registry credential for private ones. No rebuild is needed.
  </Card>
  <Card title="Revisions become deploys">
    Every deploy is recorded with its image and status. Prior images stay pinned, so rolling back switches to an image that is still there.
  </Card>
  <Card title="Logs and metrics included">
    Logs with search, request rate, error rate and latency percentiles are stored on your node. Nothing is billed by volume.
  </Card>
</CardGroup>

## When moving makes sense

The service is up all day with steady traffic, so you pay for compute around the clock on Cloud Run anyway.

You run several services and could put them on one server, which spreads a fixed price across them.

You want logs, metrics and data on machines you control, or you are tired of a bill that moves with traffic.

You are comfortable patching a Linux host, watching disk space and testing restores, or you have someone who is.

## When to stay on Cloud Run

Traffic is spiky or rare. Scale to zero and per request billing are hard to beat, and Levelrail has no autoscaling.

You need capacity to appear in seconds under load, or to span zones or regions without building it yourself.

You depend on Google identity and networking features such as workload identity or private connectivity to other Google services.

Nobody wants to be on call for a server. Moving trades a bill for operations work.

## What to know before you try it

Levelrail is self-hosted and early. It is in beta, there is no stable release yet, and the feature status page says which parts are proven and which are not.

You operate the server. OS updates, the Docker daemon, disk space, backups and restore tests are your job. A single server is a single failure domain.

Levelrail does not run on Cloud Run and does not manage Google Cloud resources. It runs on Linux machines you provide, including VMs rented from any provider.

Run it next to Cloud Run first and compare real numbers. The migration guide has a shadow run checklist.

## A cost model, with the assumptions written down

Estimates for one service with 1 vCPU and 0.5 GiB of memory for a 30 day month, using Cloud Run prices for Tier 1 regions such as us-central1, read from Google's public pricing page on 2026-10-11. Prices change, and your region may be in another tier. Check the page before you decide. Egress, load balancers, Artifact Registry, Cloud SQL and your own time are excluded on both sides.

| | Cloud Run (estimate, 2026-10-11) | Server you run (assumption) |
| --- | --- | --- |
| Always on, instance-based billing: 1 vCPU at $0.000018 per second, 0.5 GiB at $0.000002 per GiB-second | About $49 per month, about $44 after the monthly free tier | A small VPS with 2 vCPU and 4 GB at roughly $5 to $12 per month. This is an assumed range, not a quote. It can also host other services. |
| Request-based billing, one minimum instance, busy 5 percent of the time | About $12.50 per month before the free tier, plus requests above 2 million at $0.40 per million | Same fixed server price, whatever the traffic |
| Request-based billing, one minimum instance, busy 20 percent of the time | About $21 per month before the free tier | Same fixed server price |
| Request-based billing, busy all the time ($0.000024 per vCPU-second, $0.0000025 per GiB-second) | About $65 per month before the free tier. Instance-based billing is the cheaper choice here. | Same fixed server price |
| Zero minimum instances and traffic in short bursts | Can be a few dollars or free, because you pay only while a request is being handled | The server bill does not shrink with traffic. Sleep when idle stops a container, but the machine is still rented. |

[Read the Cloud Run migration runbook](/migrating-from-cloud-run)

## From Cloud Run to your own server

<Steps>
  <Step title="Export the service definition">
    gcloud run services describe with the export format gives one YAML with the image, port, env, resources and probes.
  </Step>
  <Step title="Deploy the image to Levelrail">
    Create a registry credential if the image is private, set env and secrets, and check readiness on a temporary address.
  </Step>
  <Step title="Move data and DNS">
    Dump Cloud SQL, restore into a managed database, lower TTLs, switch, and keep Cloud Run as the fallback until you trust the new setup.
  </Step>
</Steps>

## Frequently asked questions

<AccordionGroup>
  <Accordion title="Can I deploy my existing Cloud Run image unchanged?">
    Usually yes. Point Levelrail at the image and the port your container listens on, and set PORT as an env var if your app reads it. A private Artifact Registry image needs a registry credential.
  </Accordion>
  <Accordion title="Does it scale to zero or autoscale?">
    Not like Cloud Run. Replicas are fixed. An app can be put to sleep after a quiet period and woken on the next request, which is slower than a Cloud Run cold start.
  </Accordion>
  <Accordion title="Is there an importer for Cloud Run?">
    No. The runbook maps each Cloud Run concept to its Levelrail equivalent and lists what has none.
  </Accordion>
  <Accordion title="What does it cost?">
    The software is free under Apache 2.0. Your cost is the servers you run it on, plus your time operating them.
  </Accordion>
</AccordionGroup>

## Related

- [Migrating from Cloud Run](/migrating-from-cloud-run)
- [Vercel alternative](/vercel-alternative)
- [Cost estimate](/cost-estimate)
- [Feature status](/feature-status)

## Check the numbers on one service

Run a single steady service on it next to Cloud Run, and compare cost and behavior before moving anything else.

[Migration guide](/migrating-from-cloud-run) | [Get started](/getting-started)
