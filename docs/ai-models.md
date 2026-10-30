---
title: "AI models"
tag: "Experimental"
description: "Run Ollama, vLLM or llama.cpp models on NVIDIA GPU nodes, or small GGUF models on CPU with hard limits, behind an OpenAI-compatible endpoint."
"og:url": "https://docs.levelrail.com/ai-models"
---

A model is a first-class resource: an inference engine running as a container on a GPU node, a persistent volume holding the downloaded weights, and an OpenAI-compatible endpoint protected by an API key. You can manage it from the dashboard (**AI models**), the CLI (`levelrail-cli models`), the REST API (`/api/v1/models`) and the MCP server.

Version 1 supports NVIDIA GPUs on Linux nodes (Ubuntu is the documented path). AMD, Intel and Apple GPUs are not supported.

## GPU nodes

Every node reports its NVIDIA GPUs to the control plane: driver version, per-GPU VRAM total and used, utilization, and whether Docker has the `nvidia` container runtime registered. Detection runs `nvidia-smi` on the host and asks the Docker Engine API for its runtimes. The control plane checks its own host every minute (`APP_GPU_COLLECT_INTERVAL`). Agents report when they connect and every minute after.

The **AI models** page shows a card per GPU node with VRAM used and total. `levelrail-cli models gpus` prints the same data.

### Prerequisites on the node (Ubuntu)

1. Install the NVIDIA driver: `sudo ubuntu-drivers install`, then reboot. `nvidia-smi` must work.
2. Add NVIDIA's apt repository for the container toolkit, then install and register it:

```sh
sudo apt-get install -y nvidia-container-toolkit
sudo nvidia-ctk runtime configure --runtime=docker
sudo systemctl restart docker
```

When a node has a GPU but Docker has no `nvidia` runtime, the dashboard card shows "nvidia runtime missing" with this fix, `levelrail-cli doctor` and the [Status page](/status-page) raise a warning, and models placed there stay in the `GPURuntimeMissing` state until it is fixed. Run `levelrail-cli models restart <name>` afterwards.

If the agent runs inside a container, `nvidia-smi` must be reachable from it, or run the agent directly on the host.

## GPU attach: CDI and legacy

Containers get GPUs one of two ways. Levelrail prefers CDI (the Container Device Interface) when the Docker daemon on the node lists NVIDIA CDI devices (`nvidia.com/gpu=0`, `nvidia.com/gpu=all`), and otherwise uses the legacy `nvidia` runtime with a device request. The choice is made per container at create time on the node that runs it, so mixed fleets work. A node with CDI devices needs no `nvidia` runtime registered and is treated as usable.

To use CDI, generate a spec on the node and make sure Docker discovers it (Docker 28.3 or newer does by default; older Docker needs `{"features": {"cdi": true}}` in `/etc/docker/daemon.json`):

```sh
sudo nvidia-ctk cdi generate --output=/etc/cdi/nvidia.yaml
sudo systemctl restart docker
docker info | grep -A3 "Discovered Devices"
```

A request that CDI cannot serve exactly (for example more GPUs than the spec lists) falls back to the legacy runtime instead of failing. With a count, CDI attaches the first N GPUs by index. Set `APP_GPU_ATTACH=legacy` on a node to never use CDI (the default is `auto`). Regenerate the spec after a driver update or GPU change.

`levelrail-cli doctor` (and the Doctor page) checks the control plane host in detail: the driver, per-GPU memory (a warning above `APP_GPU_DOCTOR_VRAM_WARN_PERCENT`, default 90), the NVIDIA container toolkit version, whether a CDI spec exists and Docker lists its devices, whether the `nvidia` runtime is registered, and the resulting attach mode, with the exact fix command for each problem. Remote nodes report only driver, runtime or CDI availability and per-GPU memory; the toolkit and spec files of a remote node are not inspected. None of this was exercised against a real GPU: the detection and the CDI request are covered with fakes.

## Deploying a model

```sh
levelrail-cli models deploy --name chat --engine ollama --model llama3.1:8b
levelrail-cli models deploy --name llama --engine vllm --model meta-llama/Llama-3.1-8B-Instruct \
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

## Hugging Face preflight

Before deploying a Hugging Face repository, check it:

```sh
levelrail-cli models preflight bartowski/Llama-3.2-3B-Instruct-GGUF --engine llamacpp --node <node-id>
HF_TOKEN=hf_... levelrail-cli models preflight meta-llama/Llama-3.1-8B-Instruct --engine vllm --hf-token-from-env
```

The check queries the public Hub API (through the same outbound guard as webhooks, with a timeout and a response size cap) and reports:

- whether the repository exists, and whether it is gated. A gated repository says what to do next: accept the license on huggingface.co and provide an access token.
- the license, total download size and per-file sizes.
- the GGUF quantizations it contains, with sizes, and a recommended one for the target node.
- whether the node has enough free disk for the download, and which engine can load the weights (GGUF for llama.cpp and Ollama, safetensors for vLLM).

In the dashboard the same panel appears under the model field of the deploy dialog and updates as you type. Picking a quantization there fills in the `:quant` suffix.

Fit figures are estimates. A quantization is judged against the node's free VRAM as its file size plus a flat overhead for the KV cache and buffers. Real usage depends on context length, batch size, engine and driver. Fit is shown only when the node has reported its GPU, and disk only when it has reported its disk; otherwise the answer is "unknown", never a guess. System RAM is not considered.

The token you supply is used for that one request. It is not stored or logged, and it is never part of a cache key.

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_HF_CACHE_TTL` | `5m` | How long Hub answers are reused. `0` disables the cache. |
| `APP_HF_TIMEOUT` | `10s` | Per-request timeout to the Hub. |
| `APP_HF_MAX_RESPONSE_BYTES` | `16777216` | Cap on the repository listing. |
| `APP_HF_MAX_LISTED_FILES` | `100` | Files returned per repository (totals always cover all files). |
| `APP_HF_BASE_URL` | `https://huggingface.co` | Hub base URL (a mirror). |
| `APP_MODEL_FIT_OVERHEAD_PERCENT` | `20` | Added to weight size for KV cache and buffers. |
| `APP_MODEL_FIT_PERCENT` | `90` | Share of free VRAM a model may use to count as "fits". Above it, up to 100%, is "tight". |
| `APP_MODEL_DISK_HEADROOM_PERCENT` | `10` | Free disk required beyond the download. |

The disk check measures the filesystem holding Docker's volumes on the control plane host. If that path is not visible from the control plane, or the node is remote, disk is reported as unknown.

The API is `POST /api/v1/models/preflight` (`repo`, optional `engine`, `quant`, `file`, `gpu_count`, `node_id`, `hf_token`) and the MCP tool is `preflight_model`. Hub problems (missing, gated, rate limited, unreachable) come back as a `status` in a normal response so a client can always show the next step.

## VRAM fit check

Before you deploy, and on the model page, Levelrail estimates whether a model fits in each GPU node's free VRAM and rates every node: `fits`, `tight`, `wont_fit` or `unknown`. The pill has an info tip with the arithmetic, for example `weights 4.7 GiB + KV 1.2 GiB + overhead 0.8 GiB = 6.7 GiB of 8.0 GiB free`.

This is an estimate, not a guarantee. It is built from these pieces:

- Weights: Exact when the Hugging Face file sizes are known (pass `weights_bytes` from a preflight), otherwise estimated from the parameter count in the model name (`8b`, `8x7b`) and the quantization (a tag suffix such as `q4_K_M`, a `:quant` suffix, or vLLM's `--quantization`). A name with no size, such as `mistral`, is `unknown`, never guessed. Ollama and llama.cpp default to about 4.85 bits per weight, vLLM to 16 (bf16).
- KV cache: Approximated from the parameter count and the context length. With no context length set, `APP_MODEL_FIT_DEFAULT_CONTEXT` tokens (default 8192) are assumed and the tip says so.
- Overhead: A flat `APP_MODEL_FIT_GPU_OVERHEAD_MIB` per GPU (default 512), plus `APP_MODEL_FIT_VLLM_EXTRA_MIB` (default 1024) for vLLM.
- Free VRAM: From the node's GPU report (refreshed every minute). VRAM estimated for other models placed on the node that are not loaded yet is subtracted, so two deploys in the same minute do not both see the same free memory. On the model page, a loaded model's own VRAM is added back.
- Verdict: `fits` when the total is at most `APP_MODEL_FIT_PERCENT` of free VRAM (default 90), `tight` up to 100%, otherwise `wont_fit`. vLLM also needs each GPU to have `APP_MODEL_VLLM_GPU_UTILIZATION` (default 0.9) of its memory free, because it pre-allocates that much.

Nodes are ranked best first, with non-schedulable nodes last. The check is advisory: it never blocks a deploy. `APP_MODEL_FIT_KV_KIB_PER_TOKEN` overrides the KV heuristic if your model family differs.

- Dashboard: The deploy dialog shows a verdict per node and highlights the selected one. The model page Overview shows the same for the deployed model, with its current node marked.
- CLI: `levelrail-cli models fit --engine ollama --model llama3.1:8b --context 8192`, or `levelrail-cli models fit --name chat` for a deployed model.
- API: `POST /api/v1/models/fit` and `GET /api/v1/models/{name}/fit` (read ability).
- MCP: `check_model_fit` (read-only).

No GPU was involved in testing this; the numbers come from the GPU facts nodes report and have not been compared against real engine memory use.

## Model cache

Each model keeps its downloaded weights in its own Docker volume. Deleting a model keeps the volume, so those volumes are what accumulates.

```sh
levelrail-cli models cache list
levelrail-cli models cache prune --dry-run
levelrail-cli models cache prune
```

The list shows each volume with its size, its owning model, when it was last used, and whether it is safe to prune. Last use comes from gateway traffic when the model has any, otherwise from the model's last change (or, for a volume no model owns, the volume's creation time). Two models that point at the same weights are flagged and counted once in the unique total.

Prune removes only volumes that no model owns, that no container mounts, and that have been unused for `APP_MODEL_CACHE_UNUSED_DAYS` days (default 30). Weights of any deployed model, running or not, are never removed. The server rechecks each volume right before removing it. In the dashboard, the Model cache section shows a dry run in a confirmation dialog first. Pruning needs the root ability.

Only the control plane's own host is inspected today; remote nodes are listed as not yet supported. The MCP tool `list_model_cache` is read-only; the API is `GET /api/v1/model-cache` and `POST /api/v1/model-cache/prune`.

## CPU models

A small model can run on the server's CPU, with no GPU. It is the same resource as a GPU model (endpoint, keys, residency, usage), with `compute: cpu`, hard resource limits, a curated list and a download that Levelrail performs itself. Use it for short tasks on a modest server, not for chat at GPU speed.

```sh
levelrail-cli models catalog
levelrail-cli models fit --compute cpu --catalog granite-4.1-3b
levelrail-cli models deploy --compute cpu --catalog granite-4.1-3b --cpus 2 --memory-mib 4096
levelrail-cli models benchmark granite-4-1-3b
```

In the dashboard, **Deploy model** has a **Run on CPU** option: the curated models as cards (size, memory, licence, tier, estimated speed), CPU and memory sliders with the verdict updating as you move them, a plain warning of what it will use, a download progress bar, and a **Benchmark** button on the model page.

### What it runs

llama.cpp's official CPU server image (`ghcr.io/ggml-org/llama.cpp:server`). Override it with `APP_MODEL_IMAGE_LLAMACPP_CPU` (the GPU image stays `APP_MODEL_IMAGE_LLAMACPP`). Only llama.cpp supports CPU mode today; Ollama and vLLM models stay GPU only. The command has no `-ngl`, and these flags come from the limits:

| Flag | Value |
| --- | --- |
| `-t`, `-tb` | the whole CPUs of the cap (at least 1) |
| `-np` | `1`: one request at a time, so memory use stays predictable |
| `-c` | the context size of the resource (default `APP_MODEL_CPU_DEFAULT_CONTEXT`, 4096) |
| `-cram` | `APP_MODEL_CPU_CACHE_PERCENT` (5) of the memory cap, between 64 MiB and `APP_MODEL_CPU_CACHE_MAX_MIB` (512). llama.cpp's own default is 8192 MiB, which would eat a small server |
| `--no-mmap` | only when the cap leaves less than `APP_MODEL_CPU_MMAP_HEADROOM_MIB` (1024) beyond the weights, or when `APP_MODEL_CPU_MMAP=off` |

Memory mapping is the default: the kernel can drop weight pages when the whole host is short, and loading needs no second copy. With a cap barely above the weights, mapped pages would thrash the disk and slow every app sharing it, so the weights are read into memory instead and a too-small cap shows up as a clear out-of-memory kill. `APP_MODEL_CPU_MMAP=on` or `off` forces either.

### Limits

Every CPU model container is created with hard limits, so it cannot starve the apps next to it:

| Limit | Value |
| --- | --- |
| CPU | `NanoCPUs` from the cpu cap |
| Memory | the memory cap, with swap equal to memory (no swap) |
| CPU weight | `APP_MODEL_CPU_SHARES` (128, against Docker's 1024), so apps win under contention |
| OOM score | `APP_MODEL_CPU_OOM_SCORE_ADJ` (800): the kernel kills the model before app containers |
| Processes | `APP_MODEL_CPU_PIDS_LIMIT` (256) |

The caps come from the resource. Left empty they default to the smaller of 2 CPUs and half the node's CPUs (`APP_MODEL_CPU_DEFAULT_CPUS` overrides the 2), and `APP_MODEL_CPU_DEFAULT_MEMORY_PERCENT` (25) of the node's RAM. A request is refused above `APP_MODEL_CPU_MAX_CPU_PERCENT` (75) of the CPUs or `APP_MODEL_CPU_MAX_MEMORY_PERCENT` (60) of the RAM, or when the cap would leave less than `APP_MODEL_CPU_FREE_RAM_FLOOR_MIB` (1024) of the node's RAM. Only the control plane's own host reports its CPUs and RAM; for a remote node you must pass both caps. The CPU weight, OOM score and pids limit are applied on the local node only; they are not carried over the agent transport yet, the CPU and memory caps are.

### Preflight

`levelrail-cli models fit --compute cpu`, `POST /api/v1/models/fit` with `compute: cpu`, and the live panel in the deploy dialog return:

- Memory: weights plus KV cache at the chosen context plus overhead, against the cap.
- Disk: the weights download, with a free disk check.
- Speed: a range of tokens per second, always labelled an estimate. Generation is limited by memory bandwidth, so the range scales with the CPUs you allow (`APP_MODEL_CPU_BW_GBS_PER_CPU`, `APP_MODEL_CPU_BW_GBS_MAX`).
- A verdict, with the reason: `fits_comfortably` (at most `APP_MODEL_CPU_COMFORT_PERCENT` of the cap, 85), `tight`, `wont_fit` or `unknown` (the size cannot be read, never guessed), and a smaller context or model to try.

Deploying a model that will not fit is refused. A tight one is allowed with the warning.

### Curated list

The list is data (`internal/models/catalog_local.json`, embedded in the binary), not code. Only Apache-2.0 and MIT models are listed; the loader rejects any other licence. Sizes, licences, repositories and sha256 digests were read from the Hugging Face API on the date shown in the catalog. Levelrail never ships or mirrors weights: your server downloads them from Hugging Face when you deploy.

| Id | Tier | Download | Licence |
| --- | --- | --- | --- |
| `granite-4.0-1b` | light | 1.0 GB | Apache-2.0 |
| `qwen3.5-2b` | light | 1.3 GB | Apache-2.0 |
| `nomic-embed-text-v1.5` (embeddings) | light | 0.1 GB | Apache-2.0 |
| `granite-4.1-3b` | default | 2.1 GB | Apache-2.0 |
| `qwen3.5-4b` | plus | 2.7 GB | Apache-2.0 |
| `gemma-4-e4b` | plus | 5.0 GB | Apache-2.0 |

The list is provisional: the entries were checked against the Hub, not run on a CPU server here. Qwen3.5 thinks before answering by default, which is slow on CPU; Granite 4.1 does not.

Any other repository goes through the custom repository field (`--model org/name-GGUF:Q4_K_M`, plus `--weights-file` if the suffix does not pick one file). It shows its licence as read from the Hub with a notice that Levelrail did not review it, and the deploy is refused until you acknowledge it (`--accept-license`, or the checkbox). Gemma 3 and older, and Llama licences, are reachable only this way.

### Download

For a CPU model on the control plane's own host, Levelrail downloads the GGUF file itself into `$APP_DATA_DIR/model-weights/<name>/` (`APP_MODEL_WEIGHTS_DIR` overrides it) and mounts it read-only into the engine. This assumes the control plane runs as a process on the host, so the directory is visible to Docker; a control plane running inside a container needs `APP_MODEL_WEIGHTS_DIR` pointing at a path that is the same on the host. The weights are never baked into an image.

- Only `huggingface.co`, `hf.co` and their subdomains are allowed, over HTTPS, including every redirect (Hugging Face serves files from a CDN). Internal addresses are refused by the outbound guard.
- Interrupted downloads resume with an HTTP `Range` request. A server that ignores `Range` restarts from zero.
- The sha256 is verified: from the catalog, else from the digest the Hub reports. A mismatch discards the file and fails with `checksum_mismatch`.
- Limits: `APP_MODEL_DOWNLOAD_MAX_BYTES` (16 GiB), free disk plus `APP_MODEL_DISK_HEADROOM_PERCENT`.
- Progress (bytes done and total) is in the resource status: the model page, `models get` and `GET /api/v1/models/{name}` (`download`).

Failures are stated plainly in the status: `gated` (accept the licence on huggingface.co and set a token), `not_found`, `disk_full` (the partial file is kept), `interrupted` (resumes), `checksum_mismatch`, `too_large`, `host_not_allowed`. If the download finishes but the engine fails to start, the weights stay on disk and the next attempt reuses them. Deleting the model keeps the weights, like the GPU volume. On a remote node the engine pulls the weights itself and there is no progress; Ollama progress is not exposed either.

### Benchmark

`POST /api/v1/models/{name}/benchmark`, `levelrail-cli models benchmark <name>` or the **Benchmark** button runs three short fixed prompts through the running server and stores the last result: generation and prompt tokens per second, time to first token, and peak memory read from the container. It has a hard time cap (`APP_MODEL_BENCH_TIMEOUT_SECONDS`, 120), allows one run at a time, is rate limited per model (`APP_MODEL_BENCH_MIN_INTERVAL_SECONDS`, 300; the API answers `429` with `Retry-After`), never wakes a stopped model, and never runs on its own. The dashboard shows the measured speed next to the estimate.

### Idle unload

Use `--residency on_demand` (the dashboard default for CPU models). After the idle time (`--idle-ttl`, `APP_MODEL_IDLE_TTL`) the engine stops and frees its memory and CPU; weights stay on disk, so the next request only waits for the model to load again.

### Estimated and unverified

Everything here was tested against fakes and unit tests: flag and limit generation, verdicts, the downloader against a local HTTP server, the controller against a fake Docker client. No container was started and no model was run. The tokens per second range, the memory needed for each model and the quality of each model on a CPU are estimates. The model names postdate what was verified beyond the Hub listing. Run the benchmark on your server before relying on any number.

## Lifecycle and status

A model has one `Ready` condition whose reason tells you where it is:

| Reason | Meaning |
| --- | --- |
| `Starting` | The container is being created or started. |
| `Downloading` | The weights are downloading. Ollama reports percent and bytes; vLLM and llama.cpp download on start, follow `levelrail-cli models logs <name> --follow`. |
| `Loading` | The model is being loaded into VRAM (or memory, for a CPU model). |
| `DownloadFailed` | A CPU model's weights download failed. The message says why; retryable failures resume on their own. |
| `CPULimitsMissing` | A CPU model has no cpu or memory cap. Redeploy it with limits. |
| `ModelLoaded` | The model is loaded and serving. This is the readiness signal. |
| `Idle` | An on-demand model whose engine was stopped after its idle time. It starts on the first request (see On-demand residency). |
| `WakingUp` | An on-demand model's engine is starting or loading after a wake. |
| `WaitingForGPU` | An on-demand model would fit the GPUs but other workloads hold too much VRAM right now; it starts when they free it. |
| `NoGPUOnNode`, `GPURuntimeMissing`, `InsufficientGPUs` | The node cannot serve the request. See GPU nodes above. |
| `HFTokenUnavailable` | A token is set but cannot be decrypted (no master key). |
| `EndpointUnreachable` | The node is remote and not on the WireGuard mesh, so the control plane cannot reach the engine. |
| `DownloadFailed` | The engine reported a download error (retried after 30 seconds). |

The downloaded weights live in a persistent Docker volume, so a restart or redeploy does not download again. Deleting a model removes its container but keeps the volume.

## The model page

Each model has its own page at `/models/<name>` (click its name in the list). Tabs:

- Overview: Status, engine, node, GPU, context length, quantization, the endpoint URL with a copy button, and a ready-made `curl` example.
- Keys: the named keys with their limits, expiry and rotation (see Virtual keys below).
- Usage: Requests, tokens, errors and first-byte latency per key.
- Logs: The engine's live log tail.

The tab is kept in the URL (`?tab=keys`), so a link opens straight to it. From the CLI, `levelrail-cli models get <name>` prints the same overview facts.

## Engine metrics

The Overview tab shows the inference engine's own metrics with a health summary. The control plane scrapes each running engine every `APP_MODEL_ENGINE_METRICS_INTERVAL` (default 15s) over the same path its readiness probe uses, and stores the readings in the telemetry database under `model:<name>`.

| Metric | vLLM | llama.cpp | Ollama |
| --- | --- | --- | --- |
| KV cache usage | yes | yes | not available |
| Queued and running requests | yes | yes | not available |
| Prefix cache hit rate | yes | not available | not available |
| Tokens per second | yes | yes | not available |
| Time to first token | yes | not available | not available |
| VRAM in use, share running on CPU | not available | not available | yes (from `/api/ps`) |

Rates (tokens per second, prefix hit rate, time to first token) are computed between two scrapes, so they appear one interval after the model starts and skip an engine restart. llama.cpp serves `/metrics` only when started with `--metrics`, which new containers now get; a model deployed earlier needs `levelrail-cli models restart <name>`. Ollama exposes no Prometheus metrics, so the page says "Not available for ollama" instead of guessing; per-key tokens and first-byte latency are on the Usage tab.

The health pill warns when the KV cache is at or above `APP_MODEL_KV_WARN_PERCENT` (default 90), when more than `APP_MODEL_QUEUE_WARN` requests are queued (default 4), or when an Ollama model runs partly on CPU.

- CLI: `levelrail-cli models metrics <name> --since 6h`.
- API: `GET /api/v1/models/{name}/engine-metrics?since=1h` (read ability).
- MCP: `get_model_engine_metrics` (read-only).

The scrape is done by the control plane, not by a node agent, so a remote node's engine must be reachable over the mesh (the same requirement as serving it). Numbers depend on the engine version's metric names; vLLM's older `gpu_cache_usage_perc` and current `kv_cache_usage_perc` are both read.

## On-demand residency

By default a model is always loaded. On a shared GPU you can make it on demand: after an idle period the engine container is stopped, freeing its VRAM, and the first request starts it again.

```sh
levelrail-cli models deploy --name chat --engine ollama --model llama3.1:8b --residency on_demand --idle-ttl 20m
levelrail-cli models residency chat --mode on_demand --idle-ttl 30m
levelrail-cli models wake chat      # start now
levelrail-cli models sleep chat     # stop now
```

How it behaves:

- Idle: Every authenticated gateway request marks the model used (a long streaming response keeps refreshing it). When nothing has used it for the idle TTL, the reconciler stops the container and the status reason becomes `Idle`. The TTL is per model (`--idle-ttl`), defaulting to `APP_MODEL_IDLE_TTL` (15m). It cannot be set below `APP_MODEL_MIN_IDLE_TTL` (1m), so a busy model is never judged idle between activity writes (`APP_MODEL_ACTIVITY_TOUCH_INTERVAL`, default 10s).
- Wake: The first authenticated request to an idle model asks the reconciler to start it and is held while the engine loads, for at most `APP_MODEL_WAKE_WAIT` (default 90s). If the engine is not serving by then, or the wait is 0, the client gets `503` with `Retry-After` (`APP_MODEL_WAKE_RETRY_AFTER`, default 15s) and error code `model_waking`; the wake keeps going, so a retry usually succeeds. Unauthenticated or invalid requests never wake a model.
- Ollama: Ollama's own keep-alive is set to forever so weights stay in VRAM; unloading it means stopping the container, which is what on-demand does.
- GPU contention: If a wake would fit an empty GPU set but other workloads hold too much VRAM (per the fit estimate above), the model reports `WaitingForGPU` and starts when memory frees. A model too large for the GPUs is not held back, since waiting would not help. The GPU snapshot refreshes every minute, so this can lag.
- State: The reconciler owns the asleep, waking and awake states. Stop and start are idempotent: a stop that succeeded but failed to record is repaired on the next pass, and a request that lands while the reconciler is deciding to stop wins (the row is re-read right before stopping).
- Dashboard: the model page Overview has a Residency card (mode, idle minutes, Wake now, Sleep now) and the deploy dialog has a Residency field. API: `PUT /api/v1/models/{name}/residency`, `POST /api/v1/models/{name}/wake`, `POST /api/v1/models/{name}/sleep` (write ability); the model resource carries `residency`, `idle_ttl_seconds`, `effective_idle_ttl_seconds`, `residency_state` and `last_active_at`. MCP: `get_model` returns the same fields and `deploy_model` accepts `residency` and `idle_ttl_seconds`.

Swap groups (several models sharing one GPU, only one loaded at a time) are not built; give each model on a shared GPU an idle TTL instead. Cold start includes the engine's load time (minutes for a large vLLM model), so on-demand suits models used in bursts, not latency-critical ones. No GPU was involved in testing this; the engine start, stop and wake paths are covered with fakes.

## The endpoint and API key

The model's hostname is routed through the built-in ingress to the control plane, whose gateway checks the API key and proxies to the engine. It speaks the OpenAI API:

```sh
curl https://llm.example.com/v1/chat/completions \
  -H "Authorization: Bearer lr-..." -H "Content-Type: application/json" \
  -d '{"model": "llama3.1:8b", "messages": [{"role": "user", "content": "hello"}]}'
```

- The key is generated at deploy time and shown once. Only its SHA-256 hash is stored. Lost it? `levelrail-cli models rotate-key <name>` replaces the `default` key at once. For more than one key, see [Virtual keys](#virtual-keys-and-usage).
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

Limits are global (every model) and set with environment variables on the control plane. `0` or a negative value disables a limit. The current values show on `GET /api/v1/models/{name}` (`limits`) and in `levelrail-cli models get`.

| Variable | Default | Effect |
| --- | --- | --- |
| `APP_MODEL_GATEWAY_MAX_BODY_BYTES` | `33554432` (32 MiB) | Larger request bodies get `413`. |
| `APP_MODEL_GATEWAY_MAX_N` | `16` | `n` or `best_of` above this gets `400`. |
| `APP_MODEL_GATEWAY_MAX_TOKENS` | `32768` | `max_tokens`, `max_completion_tokens` or `max_output_tokens` above this, or negative (unlimited on llama.cpp), gets `400`. A request that sets none is passed through; the engine's context length bounds it. |
| `APP_MODEL_GATEWAY_MAX_INFLIGHT` | `32` | Concurrent requests per model; more get `429` with `Retry-After`. Streams hold a slot until they end. |
| `APP_MODEL_GATEWAY_RETRY_AFTER` | `5s` | The `Retry-After` value. |
| `APP_MODEL_GATEWAY_DIAL_TIMEOUT` | `5s` | Connecting to the engine. |
| `APP_MODEL_GATEWAY_HEADER_TIMEOUT` | `5m` | Waiting for the engine's response headers. A non-streaming completion sends none until it finishes, so keep this above your longest generation. Exceeded: `504`. |
| `APP_MODEL_GATEWAY_IDLE_TIMEOUT` | `2m` | Longest gap between engine output on a response. It is a gap, not a total, so long streams that keep producing tokens are never cut. |

JSON bodies are read up to the size cap so `n` and the token limits can be checked; the body is then forwarded unchanged. Audio uploads are size-capped but not parsed. Request and response bodies are never logged: each request logs only method, model, status, duration and bytes.

## Virtual keys and usage

A model can have several named keys, so each client gets its own identity, limits and usage. The key made at deploy time is the key named `default`; existing models were migrated to it, and it keeps working unchanged.

```bash
levelrail-cli models keys create chat --name ci --rpm 60 --tpm 100000 --tpd 2000000 --max-parallel 4 \
  --allow-paths /v1/chat/completions --expires-in 720h
levelrail-cli models keys list chat
levelrail-cli models keys rotate chat <key-id> --grace 30m
levelrail-cli models keys revoke chat <key-id>
levelrail-cli models usage chat --since 168h
```

In the dashboard, the gauge button on a model row opens the keys panel (create, rotate, revoke, last used, limits) and the usage card (requests, tokens, errors and time to first byte over time, plus a per key table).

- Only the SHA-256 of a key is stored. The key is shown once, with its first 8 characters kept as a prefix for identification. `last used` is updated on each flush interval.
- Rotation issues a replacement with the same name, limits and expiry. The old key keeps working for a grace window (`--grace`, default `APP_MODEL_KEY_ROTATION_GRACE`, `1h`; at most `APP_MODEL_KEY_MAX_GRACE`, `168h`; `0` retires it at once), then stops. Revoking stops a key immediately.
- Limits are per key. `rpm` and max parallel are enforced at the gateway: a request over either gets `429` with `Retry-After` and the same generic OpenAI-style error, so the response never says which limit tripped. `tpm` and `tpd` (tokens per day, rolling 24 hours) are soft: tokens are counted from responses after they finish, so the request that crosses the limit still completes and later ones get `429` until the window rolls over. An unknown, revoked or expired key always gets `401`.
- Allow lists: `allow_paths` are exact gateway paths (a trailing `/` allows everything below it) and must be routes the engine's allowlist already serves, so a key can never reach an engine admin route. `allow_models` are compared with the `model` field of the request body; a request without one is refused when the list is set.
- Each key records who created it (`created_by`). The MCP server can list keys (`list_model_keys`) and revoke one (`revoke_model_key`).
- At most `APP_MODEL_MAX_KEYS` (`50`) live keys per model.

### What is metered

Per key and model, per hour: requests, 2xx, 4xx and 5xx counts, requests refused with `429`, input and output tokens, response bytes, total duration and time to first byte (the first byte the engine sends, not the first token).

Tokens are read from the response's `usage` object: non-streaming responses, and streams opened with `stream_options.include_usage`. A stream without it, or a compressed response, is counted as a request only. Nothing is estimated, and the report says so (`usage_requests` shows how many requests carried usage). Prompts and bodies are never stored or logged; only the last `APP_MODEL_USAGE_SCAN_BYTES` of a JSON or event-stream body are held in memory to find `usage`.

Counts are aggregated in memory and written in bounded batches, so the request path never waits on the database. If the control plane stops between flushes, up to one interval of counts is lost.

| Variable | Default | Effect |
| --- | --- | --- |
| `APP_MODEL_USAGE_FLUSH_INTERVAL` | `30s` | How often aggregates are written. |
| `APP_MODEL_USAGE_BATCH_SIZE` | `200` | Rows per write. |
| `APP_MODEL_USAGE_MAX_BUFFERED` | `10000` | Hourly aggregates held between flushes; more are dropped and logged. |
| `APP_MODEL_USAGE_RETENTION` | `720h` | Hourly rows older than this are deleted. `0` keeps them. |
| `APP_MODEL_USAGE_SCAN_BYTES` | `16384` | Tail of a response searched for `usage`. `0` turns token counting off. |
| `APP_MODEL_KEY_MAX_LIMIT` | `10000000` | Largest rpm, tpm or parallel value a key may be given. |

The control plane has no Prometheus endpoint yet, so usage is exposed through `GET /api/v1/models/{name}/usage?since=24h`, the CLI and the `get_model_usage` MCP tool.

## GPU apps

Any app can request a GPU with `resources.gpu` in `app.yaml` (see the [app spec reference](/app-spec-reference)). Such an app only runs on a node with a usable GPU: the reconciler reports `NoGPUOnNode` or `GPURuntimeMissing` instead of starting it, and moving it to a node without one is rejected.

## GPU scheduling

Placement counts GPUs, not just workloads. Each node has a ledger built from the desired state: every app with `resources.gpu` and every model on the node reserves GPUs.

| Request | Reserves |
| --- | --- |
| a count, `gpus: 2` | that many GPUs, anonymous |
| device IDs (index or UUID) | those exact devices; a second workload asking for the same device does not fit |
| `all` or no count | every GPU, so the node must be completely free |

A workload fits a node when the node reports a GPU, Docker has the nvidia runtime, and enough GPUs are free (total minus reserved, never below zero). A node that is exactly full fits nothing more. A workload's own reservation is ignored when re-checking it against its own node.

Where the ledger is consulted:

- Create: A new GPU app without `node_id` is auto-placed on the least loaded node that fits (the local host is the fallback), or refused with `409` and a per-node reason. An explicit `node_id` is checked the same way once the node has reported.
- Move: `PUT /api/v1/apps/{name}/node` rejects a target that lacks the runtime or the free GPUs.
- Drain: Each GPU app is placed on a node that fits, and GPUs it takes are reserved for the apps after it in the same drain. An app no node can host stays put and is reported under `blocked` (dashboard drain dialog, `levelrail-cli nodes drain`, API). Models are never moved, so they are always listed as blocked.

Docker does not isolate GPUs by count: two containers can share a device, so the ledger is scheduling accounting, not enforcement. A running app is never stopped because the ledger says the node is oversubscribed.

### Seeing reservations

- API: `GET /api/v1/gpus` adds `reserved_gpus`, `free_gpus` and `reservations` (`app:<name>`, `model:<name>`) per node. `GET /api/v1/nodes` and `GET /api/v1/nodes/{id}` carry a `gpu` summary.
- CLI: `levelrail-cli nodes list` has a GPU column (`free/total`), `levelrail-cli nodes get` prints a GPU block, `levelrail-cli models gpus` has a RESERVED column.
- Dashboard: The AI models page GPU cards and the node detail GPU card show reserved vs total GPUs and VRAM used vs total; the node list shows a `GPU free/total` badge.
- MCP: `list_gpu_nodes` returns the same fields.

### Attention and doctor

When a GPU app or model cannot run on its own node and no eligible GPU node has enough free GPUs, `levelrail-cli doctor` and the attention list (Status page, `levelrail-cli attention`, MCP `get_attention`) raise a warning `GPU app <name> cannot be placed`. Fix it by freeing a GPU (stop or shrink another GPU workload), adding a GPU node, or installing the nvidia container toolkit on the node that has GPUs.

## Access control

Listing and reading models and GPUs needs the `read` ability. Deleting and restarting needs `write`. Listing keys and reading usage need `read`; revoking a key needs `write`. Deploying, creating or rotating a key and setting a HuggingFace token need `write:sensitive`. Model resources can be targeted by IAM policies as `model:<name>`. The AI assistant asks for confirmation before any of the mutating model tools.

## Your own model server

A server Levelrail does not run (Ollama or LM Studio on a Mac, an MLX server, a cloud API) can be registered as an inference endpoint under AI models, with a health badge, discovered models, per-key limits and usage, and an optional fallback to another model. Internal addresses need `APP_MODEL_ENDPOINT_ALLOW_PRIVATE_NETWORKS=true`. See [Use your Mac mini or MacBook as a private LLM server](/guides/use-a-mac-as-a-private-llm-server).

## Not in version 1

AMD and Apple GPUs, MIG partitioning, CPU mode for Ollama and vLLM, control plane downloads for remote nodes, automatic model-to-node scheduling (you pick the node; apps are spread, see GPU scheduling), moving a model between nodes, per-model limit overrides, swap groups, and a Prometheus endpoint for usage.

## GPU in Compose templates

A Compose file can request NVIDIA GPUs with the standard
`deploy.resources.reservations.devices` block. Levelrail reads `driver`
(only `nvidia` or unset), `count` (a number or `all`, unset means all),
`device_ids` and `capabilities: [gpu]`, and maps it onto `resources.gpu`.
The service then only starts on a node with a working NVIDIA runtime.
Catalogue entries that need a GPU carry a "Needs NVIDIA GPU" badge.
