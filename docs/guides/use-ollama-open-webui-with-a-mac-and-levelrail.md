---
title: Use Ollama and Open WebUI with a Mac and Levelrail
sidebarTitle: Ollama + Open WebUI with a Mac
description: "Run Ollama natively on a Mac, register it as a Levelrail endpoint, and point Open WebUI at the gateway with a scoped key that has limits and expiry."
"og:url": "https://docs.levelrail.com/guides/use-ollama-open-webui-with-a-mac-and-levelrail"
---

This walkthrough combines two guides: [Use your Mac as a private LLM server](/guides/use-a-mac-as-a-private-llm-server) and [Self-host Ollama + Open WebUI](/self-host/ollama-open-webui). Ollama runs natively on the Mac, where it can use the Apple GPU. Levelrail registers it as an endpoint and serves it through its gateway. Open WebUI runs on your server and calls the gateway with a key that only it holds.

Levelrail does not run anything on the Mac. You will run Ollama yourself and keep the Mac awake.

## Before you start

- A Mac with Apple silicon, Ollama installed, and a model pulled.
- A private network path from the control plane to the Mac, such as the same LAN or a VPN you manage. The Mac should not have a public address.
- An operator who sets `APP_MODEL_ENDPOINT_ALLOW_PRIVATE_NETWORKS=true` on the control plane and restarts it. Private endpoints are refused without it.
- A Levelrail CLI signed in with `levelrail-cli auth login`.

## 1. Start Ollama on the Mac

Bind Ollama to an address the control plane can reach, not only to 127.0.0.1. Bind to the VPN or LAN address if you can:

```bash
OLLAMA_HOST=0.0.0.0:11434 ollama serve
```

In another terminal, pull a model and keep the Mac awake while it serves:

```bash
ollama pull qwen2.5:7b
caffeinate -s
```

Ollama's own commands are outside Levelrail. For the menu bar app and the power settings, see the [Mac guide](/guides/use-a-mac-as-a-private-llm-server#1-make-the-server-reachable).

## 2. Test the server from the CLI

Replace the address with the Mac's VPN or LAN address:

```bash
levelrail-cli models endpoint test --url http://100.64.0.7:11434 --api ollama
```

The test calls the server's list-models route and reports health, latency and the models it found. No key is needed for a default Ollama.

## 3. Register the Mac as an endpoint

Choose a hostname you control for the gateway, so Open WebUI can reach it from the server:

```bash
levelrail-cli models endpoint add --name mac --url http://100.64.0.7:11434 --api ollama --domain llm.example.com
```

Registration prints a default gateway key once. Do not give that key to Open WebUI, because it has no limits or allow lists. Create a scoped key in the next step instead.

Check the endpoint and copy its OpenAI-compatible base URL:

```bash
levelrail-cli models get mac
```

## 4. Create a scoped key for Open WebUI

This key can only call the two routes Open WebUI needs for chat, can only use the one model, has request and token limits, and expires after 30 days:

```bash
levelrail-cli models keys create mac --name open-webui \
  --rpm 60 --tpm 50000 --tpd 1000000 --max-parallel 4 \
  --allow-paths /v1/models,/v1/chat/completions \
  --allow-models qwen2.5:7b \
  --expires-in 720h
```

The key is printed once. Save it in your password manager. Limits are enforced per key: requests over `--rpm` or `--max-parallel` get `429`, and `--tpm` and `--tpd` count tokens after each response finishes.

If a feature in Open WebUI, such as embeddings for document search, calls another route, the key refuses it. Add that route to `--allow-paths` only if you want the feature.

## 5. Deploy Open WebUI

```bash
levelrail-cli templates show open-webui
levelrail-cli templates deploy open-webui --name open-webui
levelrail-cli apps domains add open-webui chat.example.com
```

`templates show` prints what the template needs (privileges, published ports, secrets and volumes) before you deploy it. The template also starts its own Ollama container, with `OLLAMA_BASE_URL` pointing at it. This walkthrough does not use that container, so it stays idle and uses memory.

## 6. Connect Open WebUI to the gateway

Open the Open WebUI domain, sign in, and open the admin settings. Under Connections, add an OpenAI-compatible connection:

- **Base URL:** the base URL from `levelrail-cli models get mac`. It ends in `/v1`.
- **API key:** the scoped `open-webui` key from step 4.

The labels in that screen belong to Open WebUI, not Levelrail. Check them against the Open WebUI version you run. Then open a new chat and choose `qwen2.5:7b`.

## 7. Check the traffic

```bash
levelrail-cli models keys list mac
levelrail-cli models usage mac --since 1h
```

The key list shows last used time, and the usage report shows requests, tokens, errors and latency for that key. A request from Open WebUI should show up under the `open-webui` key and nowhere else.

## 8. Rotate or revoke the key

Rotate the key when it has been shared or is due. The old key keeps working for the grace window, then stops:

```bash
levelrail-cli models keys rotate mac <key-id> --grace 30m
```

Revoke a key at once when it is no longer needed:

```bash
levelrail-cli models keys revoke mac <key-id>
```

## Limits

- If the Mac sleeps or leaves the network, requests fail, and Levelrail shows the endpoint as asleep or unreachable. It keeps checking and recovers when the Mac answers.
- Levelrail does not start, stop or update Ollama, and it does not know the Mac's memory. Pick a model the Mac can hold.
- A streaming reply that starts and then stalls is not retried. See the [Mac guide](/guides/use-a-mac-as-a-private-llm-server#limits).

## Related

- [Run AI agents safely on your own server](/guides/run-ai-agents-safely)
- [AI models](/ai-models#the-endpoint-and-api-key)
- [Virtual keys and usage](/ai-models#virtual-keys-and-usage)
