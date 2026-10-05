---
description: Step-by-step tutorials for self-hosting with Levelrail, from Docker deploys and Postgres to S3 backups, GitHub Actions, and built-in logs and metrics.
---

# Tutorials

Each tutorial is a complete walkthrough you can finish in 10 to 15 minutes on your own server. The commands and sample output come from a real instance, so IDs, timestamps, and digests will differ on yours. If something does not match what you see, [open an issue](https://github.com/glincker/levelrail/issues).

New here? Install Levelrail with [Getting started](../getting-started.md) first, then pick any tutorial below. They build on each other lightly, but each one stands alone.

## Deploy and ship

<CardGroup :cols="2">

<Card title="Deploy a Docker app with a custom domain and instant rollback" href="/tutorials/deploy-a-docker-app">

Run any image on your own server, put it on a domain, ship a second version, and roll back with one command.

</Card>
<Card title="Zero-downtime deploys with health checks" href="/tutorials/zero-downtime-deploys-with-health-checks">

Make a bad release fail before it replaces a good one, read the structured failure, and recover.

</Card>
<Card title="Deploy from GitHub Actions" href="/tutorials/deploy-from-github-actions">

Build in CI, deploy to your own server on every push, and fail the pipeline if the rollout fails.

</Card>
</CardGroup>

## Data and backups

<CardGroup :cols="2">

<Card title="Connect an app to a managed Postgres database" href="/tutorials/connect-an-app-to-postgres">

Create the database, connect it with one command, and confirm the connection string reaches your app.

</Card>
<Card title="Back up Postgres to S3 and test the restore" href="/tutorials/back-up-postgres-to-s3">

Schedule backups to S3, R2, or any S3-compatible bucket, verify them, and restore over the original or into a new copy.

</Card>
</CardGroup>

## Self-host an app

<CardGroup :cols="2">

<Card title="Self-host Vaultwarden (Bitwarden-compatible)" href="/tutorials/self-host-vaultwarden">

Deploy from the template catalog, add a domain, and back up the data volume.

</Card>
</CardGroup>

## Operate

<CardGroup :cols="2">

<Card title="Debug an app with built-in logs and metrics" href="/tutorials/debug-an-app-with-logs-and-metrics">

Search logs, follow them live, read CPU and memory, and start from the health score. No Grafana to install.

</Card>
</CardGroup>

## Looking for something else?

The how-to guides cover one task each in more depth, and the [CLI reference](../cli-reference.md) lists every command. To compare Levelrail with other self-hosted platforms, see the [comparison](../comparison.md).
