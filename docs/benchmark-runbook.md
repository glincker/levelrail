---
title: "Benchmark runbook"
description: Steps to measure load and failover on a labelled Linux server with a separate load generator, and how to publish the result.
---

Use this to produce rows for [Load and failover benchmarks](/benchmarks). The script never creates or deletes servers; you do that by hand.

## Rules

- Publish only rows with a date and a hardware label. Anything unmeasured stays marked not yet measured.
- Rows with `client_saturated` above zero are invalid. Rerun with a bigger load generator.
- Never mix laptop and server numbers in one table. Say what each row ran on.
- Keep the raw JSON in `docs/benchmark-results/` next to the table.

## Provision by hand

- Server: one Linux VPS, a size you will name in the row (for example 4 vCPU, 8 GB). Install Levelrail with `install.sh`, then deploy the fixture app with 2 replicas behind the [load balancer](/load-balancing) with a domain.
- Load generator: a second VPS in the same region and network segment, never the server itself. 4 vCPU or more.
- For a node drain row, add a second node to the server's placement (see [Add a node](/add-node-anywhere)).
- Build the fixture on the server: `docker build -t bench-app test/fixtures/bench-app`.
- Make a note of the network between the two machines (region, private or public).
- Create an API token on the server for the load generator.
- Destroy both machines when done; billing is on you.

## Run

On the load generator, with `levelrail-cli` installed and `jq` available:

```bash
export BENCH_HOST=203.0.113.10 BENCH_USER=root
export BENCH_INSTALL_URL=https://levelrail.example.com
export BENCH_TOKEN=...
export BENCH_APP_URL=https://bench.example.com/
export BENCH_REPLICA_URL=http://REPLICA_IP:8080/
scripts/bench/run-vps.sh --dry-run
scripts/bench/run-vps.sh
```

Optional: `BENCH_DRAIN_NODE` and `BENCH_DRAIN_TARGET` (node drain), `BENCH_RATES`, `BENCH_DURATION`, `BENCH_NOTE` (for example `2 nodes, same region, private network`), `--skip steady,replica-kill,node-drain,cp-restart`.

## Scenarios

| Name | What it does |
| --- | --- |
| `steady-<rate>` | Open loop at each rate in `BENCH_RATES` (default 100, 500, 1000, 2000, 4000), 60 s each, with the ingress overhead comparison when `BENCH_REPLICA_URL` is set |
| `replica-kill` | 500 req/s for 90 s, one replica container exits one third in |
| `node-drain` | 500 req/s for 120 s, drains one node onto another. Stands in for node loss; a hard power-off is a manual test and stays not yet measured until someone records it |
| `cp-restart` | 500 req/s for 90 s, restarts the control plane, reports whether the ingress kept serving |

## Publish

- The script writes `docs/benchmark-results/<date>-vps-*.json` and `<date>-vps-fragment.md`. The hardware line comes from `lscpu`, `free`, `uname` and `docker version` on the server.
- Check the fragment: saturated is 0, hardware line is complete, date is today.
- Paste the fragment under "Results" in `docs/benchmarks.md` and update the matching rows in the scenario status table. Rows you did not run keep their not yet measured status.
- Add the load generator size and network note to the paragraph above the table.
