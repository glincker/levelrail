---
title: OpenClaw on Docker Compose or on Levelrail
sidebarTitle: "OpenClaw: Compose vs Levelrail"
description: "A factual comparison of the OpenClaw template run with plain docker compose up on a VPS versus on Levelrail, with passed checks and known limits."
"og:url": "https://docs.levelrail.com/guides/openclaw-on-docker-compose-vs-levelrail"
---

OpenClaw is a self-hosted agent gateway. Its Levelrail template is one Compose service, so you can run the same file with `docker compose up` on a VPS or deploy it as a Levelrail app. This page compares the two. It is based on the template file and on one local dogfood run, and it says what was not tested.

## What was measured, and what was not

- The template is [`internal/catalog/contrib/openclaw.yaml`](https://github.com/glincker/levelrail/blob/main/internal/catalog/contrib/openclaw.yaml). It has one service, `openclaw-gateway`, on image `ghcr.io/openclaw/openclaw:2026.10.5`, with port 18789, a named volume, a generated gateway token and the upstream health check.
- The Levelrail column reports only checks that passed in a local dogfood run on 2026-10-19 (control plane in dev mode, local Docker on macOS, no remote server). Checks that were partial or failed are listed under [Known limits](#known-limits).
- We did **not** run plain `docker compose up` on a VPS. The plain column describes what the compose file does and what you would have to do yourself. It is not a measured comparison, and this page includes no benchmarks.

## Side by side

| Topic | Plain `docker compose up` on a VPS | Levelrail template |
| --- | --- | --- |
| Gateway token | `OPENCLAW_GATEWAY_TOKEN` is set from `$SERVICE_HEX_32_GATEWAYTOKEN`. Compose does not generate that value, so unless you set it the variable is empty. Levelrail generates `$SERVICE_` tokens at deploy time and stores them as secrets. See [Magic variables](/templates-and-registry#magic-variables). | Generated at deploy and stored as a secret. Passed: the token did not appear in the app JSON, `/env`, `/logs`, `/deploys`, `/audit-log`, `/exposure`, or container stdout. |
| Host port | `"18789:18789"` publishes on every host interface, which is Docker's default for a short port mapping. | Passed: the exposure mode is proxied, the bind address is private and the host port is bound to 127.0.0.1 only. |
| Health check | The compose `healthcheck` runs and shows in `docker compose ps`. `docker compose up` returns when containers start unless you pass `--wait`. | Passed: readiness uses the compose health check (exec readiness, 170 second ready timeout), and the deploy waits for it. |
| Secret visibility on the host | `docker inspect` on the host shows the token. | Same. The dogfood noted that `docker inspect` on the host shows the token, which is inherent to the container runtime. |
| Exposure and posture | You review the host firewall and published ports yourself. | Passed: the security center's exposure view showed no OpenClaw finding, and the posture is instance wide. See [Exposure audit](/exposure-audit). |
| Docker guard | Not applicable. | Passed: the guard reported no rule hit and no Docker socket mount for the template. The dogfood instance ran the guard in audit mode. See [Docker access and the API guard](/docker-access). |
| Agent access to the platform | Not applicable. | See the section below. |

The `openclaw-cli` helper service from upstream is not in the template. Levelrail refuses `network_mode: service:`, which that helper depends on, so it is run with `levelrail-cli apps exec` instead. See [Known limits](#known-limits).

## Levelrail from OpenClaw

The second half of the dogfood tested the agent side. Each row below is a check that passed.

- **Scoped token.** A token with the `read` and `deploy` abilities and the agent name `openclaw` was minted in a session. `whoami` showed those abilities.
- **AI control modes.** With the mode set to `off`, requests were refused with 403. With `observe`, reads were allowed and deploys refused. With `operate`, deploys were allowed.
- **Environment kind gate.** An app with no environment (kind `custom`) was refused until `custom` was allowed, then the deploy was accepted.
- **Guardrails stay with people.** The agent could not change AI control, revoke agent tokens, approve a deploy, change the Docker guard or use exec. Each attempt returned 403 with the documented message and was audited under the agent name `openclaw`.
- **MCP.** `levelrail-mcp --readonly` refused to start with a read and deploy token. The full tool list served 21 tools, and deploy and rollback calls succeeded. A scripted eval passed 13 of 13 tasks in demo mode.

For the setup steps, see [Control Levelrail from OpenClaw](/guides/control-levelrail-from-openclaw).

## Known limits

These are the checks that were partial or failed, and the gaps the dogfood ranked. The docs do not record fixes for them, so treat them as open unless a release note says otherwise.

**Partial or failed checks**

- **Resource limits: failed.** The template lists a recommended memory of 2 GiB. The container ran with no memory limit applied. The docs describe the recommendation as a hint that is not checked against a node's free memory (see [Templates and registry](/templates-and-registry)). On plain Compose, the file sets no memory limit either.
- **Domain and TLS: partial.** Adding a domain returns the go-live steps, and the domain check reported that it resolves elsewhere in the local run. Real certificate issuance was not testable locally. The shield was not enabled, because its update request needed a protection field.
- **Image signature: partial.** Image verification returns `not_configured` because there is no upstream public key. The template cannot claim a verified supply chain.
- **Volume backups: partial.** The backup policy API works. Triggering a backup needs a backup target, and none was configured locally.
- **Rollback: partial.** Rollback targets listed the pinned image. A rollback to the previous image restored a healthy container after a failed upgrade, but the downgrade path was not proven (see gap 1).

**Ranked gaps**

1. **Single-writer apps and the default deploy strategy.** OpenClaw's gateway takes an ownership lock on its state volume. The blue-green overlap start of 2026.10.4 exited with code 1 because of that lock. Compose templates cannot declare a deploy strategy, so `apps scale --strategy` is the only override. After `--strategy recreate`, the 2026.10.4 start still exited 1 while the rollback pin was running, so a version downgrade is unproven.
2. **Recommended memory is advisory only.** The value does not reach the container as a limit. An agent gateway with no limit can starve its host.
3. **The MCP tool list is not trimmed by ability.** A token with `read` and `deploy` is offered tools such as `set_app_env`, `set_app_domains` and `unset_app_env`, and receives 403 when it calls them.
4. **Rollback pins look like apps.** A pin appears in `apps list` as `<app>-dep<id>` with an image under `levelrail-rollback/`. It ran beside the live gateway during the test, and it can be deleted by mistake or hold a volume lock.
5. **`rollback_app` accepts any image string.** A bad image is only detected at pull time. A preflight check is missing.
6. **No upstream signature key.** See the image signature item above.
7. **Status for a compose member.** `apps status` printed "no conditions reported yet" for a healthy member. Low severity.
8. **`openclaw-cli` cannot be expressed.** Services that share another service's network namespace are refused. The helper is run with `levelrail-cli apps exec` instead.

**Not tested**

- Plain `docker compose up` on a VPS, including whether upstream's own compose file needs the `--allow-unconfigured` flag. The template passes that flag. In the dogfood, an upstream start on an empty volume exited with code 78 ("gateway.mode is unset"), and the template was changed to pass `--allow-unconfigured`. A plain compose file must include the same flag or configure the gateway first.
- Certificate issuance from a real public domain.
- Backups to a real target and a restore drill for this app.
- Downgrade across versions with the single-writer lock (gap 1).

## Related

- [Run AI agents safely on your own server](/guides/run-ai-agents-safely)
- [Self-host OpenClaw](/self-host/openclaw)
- [Control Levelrail from OpenClaw](/guides/control-levelrail-from-openclaw)
