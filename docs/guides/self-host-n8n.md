---
title: Self-host n8n with Levelrail
sidebarTitle: n8n
description: "Self-host n8n workflow automation with Levelrail: one-command deploy, a generated encryption key, HTTPS, volume backups, health-gated deploys and rollback."
"og:url": "https://docs.levelrail.com/guides/self-host-n8n"
---

n8n is a workflow automation tool with a visual, node-based editor that connects your apps and APIs. This guide runs it as a single Levelrail app with its data in one named volume.

## Deploy it

```bash
levelrail-cli templates deploy n8n --name my-n8n
```

Then add a domain for the editor so it is served over HTTPS. See [domains and ingress](/domains-and-ingress).

## Security and requirements

Level: **Standard**. The template has no privileged-container finding.

What this template needs:

- Publishes ports directly on the host (`n8n`): 5678:5678.
- Passwords and keys are generated at deploy time (`n8n`): N8N_ENCRYPTION_KEY.
- Stores data in named volumes (`n8n`): n8n_data.

The encryption key is generated at deploy time and stored as a secret, not written into the template. n8n uses it to encrypt the credentials you save in workflows, so keep it safe: without it, those saved credentials cannot be decrypted.

## What Levelrail adds

- **HTTPS and an ingress filter** in front of any domain you attach. See [domains and ingress](/domains-and-ingress) and the [firewall](/firewall).
- **Secrets stored encrypted and masked** in logs and the UI. See [environment variables and secrets](/environment-and-secrets).
- **Volume backups** for `n8n_data`, which holds workflows and the local database. See [backups and storage](/backups-and-storage).
- **Health-gated deploys** that wait for the template's health check before traffic switches. See [deploy safety](/deploy-safety).
- **Rollback** to the previous image. See [deploying apps](/deploying-apps).

## What a raw docker compose misses

- The image is pinned to `n8nio/n8n:1.62.1`, but the encryption key is a `$SERVICE_HEX_64_ENCRYPTIONKEY` placeholder. Plain Docker Compose cannot resolve it, so you would generate and store the key yourself.
- `5678:5678` is published on the host, so the editor is reachable without a domain or filter unless you put one in front of it.
- Compose does not back up `n8n_data`, so workflows and credentials exist on one disk only.
- Compose has no rollback to an earlier revision. Levelrail keeps previous images for that.

## Next steps

- [Backups and storage](/backups-and-storage) to schedule `n8n_data` backups, then [prove your backups work](/prove-your-backups-work).
- [Environment variables and secrets](/environment-and-secrets) to manage the encryption key.
- [Template reference for n8n](/self-host/n8n) for the full service table.
- [All guides](/guides/index).
