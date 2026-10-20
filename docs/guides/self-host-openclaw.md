---
title: Self-host OpenClaw with Levelrail
sidebarTitle: OpenClaw
description: "Self-host the OpenClaw agent gateway on Levelrail with a generated gateway token, HTTPS, volume backups and rollback, and the settings to review first."
"og:url": "https://docs.levelrail.com/guides/self-host-openclaw"
---

OpenClaw is a self-hosted personal AI agent gateway. It connects chat apps and tools to the model provider you choose, and it keeps its state in one volume. This guide runs the gateway as a single Levelrail app. To let an agent operate Levelrail itself, see [control Levelrail from OpenClaw](/guides/control-levelrail-from-openclaw).

## Deploy it

```bash
levelrail-cli templates deploy openclaw --name my-openclaw
```

Then add a domain so the gateway is served over HTTPS. See [domains and ingress](/domains-and-ingress).

## Security and requirements

Level: **Standard**. The template has no privileged-container finding.

What this template needs:

- Publishes ports directly on the host (`openclaw-gateway`): 18789:18789.
- Passwords and keys are generated at deploy time (`openclaw-gateway`): OPENCLAW_GATEWAY_TOKEN.
- Stores data in named volumes (`openclaw-gateway`): openclaw_data.

Before you expose the gateway, review how the template starts it. The container command binds the gateway to the LAN interface (`--bind lan`) and passes `--allow-unconfigured`, so it starts without a completed setup. Use the generated gateway token for any client that connects to it.

## What Levelrail adds

- **HTTPS and an ingress filter** in front of any domain you attach. See [domains and ingress](/domains-and-ingress) and the [firewall](/firewall).
- **Secrets stored encrypted and masked** in logs and the UI, including the gateway token. See [environment variables and secrets](/environment-and-secrets).
- **Volume backups** for `openclaw_data`, which holds the gateway state and workspace. See [backups and storage](/backups-and-storage).
- **Health-gated deploys.** The gateway has a health check, and a release takes traffic only after it passes. See [deploy safety](/deploy-safety).
- **Rollback** to the previous image. See [deploying apps](/deploying-apps).

## What a raw docker compose misses

- `18789:18789` is published on the host. The gateway is reachable directly unless you filter it.
- The gateway token is a `$SERVICE_HEX_32_GATEWAYTOKEN` placeholder. Plain Docker Compose cannot resolve it, so you would generate and store the token yourself.
- Compose does not back up `openclaw_data`, so the agent state and workspace exist on one disk.
- Compose has no rollback to an earlier revision.

## Next steps

- [Control Levelrail from OpenClaw](/guides/control-levelrail-from-openclaw) to give the agent a least-privilege token.
- [Backups and storage](/backups-and-storage) for `openclaw_data`, then [prove your backups work](/prove-your-backups-work).
- [Template reference for OpenClaw](/self-host/openclaw) for the full service table.
- [All guides](/guides/index).
