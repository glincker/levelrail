---
title: Self-host Langflow with Levelrail
sidebarTitle: Langflow
description: "Self-host Langflow with Postgres on Levelrail: generated database and admin passwords, HTTPS, volume backups, health-gated deploys and rollback."
"og:url": "https://docs.levelrail.com/guides/self-host-langflow"
---

Langflow is a visual, low-code editor for building multi-agent and retrieval workflows in Python. This guide runs Langflow with its own Postgres database as one Levelrail app, with each service's data in a named volume.

## Deploy it

```bash
levelrail-cli templates deploy langflow --name my-langflow
```

Then add a domain for the editor so it is served over HTTPS. See [domains and ingress](/domains-and-ingress).

## Security and requirements

Level: **Standard**. The template has no privileged-container finding.

What this template needs:

- Publishes ports directly on the host (`langflow`): 7860:7860.
- Passwords and keys are generated at deploy time (`db`, `langflow`): LANGFLOW_DATABASE_URL, LANGFLOW_SUPERUSER_PASSWORD, POSTGRES_PASSWORD.
- Stores data in named volumes (`db`, `langflow`): langflow_db_data, langflow_data.

The template sets `LANGFLOW_AUTO_LOGIN` to `false`, so the editor asks for the superuser login. The superuser password is generated, so you need to read it from the app's secrets before your first sign-in.

## What Levelrail adds

- **HTTPS and an ingress filter** in front of any domain you attach. See [domains and ingress](/domains-and-ingress) and the [firewall](/firewall).
- **Secrets stored encrypted and masked** in logs and the UI, including the generated database URL. See [environment variables and secrets](/environment-and-secrets).
- **Volume backups** for `langflow_db_data` and `langflow_data`. See [backups and storage](/backups-and-storage).
- **Health-gated deploys.** The `langflow` service has a health check, and a release takes traffic only after it passes. See [deploy safety](/deploy-safety).
- **Rollback** to the previous image. See [deploying apps](/deploying-apps).

## What a raw docker compose misses

- The `$SERVICE_PASSWORD_*` placeholders in the template are resolved by Levelrail at deploy time and stored as secrets. A plain `docker compose up` cannot resolve them, so you would write the passwords yourself.
- `7860:7860` is published on the host. The Langflow editor is reachable from the internet unless you filter it.
- Compose does not back up either volume, so the flows and the Postgres data sit on one disk.
- Compose has no rollback to an earlier revision.

## Next steps

- [Backups and storage](/backups-and-storage) for both volumes, then [prove your backups work](/prove-your-backups-work).
- [Connecting apps to databases](/connecting-apps-to-databases) if other apps need the same Postgres.
- [Template reference for Langflow](/self-host/langflow) for the full service table.
- [All guides](/guides/index).
