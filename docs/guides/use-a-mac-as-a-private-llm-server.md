---
title: Use your Mac mini or MacBook as a private LLM server
description: "Run Ollama, LM Studio or an MLX server on a Mac, register it as an inference endpoint and serve it through Levelrail with scoped keys and limits."
"og:url": "https://docs.levelrail.com/guides/use-a-mac-as-a-private-llm-server"
---

A Mac with Apple silicon runs local models well, and it is probably already on your desk. Levelrail can put that model behind the same gateway as every other model: one OpenAI-compatible URL, a key per app, per-key limits, usage and audit. The Mac stays yours. Levelrail does not run anything on it.

## Why the model server must run natively

Apple's GPU is reached through Metal, and Docker on a Mac cannot use it. Docker Desktop runs containers inside a Linux virtual machine, and that machine has no access to the Apple GPU. A model in a container on a Mac falls back to the CPU and is far slower than the same model run natively. So Levelrail does not deploy models to a Mac. Run the model server directly on macOS, then register its address.

This feature adds no Mac node, no agent and no command execution on the Mac. The control plane only calls the server's HTTP API.

## What you need

- A model server on the Mac that speaks the OpenAI API or Ollama's API: [Ollama](https://ollama.com), [LM Studio](https://lmstudio.ai), or an MLX server such as `mlx_lm.server`.
- A private network path from the control plane to the Mac: the same LAN, or a VPN you manage (WireGuard, Tailscale or similar). The Mac does not need a public address and should not have one.
- An operator who enables internal destinations on the control plane (below).

## 1. Make the server reachable

By default Ollama listens on `127.0.0.1` only. Bind it to the Mac's network address:

```bash
OLLAMA_HOST=0.0.0.0:11434 ollama serve
```

On a Mac you start from the menu bar app, set the variable with `launchctl setenv OLLAMA_HOST 0.0.0.0:11434` and restart the app. LM Studio has a "serve on local network" switch in its server settings, and MLX takes `--host 0.0.0.0`. Bind to the VPN or LAN address instead of `0.0.0.0` if the Mac is on other networks too.

Pull a model on the Mac, for example `ollama pull qwen2.5:7b`.

Keep the Mac awake while it serves. A sleeping Mac stops answering, and Levelrail shows it as asleep or unreachable. Turn on "Prevent automatic sleeping on power adapter" in System Settings under Energy, or run `caffeinate -s` in a terminal.

## 2. Allow internal destinations

Inference endpoints are often private addresses, so Levelrail refuses them until an operator opts in. Set this on the control plane and restart it:

```bash
APP_MODEL_ENDPOINT_ALLOW_PRIVATE_NETWORKS=true
```

The older `APP_NOTIFY_ALLOW_PRIVATE_NETWORKS=true` also enables it, but it relaxes webhooks too, so prefer the specific variable. Link-local addresses (`169.254.0.0/16`, `fe80::/10`), cloud metadata addresses and the unspecified address are refused even with the opt-in. The check runs when the request is sent, on the address actually dialed, so a host name that later resolves to one of them is refused too.

## 3. Register the endpoint

In the dashboard, open AI models and choose Add endpoint. Enter the server address (for example `http://100.64.0.7:11434`), pick the API type, and use Test connection. The test calls the server's list-models route and shows health, latency and the models it found. A failed test shows what to change and a Re-check button.

From the CLI:

```bash
export MAC_KEY=...   # only if the server needs a key, otherwise drop --credential-from-env
levelrail-cli models endpoint test --url http://100.64.0.7:11434 --api ollama
levelrail-cli models endpoint add --name mac --url http://100.64.0.7:11434 --api ollama --credential-from-env MAC_KEY
```

Registering prints a gateway key once. A server that is off or unreachable still registers, and Levelrail keeps checking and shows when it comes online. A server key goes through `--credential-from-env MAC_KEY`, never a flag, and is stored encrypted in the secrets manager.

## 4. Use it like any other model

The endpoint has its own gateway URL and keys. Create one key per app with limits:

```bash
levelrail-cli models keys create mac --name notes-app --rpm 60 --tpm 50000 --tpd 1000000
```

Requests, tokens, errors and latency per key appear under Usage. Rate and daily token limits apply to requests served by the Mac, creating keys and changing routing are audited, and a scoped token can only reach the models its policy allows (`model:<name>`). The gateway forwards only the inference and listing routes, never an engine's admin API, and it never sends your gateway key upstream.

## Health states

| State | Meaning |
| --- | --- |
| Online | The list-models call succeeded within the slow threshold. |
| Slow | It succeeded but took longer than `APP_MODEL_ENDPOINT_SLOW` (default 1.5s). |
| Asleep or unreachable | Nothing answered: the Mac is asleep, off the network, or a firewall blocks the port. |
| Credential rejected | The server answered 401 or 403. Enter the key again. |
| Error | The server answered, but not like the chosen API type (wrong port, wrong API type, missing route). |

Endpoints are checked every 20 seconds (`APP_MODEL_ENDPOINT_HEALTH_INTERVAL`, `0` turns background checks off).

## Optional fallback

Off by default. When on, a request goes to another Levelrail model if the endpoint is unreachable, answers with a server error, or does not send response headers within a latency budget. The fallback can be a model on a GPU node, or a second endpoint pointing at a cloud API.

```bash
levelrail-cli models endpoint routing mac --fallback cloud-chat --fallback-model-id gpt-4o-mini --latency-budget 3s
levelrail-cli models endpoint routing mac --off
```

The fallback model id replaces the `model` field in the request, because the Mac's `qwen2.5:7b` means nothing to a cloud server. Responses served by the fallback carry an `X-Fallback-From` header and are counted on the endpoint. Usage still lands on the key the app used. The next request tries the Mac first again as soon as a health check sees it online.

## Limits

- Levelrail does not start, stop, update or wake the Mac's server, and has no on-demand residency for it.
- It does not place models on the Mac or know its memory. Pick models the Mac can hold.
- Latency to a Mac over a VPN is whatever the VPN gives you. The budget measures time to the first response bytes, so a streaming reply that starts in time but then stalls is not retried.

See [AI models](/ai-models) for GPU nodes, keys and usage.
