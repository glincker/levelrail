---
description: Run Ollama, vLLM or llama.cpp models on NVIDIA GPU nodes as a first-class resource, with an OpenAI-compatible endpoint protected by an API key.
---

# AI models

A model is a first-class resource: an inference engine running as a container on a GPU node, a persistent volume holding the downloaded weights, and an OpenAI-compatible endpoint protected by an API key. It is available from the dashboard (**AI models**), the CLI (`levelrail models`), the REST API (`/api/v1/models`) and the MCP server.

Version 1 supports NVIDIA GPUs on Linux nodes (Ubuntu is the documented path). AMD, Intel and Apple GPUs are not supported.

## GPU nodes

Every node reports its NVIDIA GPUs to the control plane: driver version, per-GPU VRAM total and used, utilization, and whether Docker has the `nvidia` container runtime registered. Detection runs `nvidia-smi` on the host and asks the Docker Engine API for its runtimes. The control plane detects its own host every minute (`APP_GPU_COLLECT_INTERVAL`), agents report on connect and every minute after.

The **AI models** page shows a card per GPU node with VRAM used and total. `levelrail models gpus` prints the same data.

### Prerequisites on the node (Ubuntu)

1. Install the NVIDIA driver: `sudo ubuntu-drivers install`, then reboot. `nvidia-smi` must work.
2. Add NVIDIA's apt repository for the container toolkit, then install and register it:

```sh
sudo apt-get install -y nvidia-container-toolkit
sudo nvidia-ctk runtime configure --runtime=docker
sudo systemctl restart docker
```

When a node has a GPU but Docker has no `nvidia` runtime, the dashboard card shows "nvidia runtime missing" with this fix, `levelrail doctor` and the Status page raise a warning, and models placed there stay in the `GPURuntimeMissing` state until it is fixed. Run `levelrail models restart <name>` afterwards.

If the agent runs inside a container, `nvidia-smi` must be reachable from it, or run the agent directly on the host.

## Deploying a model

```sh
levelrail models deploy --name chat --engine ollama --model llama3.1:8b
levelrail models deploy --name llama --engine vllm --model meta-llama/Llama-3.1-8B-Instruct \
  --gpus 2 --context 8192 --hf-token-from-env --node <node-id> --domain llm.example.com
```

| Option | Meaning |
| --- | --- |
| `--engine` | `ollama`, `vllm` or `llamacpp`. |
| `--model` | An Ollama tag (`llama3.1:8b`), a HuggingFace repo (`org/name`) for vLLM, or a GGUF repo for llama.cpp, optionally with a quant suffix (`org/name-GGUF:Q4_K_M`). |
| `--node` | Node ID. Default is the control plane's own host. |
| `--gpus`, `--gpu-devices` | A count, `all`, or specific GPU indexes or UUIDs. |
| `--context` | Maximum context length in tokens. |
| `--quantization` | vLLM only (for example `awq`). Ollama encodes it in the tag, llama.cpp in the `:quant` suffix. |
| `--domain` | Hostname for the endpoint. Without one, a zero-config hostname is used when `APP_PUBLIC_HOST` is a public IP. |
| `--hf-token-from-env` | Reads `HF_TOKEN` from your environment for gated models. |

The engine image can be overridden per engine with `APP_MODEL_IMAGE_OLLAMA`, `APP_MODEL_IMAGE_VLLM` and `APP_MODEL_IMAGE_LLAMACPP`.

### HuggingFace token

The token is stored through the envelope-encryption path (`internal/secrets`) under the model's own namespace and injected into the engine container at create time. It is never returned by the API and never logged. It requires a master key (`APP_MASTER_KEY`). Rotate it with `PUT /api/v1/models/{name}/hf-token`, which restarts the model.

## Lifecycle and status

A model has one `Ready` condition whose reason tells you where it is:

| Reason | Meaning |
| --- | --- |
| `Starting` | The container is being created or started. |
| `Downloading` | The weights are downloading. Ollama reports percent and bytes; vLLM and llama.cpp download on start, follow `levelrail models logs <name> --follow`. |
| `Loading` | The model is being loaded into VRAM. |
| `ModelLoaded` | The model is loaded and serving. This is the readiness signal. |
| `NoGPUOnNode`, `GPURuntimeMissing`, `InsufficientGPUs` | The node cannot serve the request. See GPU nodes above. |
| `HFTokenUnavailable` | A token is set but cannot be decrypted (no master key). |
| `EndpointUnreachable` | The node is remote and not on the WireGuard mesh, so the control plane cannot reach the engine. |
| `DownloadFailed` | The engine reported a download error (retried after 30 seconds). |

The downloaded weights live in a persistent Docker volume, so a restart or redeploy does not download again. Deleting a model removes its container but keeps the volume.

## The endpoint and API key

The model's hostname is routed through the built-in ingress to the control plane, whose gateway checks the API key and proxies to the engine. It speaks the OpenAI API:

```sh
curl https://llm.example.com/v1/chat/completions \
  -H "Authorization: Bearer lr-..." -H "Content-Type: application/json" \
  -d '{"model": "llama3.1:8b", "messages": [{"role": "user", "content": "hello"}]}'
```

- The key is generated at deploy time and shown once. Only its SHA-256 hash is stored. Lost it? `levelrail models rotate-key <name>` issues a new one and invalidates the old one.
- Only an allowlist of OpenAI-compatible routes is served (see below). Engine admin APIs (Ollama's pull and delete, vLLM's runtime LoRA load and unload, llama.cpp's `/props` and `/lora-adapters`) are never exposed, even ones that live under `/v1/`.
- The engine's port is published on loopback (local node) or the WireGuard mesh address (remote node), never on a public interface.

### Served routes

Matching is exact and case sensitive: trailing slashes, `//`, `..`, backslashes and encoded slashes (`%2F`) are rejected. Anything else returns a `404` OpenAI-style error before the API key is checked, and a wrong method returns `405` with an `Allow` header.

| Route | Method | Ollama | vLLM | llama.cpp |
| --- | --- | --- | --- | --- |
| `/v1/models`, `/v1/models/{id}` | GET | yes | yes | yes |
| `/v1/chat/completions` | POST | yes | yes | yes |
| `/v1/completions` | POST | yes | yes | yes |
| `/v1/embeddings` | POST | yes | yes | yes |
| `/v1/responses` | POST | yes | yes | yes |
| `/v1/audio/transcriptions`, `/v1/audio/translations` | POST | no | yes | no |

Not served on purpose: vLLM's `/v1/load_lora_adapter`, `/v1/unload_lora_adapter`, `/v1/chat/completions/batch` and the `/render` routes; llama.cpp's `/v1/chat/completions/control` and its Anthropic-style `/v1/messages`; stateful `GET` and cancel on `/v1/responses/{id}`.

### Limits

Limits are global (every model) and set with environment variables on the control plane. `0` or a negative value disables a limit. The current values show on `GET /api/v1/models/{name}` (`limits`) and in `levelrail models get`.

| Variable | Default | Effect |
| --- | --- | --- |
| `APP_MODEL_GATEWAY_MAX_BODY_BYTES` | `33554432` (32 MiB) | Larger request bodies get `413`. |
| `APP_MODEL_GATEWAY_MAX_N` | `16` | `n` or `best_of` above this gets `400`. |
| `APP_MODEL_GATEWAY_MAX_GEN_LEN` | `32768` | `max_tokens`, `max_completion_tokens` or `max_output_tokens` above this, or negative (unlimited on llama.cpp), gets `400`. A request that sets none is passed through; the engine's context length bounds it. |
| `APP_MODEL_GATEWAY_MAX_INFLIGHT` | `32` | Concurrent requests per model; more get `429` with `Retry-After`. Streams hold a slot until they end. |
| `APP_MODEL_GATEWAY_RETRY_AFTER` | `5s` | The `Retry-After` value. |
| `APP_MODEL_GATEWAY_DIAL_TIMEOUT` | `5s` | Connecting to the engine. |
| `APP_MODEL_GATEWAY_HEADER_TIMEOUT` | `5m` | Waiting for the engine's response headers. A non-streaming completion sends none until it finishes, so keep this above your longest generation. Exceeded: `504`. |
| `APP_MODEL_GATEWAY_IDLE_TIMEOUT` | `2m` | Longest gap between engine output on a response. It is a gap, not a total, so long streams that keep producing tokens are never cut. |

JSON bodies are read up to the size cap so `n` and the token limits can be checked; the body is then forwarded unchanged. Audio uploads are size-capped but not parsed. Request and response bodies are never logged: each request logs only method, model, status, duration and bytes.

## GPU apps

Any app can request a GPU with `resources.gpu` in `app.yaml` (see the [app spec reference](app-spec-reference.md)). Such an app only runs on a node with a usable GPU: the reconciler reports `NoGPUOnNode` or `GPURuntimeMissing` instead of starting it, and moving it to a node without one is rejected.

## GPU scheduling

Placement counts GPUs, not just workloads. Each node has a ledger built from the desired state: every app with `resources.gpu` and every model on the node reserves GPUs.

| Request | Reserves |
| --- | --- |
| a count, `gpus: 2` | that many GPUs, anonymous |
| device IDs (index or UUID) | those exact devices; a second workload asking for the same device does not fit |
| `all` or no count | every GPU, so the node must be completely free |

A workload fits a node when the node reports a GPU, Docker has the nvidia runtime, and enough GPUs are free (total minus reserved, never below zero). A node that is exactly full fits nothing more. A workload's own reservation is ignored when re-checking it against its own node.

Where the ledger is consulted:

- **Create.** A new GPU app without `node_id` is auto-placed on the least loaded node that fits (the local host is the fallback), or refused with `409` and a per-node reason. An explicit `node_id` is checked the same way once the node has reported.
- **Move.** `PUT /api/v1/apps/{name}/node` rejects a target that lacks the runtime or the free GPUs.
- **Drain.** Each GPU app is placed on a node that fits, and GPUs it takes are reserved for the apps after it in the same drain. An app no node can host stays put and is reported under `blocked` (dashboard drain dialog, `levelrail nodes drain`, API). Models are never moved, so they are always listed as blocked.

Docker does not isolate GPUs by count: two containers can share a device, so the ledger is scheduling accounting, not enforcement. A running app is never stopped because the ledger says the node is oversubscribed.

### Seeing reservations

- **API.** `GET /api/v1/gpus` adds `reserved_gpus`, `free_gpus` and `reservations` (`app:<name>`, `model:<name>`) per node. `GET /api/v1/nodes` and `GET /api/v1/nodes/{id}` carry a `gpu` summary.
- **CLI.** `levelrail nodes list` has a GPU column (`free/total`), `levelrail nodes get` prints a GPU block, `levelrail models gpus` has a RESERVED column.
- **Dashboard.** The AI models page GPU cards and the node detail GPU card show reserved vs total GPUs and VRAM used vs total; the node list shows a `GPU free/total` badge.
- **MCP.** `list_gpu_nodes` returns the same fields.

### Attention and doctor

When a GPU app or model cannot run on its own node and no eligible GPU node has enough free GPUs, `levelrail doctor` and the attention list (Status page, `levelrail attention`, MCP `get_attention`) raise a warning `GPU app <name> cannot be placed`. Fix it by freeing a GPU (stop or shrink another GPU workload), adding a GPU node, or installing the nvidia container toolkit on the node that has GPUs.

## Access control

Listing and reading models and GPUs needs the `read` ability. Deleting and restarting needs `write`. Deploying, rotating a key and setting a HuggingFace token need `write:sensitive`. Model resources can be targeted by IAM policies as `model:<name>`. The AI assistant asks for confirmation before any of the mutating model tools.

## Not in version 1

AMD and Apple GPUs, MIG partitioning, automatic model-to-node scheduling (you pick the node; apps are spread, see GPU scheduling), moving a model between nodes, per-key rate limits at the gateway (only the per-model concurrency cap exists), per-model limit overrides, and per-key usage accounting.

## GPU in Compose templates

A Compose file can request NVIDIA GPUs with the standard
`deploy.resources.reservations.devices` block. Levelrail reads `driver`
(only `nvidia` or unset), `count` (a number or `all`, unset means all),
`device_ids` and `capabilities: [gpu]`, and maps it onto `resources.gpu`.
The service then only starts on a node with a working NVIDIA runtime.
Catalogue entries that need a GPU carry a "Needs NVIDIA GPU" badge.
