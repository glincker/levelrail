# ADR 017: AI model resource on GPU nodes

Status: Accepted

Date: 2026-09-24

## Context

Operators want to run open-weight models (Ollama, vLLM, llama.cpp) on
NVIDIA GPU nodes and call them through an OpenAI-compatible endpoint.
Nothing in the platform knew about GPUs, and the app reconciler manages
generic containers with no notion of model downloads, "model loaded"
readiness or a per-resource API key.

## Decision

**A `model` is its own resource with its own reconciler**, not an app in
disguise. `internal/models` owns the table (`models`, migration 0124),
one `Controller` per model (same per-resource shape as the database
controller) and a `Service` the REST API, CLI and MCP tools share.
Status uses the standard `Ready` condition with reasons (`Downloading`,
`Loading`, `ModelLoaded`, `NoGPUOnNode`, and so on).

**GPU support is a Docker `DeviceRequest`**, added as `ContainerSpec.GPU`
and carried over the agent wire (`GPURequest` in the proto). `Runtime`
gained no methods, so no fake changed. `resources.gpu` in the app spec
reaches the same field.

**GPU detection is `nvidia-smi` plus the Docker daemon's runtime list**
(`internal/gpu`), never the docker CLI. The control plane samples its own
host; agents report over the existing Session stream (`GPUReport` frame).
Snapshots live in `node_gpus` (migration 0123), keyed `local` for the
control plane host because that host has no `nodes` row when the mesh is
off.

**Readiness is engine-specific HTTP against the engine's own API**:
Ollama `/api/tags` then `/api/ps` (the control plane drives `/api/pull`
in the background and surfaces percent progress), vLLM `/v1/models`,
llama.cpp `/health`.

**The API key is enforced by a gateway in the control plane, not by the
engine.** Ollama has no key support, and vLLM and llama.cpp would need
the plaintext at container-create time. The gateway checks a bearer key
against a stored SHA-256 hash, forwards only `/v1/` paths, and proxies
to the engine (loopback, or the node's mesh address). Ingress routes a
model's hostname to the control plane's own listener. The plaintext is
returned once at creation or rotation.

**The HuggingFace token uses the existing envelope-encryption path**
under its own secrets namespace and reaches only the engine container's
environment.

**Deleting is a tombstone** (`deleting = 1`) that the controller tears
down (container, secrets, then row), so the reconciler that owns the
container also owns its removal and every step retries after a partial
failure. The weights volume is deliberately kept.

**A GPU app is refused, not silently CPU-scheduled**, on a node without a
usable GPU: the application controller reports `NoGPUOnNode` and the
set-node route rejects the move.

## Consequences

- Remote nodes need the WireGuard mesh for the control plane to reach the
  engine. Without it the model runs but reports `EndpointUnreachable`.
- The agent wire format gained `GPURequest`, `Command`, `ShmSizeBytes`,
  `PortBinding.host_ip` and the `GPUReport` frame. `host_ip` also stops
  remote containers publishing a loopback-bound port on all interfaces.
- vLLM gets an 8 GiB `/dev/shm` for tensor parallelism.
- Scheduling is manual: the operator picks the node.

## Rejected alternatives

- **A model as a service in `desired_services` with a template**: reuses
  the app controller but has no place for download progress, the
  "model loaded" condition, key hashing or the gateway, and every one of
  those would leak into app code.
- **Engine-native API keys** (`--api-key`, `VLLM_API_KEY`): needs the
  plaintext stored recoverably, does not cover Ollama, and forces three
  different mechanisms.
- **A Caddy handler per model for auth**: Caddy cannot compare a bearer
  token against a stored hash without a plugin or a forward-auth hop, and
  the plaintext in the Caddy config would defeat hashing.
- **NVML bindings for detection**: requires cgo and the driver library at
  build time, which breaks the pure-Go cross-compiled single binary.
  `nvidia-smi` is present wherever the driver is.
- **Deleting the weights volume with the model**: re-downloading tens of
  gigabytes on a redeploy is the expensive mistake; leaving the volume is
  cheap and reversible.
