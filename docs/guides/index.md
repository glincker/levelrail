---
title: Guides
description: "Task-focused Levelrail guides: compare it with Coolify, self-host Postgres with verified backups, ship zero-downtime deploys and run OpenClaw agents safely."
"og:url": "https://docs.levelrail.com/guides/index"
---

Guides answer a specific question end to end and link into the reference pages for detail. For step-by-step walkthroughs on a live server, see [tutorials](/tutorials).

- [Coolify vs Levelrail for self-hosters](/guides/coolify-vs-levelrail): how they differ, when each fits, and what Levelrail does not do yet.
- [Self-host Postgres with verified backups](/guides/self-host-postgres-with-backups): create a database, back it up to S3-compatible storage, and rehearse a restore.
- [Zero-downtime deploys without Kubernetes](/guides/zero-downtime-deploys-without-kubernetes): readiness gating, digest-pinned images, rollback, and freeze windows on plain Docker.
- [Control Levelrail from OpenClaw](/guides/control-levelrail-from-openclaw): a least-privilege agent token, the MCP config, and what an agent can and cannot do.
- [Use your Mac mini or MacBook as a private LLM server](/guides/use-a-mac-as-a-private-llm-server): run Ollama, LM Studio or MLX natively on a Mac and serve it through the gateway with keys, limits and a fallback.

## Self-host popular AI and automation apps

- [Self-host Ollama with Open WebUI](/guides/self-host-ollama-open-webui): a local model runtime with a chat interface, and why Docker on a Mac cannot use the GPU.
- [Self-host n8n with Levelrail](/guides/self-host-n8n): workflow automation with a generated encryption key and volume backups.
- [Self-host Langflow with Levelrail](/guides/self-host-langflow): a visual workflow builder on its own Postgres database.
- [Self-host Flowise with Levelrail](/guides/self-host-flowise): an LLM flow builder with a generated admin password.
- [Self-host OpenClaw with Levelrail](/guides/self-host-openclaw): the agent gateway, and the start-up settings to review before you expose it.
- [Self-host Qdrant with Levelrail](/guides/self-host-qdrant): vector search with a generated API key.
- [Self-host LiteLLM Proxy with Levelrail](/guides/self-host-litellm): one OpenAI-format gateway with a generated master key and Postgres.
- [Run AI agents safely on your own server](/guides/run-ai-agents-safely): the risk, the controls Levelrail gives you today, the self-host AI guides, and what is not covered yet.
- [OpenClaw on Docker Compose or on Levelrail](/guides/openclaw-on-docker-compose-vs-levelrail): a factual comparison of the OpenClaw template run by hand and on Levelrail, with the checks that passed and the known limits.
- [Use Ollama and Open WebUI with a Mac and Levelrail](/guides/use-ollama-open-webui-with-a-mac-and-levelrail): an end-to-end walkthrough from a native Ollama on a Mac to Open WebUI with a scoped key.
