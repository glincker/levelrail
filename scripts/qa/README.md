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

## Stack scenarios

`stack-run.sh` makes QA and agent test environments stack-driven instead of imperative `apps create` calls. Each scenario is a YAML stack in `scripts/qa/stacks/`. To run apps on droplets from `droplet.sh`, pass the control plane IP as `--server`.

| Scenario | What it declares | Assertion |
| --- | --- | --- |
| `hello-web` | One image app with a readiness probe | HTTP 200 and the body contains `QA_BODY_MATCH` |
| `app-db` | An app plus Postgres, `DATABASE_URL` set with `from: {database, field: url}` | Same as `hello-web` |
| `rollback-loop` | One blue-green app applied with two image tags | A 200ms request loop runs through `stack rollback`; non-200 responses are counted and must not exceed `QA_LOOP_MAX_BAD` |

```bash
scripts/dev-local.sh start                  # local Docker Desktop, login dev/dev
scripts/qa/stack-run.sh hello-web --dry-run
scripts/qa/stack-run.sh hello-web --server http://127.0.0.1:8080 --token "$TOKEN"
scripts/qa/stack-run.sh rollback-loop --server http://203.0.113.10:8080 --token "$TOKEN" --keep
```

Each run renders the scenario with a unique suffix, runs `levelrail-cli stack apply`, waits for the deploy to converge (`apps deploys wait`, bounded by `QA_STACK_WAIT`), runs the assertion, destroys the stack (`stack delete --mode destroy`, skipped with `--keep`) and prints one line: `PASS|FAIL <scenario> <stack>`. Server and token default to `APP_API_URL` and `APP_API_TOKEN`. Stacks need `APP_EXPERIMENTAL=iac` on the control plane (the script sets it for the CLI). The app hostname is the zero-config `sslip.io` name, resolved to the server IP for the check; set `QA_APP_URL` to assert against a fixed URL instead, or `QA_INGRESS_HTTP_PORT` and `QA_INGRESS_HTTPS_PORT` when the server's ingress is not on 80 and 443. Each run creates its own project `qa-<suffix>` (a first stack apply needs one) and removes it, and the `app-db` database, after the destroy. Other knobs are listed at the top of the script.

`--dry-run` prints every `levelrail-cli` command instead of running it. `scripts/qa/stack-run_test.sh` covers the dry-run output with `levelrail-cli`, `curl` and `doctl` stubbed.

## Not covered offline

Real droplet creation and ssh readiness, the rsync resume path, the join command parsing in `enroll` (it expects a line starting with `sudo ` in the `nodes add` output), and the service restart units. Check these on the first real run.
