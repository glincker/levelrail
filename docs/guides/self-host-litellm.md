---
title: Self-host LiteLLM Proxy with Levelrail
sidebarTitle: LiteLLM Proxy
description: "Self-host LiteLLM Proxy with Postgres on Levelrail: generated master key and database password, HTTPS, volume backups, health-gated deploys and rollback."
"og:url": "https://docs.levelrail.com/guides/self-host-litellm"
---

LiteLLM Proxy is one OpenAI-format gateway in front of many model providers, with virtual keys, budgets and spend tracking. This guide runs it as a Levelrail app with its own Postgres database, which stores the keys and the models you configure in the UI.

## Deploy it

```bash
levelrail-cli templates deploy litellm --name my-litellm
```

Then add a domain for the proxy so clients reach it over HTTPS. See [domains and ingress](/domains-and-ingress).

## Security and requirements

Level: **Standard**. The template has no privileged-container finding.

What this template needs:

- Publishes ports directly on the host (`litellm`): 4000:4000.
- Passwords and keys are generated at deploy time (`db`, `litellm`): DATABASE_URL, LITELLM_MASTER_KEY, POSTGRES_PASSWORD.
- Stores data in named volumes (`db`): litellm_db_data.

The master key is generated at deploy time. Use it to create the virtual keys your apps send, and do not hand it to clients.

## What Levelrail adds

- **HTTPS and an ingress filter** in front of any domain you attach. See [domains and ingress](/domains-and-ingress) and the [firewall](/firewall).
- **Secrets stored encrypted and masked** in logs and the UI, including the master key and the database URL. See [environment variables and secrets](/environment-and-secrets).
- **Volume backups** for `litellm_db_data`, which holds the keys and spend records. See [backups and storage](/backups-and-storage).
- **Health-gated deploys.** The `litellm` service checks its liveness endpoint, and a release takes traffic only after it passes. See [deploy safety](/deploy-safety).
- **Rollback** to the previous image. See [deploying apps](/deploying-apps).

## What a raw docker compose misses

- `4000:4000` is published on the host, so the proxy is reachable directly unless you filter it.
- The `$SERVICE_*` placeholders are resolved by Levelrail at deploy time and stored as secrets. Plain `docker compose up` cannot resolve them, so you would write the master key and database password yourself.
- `STORE_MODEL_IN_DB` is set to `True`, so models you add in the UI are saved to the database, not to a config file. Back up that database.
- Compose does not back up `litellm_db_data`, so the spend records and virtual keys exist on one disk.
- Compose has no rollback to an earlier revision.

## Next steps

- [Backups and storage](/backups-and-storage) for `litellm_db_data`, then [prove your backups work](/prove-your-backups-work).
- [Connecting apps to databases](/connecting-apps-to-databases) if other apps need the same Postgres.
- [Template reference for LiteLLM Proxy](/self-host/litellm) for the full service table.
- [All guides](/guides/index).
