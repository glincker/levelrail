# Stress testing a local control plane

`levelrail-stress` is a developer tool that boots its own control plane on a scratch data directory and free loopback ports, loads it, and judges the run against budgets you keep in a profile file. It never touches a running install.

```sh
go run ./cmd/levelrail-stress -profile stress/profiles/small.yaml -report stress-report.json
```

The command prints a one-screen summary and exits with code 1 when any budget is exceeded.

## What a run does

1. Builds the control plane (or uses `-cp-bin`) and starts it with Docker pointed at a socket that does not exist, so no containers are touched.
2. Creates the first admin and enrolls fake nodes through the real join-token and agent API path. The fake nodes hold real sessions and heartbeat, backed by an in-memory container runtime.
3. Seeds apps (with env vars and domains), databases and deploy history, placed round robin on the fake nodes.
4. Runs three timed phases: steady (light reads), churn (deploys at the profile rate) and load (list clients plus watch streams, with deploys continuing).
5. Samples the control plane once a second (RSS, goroutines from pprof, SQLite file sizes) and polls reconcile pass time from the control plane health endpoint.

## Profiles

Profiles live in `stress/profiles/` (`small`, `medium`, `large`). Each sets the estate size, load shape, duration, seed and the thresholds: p95 and p99 latency, RSS, goroutines, SQLite size, reconcile pass time and error rate. Unknown keys are rejected. Set `LEVELRAIL_STRESS_PROFILE` instead of `-profile` if you prefer. The shipped thresholds are starting points for a developer machine, not guarantees.

## Limits

- Fake mode has no real containers, so ingress reloads under traffic, Docker event storms and container memory are not covered.
- Log ingest is not driven yet; the report says so.
- Caddy's admin endpoint keeps its default address, so do not run it next to another control plane that holds it.
- Numbers from a laptop describe that laptop only.

The seeder is also available as `go run ./scripts/perf-fixtures -data-dir <scratch dir>` for UI measurement.
