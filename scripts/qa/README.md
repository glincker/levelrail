# QA droplets

`droplet.sh` runs real-VPS validation on throwaway DigitalOcean droplets. Every droplet carries the tag `levelrail-qa`, and `down` only ever touches that tag.

```bash
export QA_SSH_KEY=~/.ssh/my_qa_key        # private key path; never copied into the repo
scripts/qa/droplet.sh --dry-run up 2      # print the exact commands first
scripts/qa/droplet.sh up 2 --region nyc3 --size s-2vcpu-4gb --runtime docker
scripts/qa/droplet.sh build --arch both   # web + control plane (-tags embedweb) + cli + agent
scripts/qa/droplet.sh push 203.0.113.10 203.0.113.11
QA_API_TOKEN=... scripts/qa/droplet.sh enroll
scripts/qa/droplet.sh status
scripts/qa/droplet.sh cost
scripts/qa/droplet.sh down --yes
```

## Commands

| Command | What it does |
| --- | --- |
| `up N` | Creates N droplets tagged `levelrail-qa`, waits for ssh, installs the container runtime. The DO ssh key comes from `QA_DO_SSH_KEY` or the first key `doctl` lists. |
| `build` | Builds for linux/amd64, arm64 or both into `$QA_CACHE/<key>/<arch>` (default `~/.cache/levelrail-qa`). The key hashes HEAD, the working tree diff and the arch, so an unchanged tree is a cache hit. Refuses to finish when `web/dist/index.html` is missing or the control plane binary is too small to contain the dashboard (`QA_MIN_CP_BYTES`). |
| `push <ips>` | Per binary: skips when the remote sha256 already matches, else compresses (zstd, gzip fallback), rsyncs with `--partial --append-verify` and 5 retries, verifies the sha256 on the host, swaps it in with an atomic `mv`, then restarts `levelrail` and `levelrail-agent` if their units exist. The architecture is read from `uname -m` on each host. |
| `enroll` | The first droplet is the control plane, the rest are agents. Mints a join command per agent with `levelrail-cli nodes add` on the control plane and runs it on the agent. The API token travels over ssh stdin only. |
| `status` / `cost` | Lists tagged droplets with service state and binary sha prefixes; sums their hourly price. |
| `down --yes` | Prints the tagged droplets, then deletes by tag. Without `--yes` it only prints and exits non-zero. |

`--dry-run` is global: every command that changes anything (doctl, ssh, rsync, go build) is printed instead of run.

## Tests

`scripts/qa/droplet_test.sh` covers the pure helpers and the dry-run output with `doctl`, `ssh` and `rsync` stubbed on `PATH` (the stubs exit 99, so a leak to a real droplet fails the test).

## Not covered offline

Real droplet creation and ssh readiness, the rsync resume path, the join command parsing in `enroll` (it expects a line starting with `sudo ` in the `nodes add` output), and the service restart units. Check these on the first real run.
