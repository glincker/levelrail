---
description: Step-by-step tutorials for self-hosting with Levelrail, from Docker deploys and Postgres to S3 backups, GitHub Actions, and built-in logs and metrics.
---

# Tutorials

Each tutorial is a complete walkthrough you can finish in 10 to 15 minutes on your own server. Every command in them was run against a real instance, and the output shown is real. If something does not match what you see, [open an issue](https://github.com/glincker/levelrail/issues).

New here? Install Levelrail with [Getting started](../getting-started.md) first, then pick any tutorial below. They build on each other lightly, but each one stands alone.

## Deploy and ship

- [Deploy a Docker app with a custom domain and instant rollback](deploy-a-docker-app.md): run any image on your own server, put it on a domain, ship a second version, and roll back with one command.
- [Zero-downtime deploys with health checks](zero-downtime-deploys-with-health-checks.md): make a bad release fail before it replaces a good one, read the structured failure, and recover.
- [Deploy from GitHub Actions](deploy-from-github-actions.md): build in CI, deploy to your own server on every push, and fail the pipeline if the rollout fails.

## Data and backups

- [Connect an app to a managed Postgres database](connect-an-app-to-postgres.md): create the database, connect it with one command, and confirm the connection string reaches your app.
- [Back up Postgres to S3 and test the restore](back-up-postgres-to-s3.md): schedule backups to S3, R2, or any S3-compatible bucket, verify them, and restore over the original or into a new copy.

## Self-host an app

- [Self-host Vaultwarden (Bitwarden-compatible)](self-host-vaultwarden.md): deploy from the template catalog, add a domain, and back up the data volume.

## Operate

- [Debug an app with built-in logs and metrics](debug-an-app-with-logs-and-metrics.md): search logs, follow them live, read CPU and memory, and start from the health score. No Grafana to install.

## Looking for something else?

The how-to guides cover one task each in more depth, and the [CLI reference](../cli-reference.md) lists every command. To compare Levelrail with other self-hosted platforms, see the [comparison](../comparison.md).
