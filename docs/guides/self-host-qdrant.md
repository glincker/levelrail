---
title: Self-host Qdrant with Levelrail
sidebarTitle: Qdrant
description: "Self-host Qdrant vector search on Levelrail: a generated API key, HTTPS through the ingress, volume backups, health-gated deploys and rollback."
"og:url": "https://docs.levelrail.com/guides/self-host-qdrant"
---

Qdrant is a vector similarity search engine for storing and querying embeddings. This guide runs Qdrant as a single Levelrail app, protected by a generated API key and with its storage in one named volume.

## Deploy it

```bash
levelrail-cli templates deploy qdrant --name my-qdrant
```

Then add a domain so clients reach Qdrant over HTTPS. See [domains and ingress](/domains-and-ingress).

## Security and requirements

Level: **Standard**. The template has no privileged-container finding.

What this template needs:

- Publishes ports directly on the host (`qdrant`): 6333:6333.
- Passwords and keys are generated at deploy time (`qdrant`): QDRANT__SERVICE__API_KEY.
- Stores data in named volumes (`qdrant`): qdrant_data.

Clients must send the generated API key. Read it from the app's secrets and keep it out of your application's source code.

## What Levelrail adds

- **HTTPS and an ingress filter** in front of any domain you attach. See [domains and ingress](/domains-and-ingress) and the [firewall](/firewall).
- **Secrets stored encrypted and masked** in logs and the UI, including the API key. See [environment variables and secrets](/environment-and-secrets).
- **Volume backups** for `qdrant_data`, the vector storage. See [backups and storage](/backups-and-storage).
- **Health-gated deploys.** The template's health check must pass before traffic moves. See [deploy safety](/deploy-safety).
- **Rollback** to the previous image. See [deploying apps](/deploying-apps).

## What a raw docker compose misses

- The template uses `qdrant/qdrant:v1.12.4`. Its source notes that this tag could not be checked against a live registry when it was added, so confirm it works before you rely on it.
- `6333:6333` is published on the host, so the REST API is reachable without a domain or filter unless you add one.
- The API key is a `$SERVICE_HEX_64_APIKEY` placeholder. Plain Docker Compose cannot resolve it, so you would generate and store the key yourself.
- Compose does not back up `qdrant_data`, so the stored vectors exist on one disk.
- Compose has no rollback to an earlier revision.

## Next steps

- [Backups and storage](/backups-and-storage) to schedule `qdrant_data` backups, then [prove your backups work](/prove-your-backups-work).
- [Exposure audit](/exposure-audit) to confirm which ports are reachable from outside.
- [Template reference for Qdrant](/self-host/qdrant) for the full service table.
- [All guides](/guides/index).
