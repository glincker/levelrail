---
title: Self-host Ollama with Open WebUI
sidebarTitle: Ollama with Open WebUI
description: "Run Ollama and Open WebUI on your own server with Levelrail: HTTPS, generated settings, volume backups and rollback. Docker on a Mac cannot use the GPU."
"og:url": "https://docs.levelrail.com/guides/self-host-ollama-open-webui"
---

Ollama runs open language models on your own hardware, and Open WebUI gives you a ChatGPT-style chat interface on top of it. This guide deploys both as one Levelrail app, with Open WebUI as the only service on the host and Ollama reachable only inside the app's network.

## Deploy it

```bash
levelrail-cli templates deploy ollama-open-webui --name my-ollama-open-webui
```

Then add a domain so the chat UI is served over HTTPS. See [domains and ingress](/domains-and-ingress).

If you want to run this on a Mac, read [use a Mac as a private LLM server](/guides/use-a-mac-as-a-private-llm-server) first. Docker on a Mac cannot use the GPU, so this container does not get GPU acceleration there.

## Security and requirements

Level: **Review**. The template has no privileged-container finding.

What this template needs:

- Image is not pinned to a version tag, so a redeploy can pull different code (`ollama-api`): ollama/ollama:latest.
- Publishes ports directly on the host (`open-webui`): 8080:8080.
- Stores data in named volumes (`ollama-api`, `open-webui`): ollama_data, open_webui_data.

The template generates no secrets. Open WebUI manages its own user accounts inside the app.

## What Levelrail adds

- **HTTPS and an ingress filter** in front of any domain you attach. See [domains and ingress](/domains-and-ingress) and the [firewall](/firewall).
- **Volume backups** for `ollama_data` (the downloaded models) and `open_webui_data` (chats and accounts), to S3-compatible storage. See [backups and storage](/backups-and-storage).
- **Health-gated deploys.** A new release takes traffic only after its health check passes. See [deploy safety](/deploy-safety).
- **Rollback** to the previous image. See [deploying apps](/deploying-apps).
- **An exposure report** that shows which published ports are reachable from outside. See the [exposure audit](/exposure-audit).

## What a raw docker compose misses

- The `ollama/ollama:latest` tag moves. A plain compose file pulls whatever is newest on the next `up`. The template does not pin it, so check the image you get before you rely on it.
- `open-webui` publishes `8080:8080` straight on the host, so the port is open unless you filter it yourself. Levelrail's [exposure audit](/exposure-audit) shows this.
- Compose does not back up the `ollama_data` volume, so a lost disk loses every model you pulled.
- The template has no GPU reservation, so it runs models on the CPU on any host.

## Next steps

- [Use a Mac as a private LLM server](/guides/use-a-mac-as-a-private-llm-server) for the native-Ollama setup on macOS.
- [Backups and storage](/backups-and-storage) to schedule volume backups, then [prove your backups work](/prove-your-backups-work).
- [Template reference for Ollama + Open WebUI](/self-host/ollama-open-webui) for the full service table.
- [All guides](/guides/index).
