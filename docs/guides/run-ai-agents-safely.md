---
title: Run AI agents safely on your own server
sidebarTitle: Run AI agents safely
description: "What it takes to run AI agents on your own server: the risk, what Levelrail gives you today, the self-host AI guides, and what is not covered yet."
"og:url": "https://docs.levelrail.com/guides/run-ai-agents-safely"
---

An AI agent runs code, calls APIs and holds keys. On your own server, the damage it can do is bounded by what that server and its keys can reach. This page sets out the risk, the controls Levelrail provides today with a link to each one, the guides for running AI apps, and the gaps that remain.

## The risk in five lines

- An agent runs code. Anything it runs inside a container can read that container's environment.
- It holds API keys for models, git hosts and clouds. A leaked key is a leaked account.
- Its tools act on your apps. A token with deploy rights can ship a change, and one with write rights can change data.
- A mistake or a hostile prompt can make it call those tools in ways you did not intend.
- Other apps share the same server, so an agent that goes wrong reaches them unless you limit it.

## What Levelrail gives you today

- **HTTPS and per-domain protection.** Certificates are issued and renewed automatically by the embedded ingress. Each domain can also have a bot and abuse shield that limits request rate, concurrency and body size, and refuses bot user agents on a denylist. See [Security overview](/security#tls-and-network-exposure) and [Domain traffic controls](/domain-traffic-controls#bot-and-abuse-shield).
- **Secret masking.** Values that look like credentials are masked wherever they are read. Reading one back needs a fresh confirmation and a step-up. See [Environment variables and secrets](/environment-and-secrets).
- **Scoped agent tokens and AI control modes.** Give each agent its own token with an agent name and only the abilities it needs. The instance-wide AI control switch sets `off`, `observe`, `operate` or `admin`. In `operate` and `admin` an agent can never approve a deploy or change AI control, and it can only act in environment kinds you allow. The `admin` mode needs the `ai-control` experimental flag. See [AI control](/ai-control) and [Agents](/agents).
- **Template requirements before you deploy.** Every template page has a "Security and requirements" section that lists unpinned images, published host ports and volumes. `levelrail-cli templates show <id>` prints what a template needs: privileges, published ports, secrets and volumes. See [Templates and registry](/templates-and-registry).
- **Exposure report.** The Security center's Exposure tab shows what the internet can reach for every app and database, with its domains and published host ports. See [Security center](/security-center) and [Exposure audit](/exposure-audit).
- **Outbound allowlist per app.** An app can be restricted to the `host:port` pairs it needs. It is off by default. See [Deploying apps](/deploying-apps#outbound-network-egress-allowlist).
- **Backups with restore drills.** A drill restores a stored backup into a scratch resource, checks it, and destroys the scratch resource. See [Prove your backups work](/prove-your-backups-work).
- **Health-gated deploys and rollback.** Traffic moves only after the readiness probe passes. The previous release is held, prior images stay pinned as rollback targets, and a post-deploy watch can roll back automatically. See [Zero-downtime deploys](/zero-downtime-deploys).
- **Audit log.** Every mutating API call is recorded with who, what, when and the outcome, and it can be exported as CSV. See [Identity and access](/identity-and-access#audit-log).

## Self-host AI guides

These guides and template pages cover specific AI apps. The guides explain the whole setup. The template pages list the services, ports, volumes and generated secrets for each app.

| Page | What it covers |
| --- | --- |
| [Use your Mac mini or MacBook as a private LLM server](/guides/use-a-mac-as-a-private-llm-server) | Running Ollama, LM Studio or MLX natively on a Mac and serving it through the gateway with keys and limits. |
| [Use Ollama and Open WebUI with a Mac and Levelrail](/guides/use-ollama-open-webui-with-a-mac-and-levelrail) | Walkthrough: a native Ollama on a Mac, registered as an endpoint, with Open WebUI using a scoped key. |
| [Control Levelrail from OpenClaw](/guides/control-levelrail-from-openclaw) | Giving an OpenClaw agent a least-privilege token and the MCP config. |
| [OpenClaw on Docker Compose or on Levelrail](/guides/openclaw-on-docker-compose-vs-levelrail) | A factual comparison of running the OpenClaw template by hand and on Levelrail, with known limits. |
| [Self-host OpenClaw](/self-host/openclaw) | The OpenClaw template page. |
| [Self-host Ollama + Open WebUI](/self-host/ollama-open-webui) | The bundled Ollama and Open WebUI template. |
| [Self-host Open WebUI](/self-host/open-webui) | The Open WebUI template, with its own Ollama container. |
| [Self-host n8n](/self-host/n8n) | A visual, node-based workflow editor that connects your tools. |
| [Self-host Langflow](/self-host/langflow) | A visual low-code tool for multi-agent and RAG workflows. |
| [Self-host Flowise](/self-host/flowise) | A drag-and-drop builder for LLM chatbots, agents and RAG flows. |
| [Self-host Qdrant](/self-host/qdrant) | A vector similarity search engine for embeddings. |
| [Self-host LiteLLM Proxy](/self-host/litellm) | One OpenAI-format gateway in front of 100+ model providers, with keys and budgets. |

## What is not covered yet

- **Secret scanning is limited.** It checks the checked-out tree and plain env values at deploy time. It does not check git history or image layers. Image vulnerability scanning is optional and off by default. See [Security overview](/security#what-this-does-not-cover-yet) and [Threat model](/threat-model).
- **Outbound traffic is unrestricted until you set an allowlist.** DNS and loopback stay open even with one. There is also a brief window after a deploy or restart before the allowlist sidecar attaches. See [Deploying apps](/deploying-apps#outbound-network-egress-allowlist).
- **Hooks and health-check commands run shell by design.** Pre and post deploy hooks, health-check `exec` strings and pipeline `run` steps execute inside the container. Anyone who can edit an app's spec or pipeline, or who holds root, can run code there. See [Threat model](/threat-model).
- **AI control does not govern every caller.** It does not apply to people signed in with a session, or to API tokens without an agent name. Give every agent token an agent name so the switch can see it. See [AI control](/ai-control).
- **Per-resource permissions are limited.** Per-resource IAM covers apps, databases and models. Nodes, projects, domains, registries and settings are gated only by ability. See [Threat model](/threat-model).
- **The master key sits beside the database by default.** It is stored in the data directory unless `APP_MASTER_KEY` or `APP_MASTER_KEY_FILE` is set, so a copy of the whole data directory decrypts every secret. See [Threat model](/threat-model).
- **Container hardening has no per-app override.** Read-only root filesystems are opt-in. See [Security overview](/security#what-this-does-not-cover-yet).
- **The Docker API guard does not stop root code in the agent container.** The agent container holds the Docker socket, so a compromised agent is root on its node. See [Threat model](/threat-model).
- **Restore drills for most engines check the object, not a replay.** Only Postgres, MySQL and MariaDB drills replay the dump. See [Prove your backups work](/prove-your-backups-work).
- **Sessions are in memory.** A control plane restart, including an upgrade, signs everyone out. API tokens are not affected. See [Security overview](/security#what-this-does-not-cover-yet).
- **In-app assistant turns are not re-checked mid-turn.** A revoked caller can finish one in-flight turn. See [Threat model](/threat-model).

## Next steps

- Read the [Security overview](/security) and the [Threat model](/threat-model) before you give an agent a token.
- Set up the agent with a least-privilege token by following [Control Levelrail from OpenClaw](/guides/control-levelrail-from-openclaw).
