# Benchmark results

This folder holds the raw result files from Levelrail load and failover runs, each with the date and hardware it was measured on. The tables and prose built from them are in [Load and failover benchmarks](../benchmarks.md). The steps to produce a new run are in the [benchmark runbook](../benchmark-runbook.md).

## Files

| File | Date | What it measures | Hardware line as recorded | Levelrail version as recorded | Runs | Type |
| --- | --- | --- | --- | --- | --- | --- |
| [2026-10-22-all-scenarios.json](2026-10-22-all-scenarios.json) | 2026-10-22 | Chaos and load scenarios: `kill-container`, `daemon-unreachable`, `control-plane-restart`, `disk-pressure`, `oom-during-readiness`, `ingress-steady`, `deploy-latency`, `rollout-under-traffic`, `ingress-concurrency`, `replica-failover` | Apple M4 Max, 14 CPUs, macOS 27.2, Docker Desktop 28.3.0 (7 GiB visible to Docker), "dev laptop, shared with other work" | build `v0.8.3-0.20261022162604-a9342ea38480`, commit `58233c72355897b6e33a13a0cd7f27e6d0a40049` | 2 per scenario, 20 entries (16 passed, 4 skipped) | JSON |
| [2026-10-22-deploy-rollout-concurrency.json](2026-10-22-deploy-rollout-concurrency.json) | 2026-10-22 | `deploy-latency`, `rollout-under-traffic`, `replica-failover`, `ingress-concurrency` (the scenarios the first file skipped) | Same laptop as above | build `v0.8.3-0.20261022162604-a9342ea38480`, commit `1403b01497f596f25d05913ddc1e690d6663a02c` | 2 per scenario, 8 entries (7 passed, 1 failed) | JSON |
| [2026-10-22-replica-failover-rerun.json](2026-10-22-replica-failover-rerun.json) | 2026-10-22 | `replica-failover` again, after a harness fix | Same laptop as above | build `v0.8.3-0.20261022162604-a9342ea38480`, commit `fc525fa85c57d87f342b4ef6eb0eb164111a61fb` | 3 | JSON |
| [2026-11-18-oracle-arm-1vcpu.json](2026-11-18-oracle-arm-1vcpu.json) | 2026-11-18 | Upgrade, idle footprint, deploy, redeploy under 200 req/s, bad deploy, rollback, Postgres backup and restore, reboot | Oracle Cloud Ampere A1, aarch64, 1 vCPU, 5.8 GB RAM, 48 GB disk, Ubuntu 24.04, 2 VMs on one private network (the second is the load generator) | before `v0.9.5`, after `v1.5.3`, generator CLI `v1.5.3` | 1 | JSON |
| [2026-11-18-oracle-arm-1vcpu.md](2026-11-18-oracle-arm-1vcpu.md) | 2026-11-18 | Human-readable summary of the Oracle run above, with the failures and what was not run | Same as the Oracle JSON | Same as the Oracle JSON | 1 | Markdown |

The three 2026-10-22 files come from one laptop where the load generator, the ingress and the fixture container shared the machine over loopback, and the host was busy with other work. [Load and failover benchmarks](../benchmarks.md) explains what happened during those runs and how to read them. The Oracle run is summarized in its own Markdown file, so its numbers are not repeated here.

## How the results are produced

The 2026-10-22 files come from the chaos runner, built with the `chaoslive` tag and run on a machine with a local Docker daemon:

```bash
go build -tags chaoslive -o levelrail-cli ./cmd/levelrail-cli
BENCH_NOISY_LOAD_RATIO=1 ./levelrail-cli bench chaos run all --repeat 3 \
  --out results.json --md-out results.md --hardware "short note on the machine"
```

For a Linux server with a separate load generator, the runbook uses `scripts/bench/run-vps.sh`. Run it on the load generator, never on the server under test. It reads the hardware line from the server over SSH and drives the app through `levelrail-cli bench run`:

```bash
export BENCH_HOST=203.0.113.10 BENCH_USER=root
export BENCH_INSTALL_URL=https://levelrail.example.com
export BENCH_TOKEN=...
export BENCH_APP_URL=https://bench.example.com/
export BENCH_REPLICA_URL=http://REPLICA_IP:8080/
scripts/bench/run-vps.sh --dry-run
scripts/bench/run-vps.sh
```

The script writes `docs/benchmark-results/<date>-vps-*.json` (one file per scenario, such as `steady-500`, `replica-kill`, `node-drain`, `cp-restart`) and `<date>-vps-fragment.md`.

The 2026-11-18 Oracle file is a hand-assembled summary of a manual run (upgrade, backup, reboot and deploy checks plus one open-loop redeploy test). It was not written by `run-vps.sh`.

## How to read a file

The three 2026-10-22 files share one layout (`schema_version` 1):

- `host`: OS, CPU model and count, memory, Docker version and engine, `load1_start` and `load1_end` (1 minute load average before and after) and a free-text `note`.
- `build`: `version`, `commit` and `dirty`. The commit is the one to trust, because the version is a pseudo-version stamped from a different checkout.
- `repeat`: how many times each scenario ran. Every run is kept as its own entry in `results`, nothing is averaged or dropped.
- `results`: one entry per run, with `scenario`, `status` (`passed`, `failed` or `skipped`), `summary`, `requests`, `request_errors`, `error_window_ms`, `recovery_ms`, `duration_ms`, `hardware`, `assertions` and `metrics`.
- `metrics`: for load scenarios, `achieved_rps`, `p50_ms`, `p95_ms`, `p99_ms`, `max_ms` and `client_saturated`.
- `methodology`, `caveats` and `limits`: how the run was done, what to keep in mind, and which questions the run did and did not answer.

Terms:

- A scenario is one named test, for example `replica-failover`. A run is one execution of it. With `--repeat 2`, each scenario has two entries in `results`.
- Times are in milliseconds (`_ms`) unless a field says seconds (`_s`).
- Percentiles (`p50`, `p95`, `p99`) are nearest rank over every request, failures included. `max_ms` is the slowest request.
- Latency is measured from the intended send time of each request, so a stall shows up as queueing delay instead of being hidden. Load is open loop, except the `ingress-concurrency` steps, which are closed loop and say so.
- `error_window_ms` is the span from the first to the last failed request after the event. `recovery_ms` is the event to the last failed request.
- `client_saturated` counts requests the load generator shed because it hit its in-flight cap of 20000. A value above zero means the generator was the bottleneck and the run is not valid evidence about the server.

The files from `run-vps.sh` are one run per file. The fields the script reads are `rate_rps`, `ingress.achieved_rps`, `ingress.sent`, `ingress.failed`, `ingress.p50_ms`, `ingress.p95_ms`, `ingress.p99_ms`, `ingress.max_ms`, `ingress.client_saturated`, `ingress_overhead.p99_ms` (present when `BENCH_REPLICA_URL` is set), `event.dropped_requests`, `event.error_window_ms` and `event.recovery_ms`. Rows with `client_saturated` above zero are invalid and should be rerun with a bigger load generator.

The Oracle JSON has its own layout (`schema_version` 1): `hardware`, `versions`, `metrics` (one named entry per check, for example `redeploy_under_load_200rps_90s` with `sent`, `ok`, `failed`, `p50_ms`, `p95_ms`, `p99_ms`, `max_ms`), `failures`, `not_run` and `sanitized`. Addresses, hostnames, user names, tokens and keys are removed.

## Reproduce on your own server

1. Follow "Provision by hand" in the [benchmark runbook](../benchmark-runbook.md): one Linux server with Levelrail installed and the fixture app at 2 replicas, and a separate load generator of 4 vCPU or more in the same region and network segment. Build the fixture with `docker build -t bench-app test/fixtures/bench-app`.
2. Run `scripts/bench/run-vps.sh --dry-run` to see the commands, then `scripts/bench/run-vps.sh`. Optional settings are `BENCH_DRAIN_NODE`, `BENCH_DRAIN_TARGET`, `BENCH_RATES`, `BENCH_DURATION`, `BENCH_NOTE` and `--skip steady,replica-kill,node-drain,cp-restart`.
3. Check that `client_saturated` is 0 in every row, the hardware line is complete and the date is today.

The script never creates or deletes servers. Destroy both machines when you are done.

## Contribute a result

- Name the files `<date>-<host>-*.json`, with the fragment as `<date>-<host>-fragment.md` next to them. The script produces this shape with `vps` as the host label.
- Record the hardware: CPU model and core count, RAM, kernel, Docker version, node count and placement, the network between the load generator and the ingress, and the replica count and app resource limits. `run-vps.sh` reads the first four from the server. Add the rest with `BENCH_NOTE` (for example `2 nodes, same region, private network`) and in the paragraph above the table.
- Publish only rows with a date and a hardware label. Do not mix laptop and server numbers in one table. Anything you did not measure stays marked not yet measured.
- Keep the raw JSON in this folder, and paste the fragment into [Load and failover benchmarks](../benchmarks.md) as the runbook describes.

## Scope

- Each file is one run on one host. The 2026-10-22 files are from a shared laptop with Docker Desktop, and the 2026-11-18 file is from a single Oracle Ampere A1 vCPU. None of them is a large-server result.
- Numbers depend on the hardware, the network and the fixture app, which answers `GET /` with `ok`. Use them as a reference for that setup, and measure your own for yours.
- Runs per scenario are 1 to 3, so no confidence interval is given.
- These files describe Levelrail only. They are not comparisons against other products.
- Rows you did not run stay not yet measured, and no limit is claimed for them.
