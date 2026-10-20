---
title: Self-host Flowise with Levelrail
sidebarTitle: Flowise
description: "Self-host Flowise, the drag-and-drop LLM flow builder, on Levelrail: generated admin password, HTTPS, volume backups, health-gated deploys and rollback."
"og:url": "https://docs.levelrail.com/guides/self-host-flowise"
---

Flowise is a drag-and-drop builder for LLM chatbots, agents and retrieval flows, and each flow can be called through an API. This guide runs Flowise as a single Levelrail app with its data in one named volume.

## Deploy it

```bash
levelrail-cli templates deploy flowise --name my-flowise
```

Then add a domain for the builder so it is served over HTTPS. See [domains and ingress](/domains-and-ingress).

## Security and requirements

Level: **Standard**. The template has no privileged-container finding.

What this template needs:

- Publishes ports directly on the host (`flowise`): 3000:3000.
- Passwords and keys are generated at deploy time (`flowise`): FLOWISE_PASSWORD.
- Stores data in named volumes (`flowise`): flowise_data.

The username is `admin` and the password is generated. Read it from the app's secrets before you sign in for the first time.

## What Levelrail adds

- **HTTPS and an ingress filter** in front of any domain you attach. See [domains and ingress](/domains-and-ingress) and the [firewall](/firewall).
- **Secrets stored encrypted and masked** in logs and the UI. See [environment variables and secrets](/environment-and-secrets).
- **Volume backups** for `flowise_data`, which holds the flows, the API key file and the secret key file that Flowise writes there (`APIKEY_PATH` and `SECRETKEY_PATH`). See [backups and storage](/backups-and-storage).
- **Health-gated deploys.** The template's health check must pass before traffic moves. See [deploy safety](/deploy-safety).
- **Rollback** to the previous image. See [deploying apps](/deploying-apps).

## What a raw docker compose misses

- `3000:3000` is published on the host, so the builder is reachable directly unless you put a filter in front of it.
- The admin password is a `$SERVICE_PASSWORD_ADMIN` placeholder. Plain Docker Compose cannot resolve it, so you would choose and store the password yourself.
- Compose does not back up `flowise_data`. Losing that volume loses your flows and the keys stored with them.
- Compose has no rollback to an earlier revision.

## Next steps

- [Backups and storage](/backups-and-storage) to schedule `flowise_data` backups, then [prove your backups work](/prove-your-backups-work).
- [Flowise with Postgres and Qdrant](/self-host/flowise-postgres) if you want a database and a vector store behind it.
- [Template reference for Flowise](/self-host/flowise) for the full service table.
- [All guides](/guides/index).
