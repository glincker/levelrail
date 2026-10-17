# ADR 034: Plugin contract and marketplace

Status: Proposed (direction approved by the owner 2026-10-17; contract details open)

Decided for slice 1 (notification channels, behind the `plugins` experimental flag):

- Transport: gRPC over a unix socket in a host directory bind-mounted into the plugin container. No network exposure. Remote plugins over the mesh stay open.
- Packaging: an OCI image reference pinned by digest, plus a `plugin.yaml` manifest shipped alongside it at install. Archive packages and a manifest in an image label are later.
- Signing: ed25519 detached signature over the manifest bytes and the image digest, verified against an operator-managed trust list kept in the database. The default list is empty, so nothing third party loads until the operator adds a key.
- Abilities: only `read` and `read:sensitive` are grantable. Approval is always an explicit operator step, never implied by install, and is re-checked on every call. Plugins get no API token in this slice.
- Network: a plugin with no declared outbound hosts runs with no network. Declared hosts join a dedicated bridge network, shown at approval but not yet enforced per host.
- Governance, signing roots, marketplace and remote placement stay open.

Date: 2026-10-17

## Context

The core is meant to stay small: reconcile, ingress, mesh, store, auth and
telemetry. Everything else (DNS and CDN providers, cloud provisioners,
notification channels, git providers, AI providers, template packs, importers,
dashboard panels) keeps growing inside the one binary. That raises idle cost
and review load for installs that use none of it, and it makes partner work
(for example a Cloudflare or Akamai steering integration, ADR 033 step 7)
depend on core releases.

ADR 031 settled the rule for today: five integration points (sources,
targets, templates, events, automation), out of process, no in-process
extension of the control plane, and providers written natively in this repo.
It also said "no plugin runtime". This ADR refines that: it keeps the
out-of-process rule and the no-third-party-code-in-the-control-plane rule,
and replaces "no plugin runtime" with a defined boundary that plugins run
behind. ADR 031 point 3 (native clients for providers) applies until a
provider has been extracted.

The control plane holds the master key, every stored credential and reach to
every node, so the boundary is a security decision first.

### Existing seams this reuses

- Scoped token abilities (ADR 012, `internal/api/abilities.go`): the
  vocabulary for what a caller may do, minted per token and checked on every
  request, with audit.
- Supply chain checks (`internal/supplychain`): SBOM generation, scanning and a
  gate with allow, warn, block and override decisions. Today it gates app
  releases; the same gate can check a plugin package.
- Provider code that is already shaped like a plugin: `internal/dnszones` and
  `internal/dnsrecords` (Cloudflare, Route53), `internal/provision` (node
  providers), `internal/alerting` and `internal/eventhook` (events and
  notification channels), the git provider packages and `internal/webhook`.
- The template manifest format in `internal/catalog` (`manifest.go`,
  `manifest_load.go`, `manifests/`), which is already declarative data with
  prompts and health checks, and the community repository model of ADR 031.
- Agent transport: `proto/agent/v1` shows the project's gRPC conventions,
  versioning and mTLS enrollment (ADR 003, ADR 032).
- The MCP tool layer (`internal/mcptools`), which already exposes platform
  actions to out-of-process callers under token scopes.
- `internal/integrations`, the catalog of known integrations shown to
  operators, which becomes the natural home for installed-plugin entries.

## Decision

1. **Out-of-process plugins over a versioned gRPC contract.** A plugin is a
   separate process or container. The control plane calls it through a
   contract defined in `proto/plugin/v1`, with an explicit contract version
   that both sides negotiate at connect time. Nothing is loaded into the
   control plane or agent binary. Each extension kind (DNS provider, node
   provisioner, notification channel, git provider, AI provider, importer,
   steering provider) is a small service in the contract, so a plugin
   implements only what it declares.
2. **Capabilities, not trust.** A plugin manifest lists the abilities it
   needs, using the ADR 012 vocabulary, plus any outbound network hosts it
   will contact. The operator reviews and approves them at install. At run
   time the plugin holds a token with exactly those abilities, so the existing
   checks and audit log apply to it like any other caller. Anything not
   approved is refused by the host, not by the plugin's good behavior.
3. **Secrets per call.** The host passes the credential a call needs with
   that call, and the plugin is told not to persist it. Plugins do not read the
   control plane store, the master key or other plugins' secrets. A plugin has
   its own small state directory for non-secret cache.
4. **Signed, digest-pinned packages.** A package is an OCI image (or an
   archive for the first stage, see open questions) referenced by digest, with
   a signature and an SBOM. Install verifies the signature against a trusted
   publisher key, pins the digest, and runs the `internal/supplychain` gate
   (scan, license, SBOM) with the operator's configured mode. An update is a
   new digest and re-asks for any new ability.
5. **Declarative UI and CLI extensions.** A plugin contributes settings forms,
   pages, panels and CLI subcommands through a manifest that the core renders
   with its own components and i18n. A plugin does not ship JavaScript into
   the dashboard origin. Plugin supplied text goes through the same escaping
   as any other user data.
6. **First-party plugins use the same contract.** The boundary is exercised
   daily or it rots. A first-party plugin gets no private channel and no
   extra ability that a third party could not request.
7. **Staged order, and nothing moves for its own sake.**
   1. This ADR and the contract definition (`proto/plugin/v1`), with a
      conformance test suite a plugin can run.
   2. One extracted plugin end to end, the Cloudflare DNS and CDN provider
      (from `internal/dnszones`, `internal/dnsrecords`), running beside the
      built-in client until it matches it, then replacing it if that removes
      complexity.
   3. Install flow in the dashboard, CLI and API: review abilities, approve,
      install, update, disable, remove, with audit entries.
   4. Marketplace index: a reviewed list of plugin entries (publisher, digest,
      abilities, verification status), a static signed document before any
      hosted service.

   Existing built-in integrations move only when doing so removes complexity or
   idle cost. Horizon 1 and Horizon 2 work is not delayed by this.

## Threat model and mitigations

| Threat | Mitigation |
| --- | --- |
| Malicious plugin tries to read secrets or reach nodes | Separate process with no store or key access; per-call secrets; token limited to approved abilities; host-side enforcement; every call audited |
| Plugin escalates beyond what the operator approved | Abilities fixed at install and re-approved on change; no ability implied by another except as in ADR 012; `root` is never grantable to a plugin |
| Supply chain: tampered or typosquatted package | Signature check against trusted publisher keys; digest pinning; no tag following; supplychain scan and gate at install and update; install refuses unsigned packages unless the operator overrides with a recorded reason |
| Secret exposure through logs or plugin storage | Secrets passed per call and redacted from host logs; plugin logs are captured separately and run through the existing redaction; plugin state directory is non-secret by policy and size limited |
| Resource exhaustion by a plugin | Container limits for CPU, memory, disk and process count; per-call timeouts and concurrency caps; circuit breaker that disables a plugin after repeated failures; a plugin failure never blocks reconciliation |
| UI injection or phishing through plugin-supplied content | No plugin code in the dashboard origin; declarative manifests rendered by core components; escaped text; links and images from allow-listed hosts only; publisher name shown on every plugin page |
| Plugin impersonates a built-in feature | Plugin-provided pages are labeled with the plugin name and publisher in the UI chrome |
| Compromised publisher key | Key rotation and revocation list in the marketplace index; installed plugins are re-checked against it; revoked digests are disabled and flagged |
| Plugin exfiltrates data over the network | Declared outbound hosts are enforced by network policy where the runtime supports it, and shown at approval; undeclared hosts are a documented gap until enforced |

## Rejected alternatives

- **In-process Go plugins.** Rejected: loaded code inherits the master key,
  the Go `plugin` package is fragile across builds and breaks the static
  cross-compiled binary (ADR 001).
- **WASM only.** Rejected as the sole mechanism: a sandbox is attractive, but
  provider SDKs, network clients and cloud libraries are not yet comfortable
  in WASM runtimes, and it adds a runtime to the binary. It stays open as a
  second execution mode behind the same contract.
- **Lua or JS scripting in core.** Rejected: embeds an interpreter in the
  privileged process and makes every script a code review of secret handling.
- **Shipping everything built in forever.** Rejected: idle cost and review
  load grow with every provider, and partners cannot ship on their own
  schedule. Built-ins stay only where they are small and core to the product.
- **An unreviewed open registry at launch.** Rejected: an open registry is a
  malware distribution channel for a tool that holds cloud credentials. The
  first index is a curated, signed list with entries added by reviewed pull
  request, the model ADR 031 set for community templates.
- **A hosted marketplace with accounts, ratings and payments first.**
  Rejected for now: it needs moderation and a service to run, which do not
  fit a self-hosted project at this stage.

## Consequences

- A contract version and a deprecation policy become public commitments.
  Breaking the contract breaks third parties.
- Plugin calls add latency and a failure mode to paths that were function
  calls. Calls need timeouts, retries only where idempotent, and a clear
  condition on the resource when a plugin is unavailable.
- Operators get a place to see what is installed, what it may do and who
  published it, in the dashboard, CLI and API together.
- ADR 031 needs a cross reference to this ADR once accepted.

## What we do not do

- No third-party code in the control plane or agent process.
- No auto-install, auto-update or install from an arbitrary URL.
- No plugin with `root`, and no ability beyond the operator-approved list.
- No marketplace payments or revenue sharing in this ADR.
- No rewrite of working built-in integrations only to make them plugins.

## Open questions

- **Transport:** gRPC over a unix socket for same-host plugins and over the
  mesh with mTLS for remote ones, or one transport for both. HTTP and JSON as
  an alternative for simple plugins.
- **Packaging format:** OCI image only, or also a signed archive with a
  single static binary for hosts without a registry.
- **Signing roots:** a project-held root with delegated publisher keys,
  keyless signing with a transparency log, or both. Who holds the root.
- **Governance:** who reviews entries, the criteria, the response time, how
  disputes and takedowns work, and whether any revenue model exists.
- **Network enforcement:** how to enforce declared outbound hosts on nodes
  without a supporting firewall layer.
- **Where plugins run:** on the control plane host only, or on nodes through
  the agent, which changes the trust and resource story.
