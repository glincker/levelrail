# Local lab, tier 1

An opt-in second control plane for checking scheduler rules without agents. It is not wired into CI or hooks.

- `up.sh` builds the binaries, seeds `LAB_NODES` nodes (zones from `LAB_ZONES`) straight into a fresh store, and starts a control plane on `LAB_PORT` (default 18480) with its own `LAB_DIR`.
- `check.sh` drives the real CLI and REST API and prints a pass/fail table: NoSchedule taint, toleration, required anti-affinity, the bin-pack setting, and a watch answering 410 after a restart. It exits non-zero and names each failed check.
- `down.sh` stops the process; `--purge` also deletes the lab dir.
- The scripts refuse ports 8080, 8180, 5273, 5173 and any existing data dir.
- Seeded nodes report no capacity, so the headroom ordering that separates bin-pack from spread, and the server-side apply conflict check, wait for tier 2 with real agents.
- `repo/` holds the YAML fixtures the checks apply.
