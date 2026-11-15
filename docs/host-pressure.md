---
title: "Host pressure guard (beta)"
description: How Levelrail notices a host running out of CPU or memory, holds new builds until it recovers, and tells you what to change.
"og:url": "https://docs.levelrail.com/host-pressure"
---

A small host that runs many containers and several builds at once can thrash: load climbs into the hundreds, memory stalls, and the kernel starts killing processes, sometimes the control plane itself. The host pressure guard notices this early and sheds new work instead.

## What it reads

Every few seconds the control plane reads `/proc/pressure/cpu` and `/proc/pressure/memory` (PSI, the 10 second averages), `MemAvailable` and `SwapTotal` from `/proc/meminfo`, the CPU count and the load average. These are a few small files, not a process scan.

Where PSI is not available (old kernels, containers, other operating systems) the guard falls back to available memory and load per CPU. Where there is no memory source at all it stays quiet and Doctor shows the row as unknown.

## States

| State | Meaning | Effect |
| --- | --- | --- |
| ok | Pressure is normal. | None. |
| warn | One signal crossed its warn threshold. | One build runs at a time. |
| critical | One signal crossed its critical threshold. | New builds wait in the queue. |

The state rises at once and falls only when every signal is below the exit threshold (a percent of the enter threshold) for the hold time, so a brief lull does not release the queue into a storm.

## What is held, and what never is

While critical, new builds on this host wait with the status `waiting: host under memory pressure`. When the state returns to ok they resume after a random delay, so they do not all start together. Never held: running builds and apps, the API, the reconcile loop for existing apps, rollbacks, stops, and builds on other nodes (the reading is for this host only). Pulls of prebuilt images are not held yet.

## What you see

- Doctor: a `Host pressure` row and a `Swap` row. The swap row warns when no swap is configured and RAM is below the threshold.
- The attention feed: one item while the host is warm or critical, with the top memory users by container and concrete actions.
- An event, `host.pressure`, on every state change, for event webhooks.
- Metrics samples `host_pressure_state` (0 ok, 1 warn, 2 critical), `host_psi_cpu_some_avg10`, `host_psi_memory_some_avg10`, `host_psi_memory_full_avg10` and `host_load1`, readable through the metrics remote read endpoint.
- `levelrail-cli doctor` lists both rows.

## What to do about it

1. Add swap. A 1 to 2 GB swap file turns a sudden memory spike into a slowdown instead of a kill.
2. Lower build concurrency in the build limits. With no saved value, the slot count is derived at boot from CPUs and RAM (one per two CPUs and per 4 GiB, at least one).
3. Set memory limits on the apps listed as top consumers.

## Settings

All thresholds are runtime settings (Settings, then the `host.` keys, or the API and CLI). The environment variables only seed the defaults.

| Key | Default | Purpose |
| --- | --- | --- |
| `host.pressure_enabled` | true | Sampling and reporting. |
| `host.pressure_shed_enabled` | true | Hold new builds under pressure. |
| `host.pressure_interval` | 10s | Sample interval. |
| `host.pressure_hold` | 2m | Time below exit before stepping down. |
| `host.pressure_exit_percent` | 70 | Exit threshold as a percent of enter. |
| `host.pressure_resume_jitter` | 30s | Random delay before a queued build resumes. |
| `host.pressure_cpu_warn`, `host.pressure_cpu_critical` | 60, 85 | CPU some avg10. |
| `host.pressure_mem_warn`, `host.pressure_mem_critical` | 15, 30 | Memory some avg10. |
| `host.pressure_mem_full_warn`, `host.pressure_mem_full_critical` | 5, 15 | Memory full avg10. |
| `host.pressure_avail_warn_percent`, `host.pressure_avail_critical_percent` | 10, 5 | MemAvailable as a percent of RAM. |
| `host.pressure_load_warn`, `host.pressure_load_critical` | 200, 400 | Load per CPU times 100, without PSI. |
| `host.no_swap_ram_threshold_mb` | 16384 | Warn about missing swap below this RAM. |

This feature is beta: the defaults are conservative and have been exercised with injected readings, not yet on a real overloaded host.
