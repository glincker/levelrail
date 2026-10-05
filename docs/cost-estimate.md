---
description: How Levelrail's "what this would cost elsewhere" app cost estimate is calculated, its assumptions, and the env vars that let you correct the reference rates for your own region.
---

# Cost estimate

Every app's **Resources** tab shows an estimated monthly cost under a
few illustrative reference pricing profiles, alongside the resource
limits editor. The CLI exposes the same number with `levelrail-cli apps
cost <name>`, and the API with `GET /api/v1/apps/{name}/cost-estimate`.

![Levelrail Resources tab with a resource suggestion and a what-this-would-cost-elsewhere estimate](assets/screenshots/app-resources.png)

**This is an estimate, not a real bill.** It never calls a live pricing
API and never reflects what your own hardware actually costs you. The
point is comparative: showing roughly what the same CPU/memory envelope
would cost under typical metered cloud pricing, so self-hosting's real
advantage shows up as a number instead of an assertion.

## What it's based on

The estimate needs a CPU and a memory size for the app. It resolves
each dimension independently:

1. **Declared**, when the app's `resources:` block in `app.yaml` (or an
   equivalent API/CLI call) sets a memory or CPU limit. This is the
   preferred basis: it's what you actually told Levelrail to reserve.
2. **Observed**, when no limit is declared but at least a few hours of
   usage telemetry exist. The estimate falls back to the 95th
   percentile (p95) of actual CPU and memory usage over the last 7
   days, the same lookback window the resource recommendation feature
   uses.
3. **Unavailable**, when neither a declared limit nor enough usage
   history exists yet (a brand-new app with no `resources:` block and
   telemetry disabled or not yet collecting). The estimate still
   returns a response, using the provider's minimum monthly charge, so
   the UI has something to show rather than an error, but the size and
   basis fields make it clear there's nothing real behind the number
   yet.

## The formula

For each reference provider:

```
cpu_cost    = vcpu_cores  * provider.cpu_per_core_usd
memory_cost = memory_gib  * provider.memory_per_gib_usd
total       = max(cpu_cost + memory_cost, provider.minimum_usd)
```

That's it. No tiered discounts, no sustained-use pricing, no egress or
storage costs (Levelrail doesn't yet have a per-app storage usage
metric to estimate that dimension honestly, so it's left out rather
than guessed).

## Reference providers

Three illustrative profiles ship by default, in
`internal/costestimate/costestimate.go`. None of them is any single
real provider's actual rate card; they're round numbers meant to be
directionally honest for 2026-era budget cloud pricing, not a quote.

| Provider | What it represents | $/vCPU-core/mo | $/GiB/mo | Minimum/mo |
| --- | --- | --- | --- | --- |
| Generic cloud VM | A bare unmanaged VM/IaaS instance | $4 | $4 | $4 |
| Generic managed VM | A managed VM with backups/support bundled in | $8 | $6 | $10 |
| Generic metered container PaaS | Heroku/Render/Railway-shaped metered pricing, which bundles a real margin over raw compute | $25 | $10 | $7 |

The gap between the first and third rows is the point: metered
container platforms charge several times raw compute cost for the
convenience of not managing a server, which is exactly the cost
Levelrail's self-hosted model avoids.

## Correcting the rates for your own region

Hardcoded defaults are never going to match your actual provider or
region, so every rate is overridable with an env var on the control
plane process:

```
APP_COST_ESTIMATE_VM_CPU_USD=3.5
APP_COST_ESTIMATE_VM_MEMORY_USD=3.0
APP_COST_ESTIMATE_VM_MINIMUM_USD=5

APP_COST_ESTIMATE_MANAGED_VM_CPU_USD=7
APP_COST_ESTIMATE_MANAGED_VM_MEMORY_USD=5
APP_COST_ESTIMATE_MANAGED_VM_MINIMUM_USD=12

APP_COST_ESTIMATE_PAAS_CPU_USD=20
APP_COST_ESTIMATE_PAAS_MEMORY_USD=8
APP_COST_ESTIMATE_PAAS_MINIMUM_USD=7
```

A malformed or negative value is ignored and the built-in default is
kept; there's no way to misconfigure this into a crash. Restart the
control plane after changing any of these for the new rate to take
effect.
