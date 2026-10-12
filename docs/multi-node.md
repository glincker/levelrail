---
title: "Multi-node: adding and managing nodes"
tag: "Beta"
description: Enroll additional nodes, manage placement and health, migrate apps with their volumes, and configure the WireGuard mesh.
---

<div className="flex flex-wrap items-center gap-2 not-prose"><span className="inline-flex h-9 w-9 items-center justify-center rounded-lg bg-white p-1.5 ring-1 ring-black/10"><img src="/assets/logos/wireguard.svg" alt="WireGuard" title="WireGuard" width="24" height="24" /></span></div>

Everything on this page is optional. A fresh install runs entirely on the control plane's own local node, and nothing here has to be touched to make it work.

## What this covers

| Task | Where |
| --- | --- |
| Add a worker (token, SSH, or cloud) | [Enrolling a second node](#enrolling-a-second-node) |
| Check whether a node is healthy | [Node health and heartbeat](#node-health-and-heartbeat) |
| Take a node out of rotation | [Cordon, drain, uncordon](#cordon-drain-uncordon) |
| Move an app or database | [Verified moves](#verified-moves) |
| Control where things run | [Labels and placement selectors](#labels-and-placement-selectors) |
| Turn on cross-node networking | [WireGuard mesh and internal DNS](#wireguard-mesh-and-internal-dns) |
| Fix an expiring or expired agent certificate | [Agent certificates: renewal and re-enrollment](#agent-certificates-renewal-and-re-enrollment) |

<Note>
The platform is single-node-first. The dashboard node picker only renders once a node besides the local one exists. A `node_id` of the empty string is a real, permanent value meaning "the local node". Auto-placement only picks a remote node when one is registered, schedulable and online.
</Note>

## Enrolling a second node

Enrollment exchanges a one-time join token for a client certificate. The agent dials out to the control plane, and the control plane never initiates a connection. For a cloud server created for you, see [Node provisioning](/node-provisioning). To have the control plane set up a machine over SSH, see [Enrolling over SSH](#enrolling-over-ssh-instead-of-by-hand).

<Warning>
Set the advertise host first. Before enrolling a real (non-local) node, set `APP_AGENT_ADVERTISE_HOST` on the control plane to the host or IP the remote agent uses in `APP_CONTROL_PLANE_ADDR`. It defaults to `127.0.0.1`, which only matches a single-machine test. With a mismatch, enrollment still succeeds (the first certificate exchange pins the CA fingerprint, not the hostname) and the node appears in `nodes list`, but its session then fails TLS hostname verification on every attempt and the node stays `pending` forever.
</Warning>

```mermaid
graph LR
    A["Mint join<br/>token"] --> B["Agent runs<br/>with token"]
    B --> C["Agent exchanges<br/>token for cert"]
    C --> D["Agent dials<br/>control plane"]
    D --> E["Heartbeat<br/>established"]
    E --> F["Node<br/>online"]
```

<Steps>
<Step title="Mint a join token">

<Tabs>
<Tab title="Dashboard">

Open the Nodes page and click **Add node**. The dialog shows the `docker` and `binary` commands with copy buttons, plus the expiry time.

</Tab>
<Tab title="CLI">

```bash
levelrail-cli nodes join-token
# or, in one command that also prints the enroll command:
levelrail-cli nodes add --name w2 --mesh
```

</Tab>
<Tab title="API">

```bash
curl -X POST -H "Authorization: Bearer $TOKEN" \
  https://control-plane.example.com/api/v1/nodes/join-tokens
```

</Tab>
</Tabs>

The token is single use and is returned in plaintext exactly once, because the server stores only its hash. It is valid for 15 minutes by default (`APP_NODE_JOIN_TOKEN_TTL`, clamped to 1 minute through 24 hours). If it expires unused, mint a new one. Tokens cannot be renewed.

The request body is optional:

| Field | Meaning |
| --- | --- |
| `control_plane_addr` | `host:port` the agent dials. |
| `node_name` | Lowercase letters, digits and dashes, starting with a letter. |
| `mesh` | Join the WireGuard mesh. |

When the server knows or was given the control plane address, the response carries `join_commands` with a ready-made `docker` command and `binary` command.

</Step>
<Step title="Run the agent on the new machine">

<Tabs>
<Tab title="Docker">

The agent image runs as root on purpose: it drives `/var/run/docker.sock` (root-equivalent on the host anyway) and creates a WireGuard device for the mesh.

```bash
sudo mkdir -p /var/lib/levelrail-agent-data && sudo chmod 700 /var/lib/levelrail-agent-data
docker run -d --name levelrail-agent --restart unless-stopped --network host \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v /var/lib/levelrail-agent-data:/var/lib/levelrail-agent-data \
  -e APP_CONTROL_PLANE_ADDR=controlplane.example.com:9443 \
  -e APP_JOIN_TOKEN=<token from step 1> \
  -e APP_CA_FINGERPRINT=<ca fingerprint from step 1> \
  -e APP_NODE_NAME=worker-1 \
  -e APP_AGENT_IDENTITY_FILE=/var/lib/levelrail-agent-data/identity.json \
  ghcr.io/glincker/levelrail-agent:beta
```

Add `--user 0:0` for images older than `v0.2.0-beta.16`, which defaulted to a non-root user that could not write into a root-owned directory.

</Tab>
<Tab title="Binary or package">

Install the `.deb` or `.rpm` packages attached to releases after `v0.2.0-beta.15` (see [Docker: without a container](/docker#without-a-container)), or run the binary as root. Releases before `v0.2.0-beta.16` have no agent binary asset, so use the image or a later release.

```bash
sudo APP_CONTROL_PLANE_ADDR=controlplane.example.com:9443 \
APP_JOIN_TOKEN=<token from step 1> \
APP_CA_FINGERPRINT=<ca fingerprint from step 1> \
APP_AGENT_IDENTITY_FILE=/var/lib/levelrail-agent/identity.json \
./levelrail-agent-linux-amd64
```

</Tab>
</Tabs>

| Variable | Meaning |
| --- | --- |
| `APP_CONTROL_PLANE_ADDR` | `host:port` of the control plane's agent gRPC listener. Required. |
| `APP_JOIN_TOKEN` | The token from step 1. Single use. |
| `APP_CA_FINGERPRINT` | The control plane's CA fingerprint, shown next to the token. Recommended: the agent then refuses to enroll, and never sends the token, unless the control plane's certificate chains to this CA. Without it, the agent trusts whatever answers at the address on first use. |
| `APP_NODE_NAME` | Optional. Defaults to the machine hostname. |
| `APP_AGENT_IDENTITY_FILE` | Where to save the identity, mode `0600`. Default `./levelrail-agent-identity.json`. |

On first run the agent generates its own private key, sends only a certificate signing request with the join token, and saves the signed certificate, its key and the control plane's CA certificate locally. The private key never leaves the machine. On later runs it skips enrollment and reconnects with the saved certificate, which it renews on its own (see [Agent certificates](#agent-certificates-renewal-and-re-enrollment)).

The agent checks that the identity directory is writable before it sends the token. If it is not, it exits naming the directory and says the token was not used, so you can fix permissions and retry with the same token.

</Step>
<Step title="Confirm it registered">

```bash
levelrail-cli nodes list
```

The node appears as soon as enrollment is saved, with `status: pending` until its first heartbeat, then `online`.

</Step>
<Step title="Use it for placement">

Once connected, the node is a normal placement target. Pick it in an app or database create form, move an existing resource onto it, or let [auto-placement](#simple-spread-placement-auto-placement) send new resources there.

A node accepts app workloads by default but not build workloads. See [Build node routing](/build-node-routing) to enable builds.

</Step>
</Steps>

<Accordion title="Node stuck in pending (never connected)">

A node `pending` for more than five minutes (`APP_NODE_PENDING_STALE_AFTER`) is flagged "Never connected" in the dashboard, and `nodes list` prints `pending (never connected)` with the next step. The token was spent but the agent never opened a session, so it cannot be retried.

1. Check the agent logs on the host. The usual causes are an unwritable identity directory or a wrong `APP_AGENT_ADVERTISE_HOST`.
2. Delete the node with `levelrail-cli nodes delete <id>`.
3. Enroll again with a new token.

</Accordion>

## Enrolling over SSH instead of by hand

Steps 1 and 2 above can be automated: the control plane SSHes into the machine, mints the token and installs the agent for you.

<Tabs>
<Tab title="Dashboard">

Nodes page, **Add node**, then **Connect over SSH**. After the host and port, the **Verify server** step shows the server's host key fingerprints. Confirm one before the credential fields appear. The wizard shows each stage plus a live log tail.

</Tab>
<Tab title="CLI">

```bash
levelrail-cli nodes ssh-provision \
  --host 192.0.2.10 --user root --key-file ~/.ssh/id_ed25519 \
  --name home-server --control-plane-addr controlplane.example.com:9443
```

| Flag | Meaning |
| --- | --- |
| `--key-file` | Private key for key auth (with `--passphrase` if encrypted). |
| `--password` | Password auth instead of a key, prompted without echo. |
| `--port` | Non-default SSH port (default 22). |
| `--role build` | Make it a build node (default `general`). |
| `--host-key-fingerprint SHA256:...` | Pin the host key non-interactively, for scripts. |
| `--insecure-skip-host-key-check` | Skip verification entirely (warned and audited). |

Without a pin flag, the command prints the server's host key fingerprints and asks you to confirm before it reads any credential.

Track a run with `levelrail-cli nodes ssh-provisions show <id>`.

</Tab>
<Tab title="API">

`POST /api/v1/nodes/ssh-provision` accepts a host, port (default 22), username, either a private key (optionally passphrase-protected) or a password, and the server's host key fingerprint. Poll `GET /api/v1/ssh-node-provisions/{id}`, which carries the accumulated install log and the detected OS and architecture.

Status moves through `connecting`, `detecting`, `installing`, `enrolling`, `ready`, or `failed` with a reason.

</Tab>
</Tabs>

What it does in the background:

1. Verifies the host key against the pinned fingerprint, connects, and detects the OS, architecture, and whether Docker and systemd are present. Only Linux with systemd is supported (the same requirement `install.sh` has). An unsupported host fails here with a clear reason before anything is changed.
2. Installs Docker via `get.docker.com` if it is missing.
3. Writes the agent's environment file and a systemd unit, then enables and starts it (the same steps cloud-init runs on a new VM, see [Node provisioning](/node-provisioning)).
4. Confirms the service stays active, surfacing the last `journalctl` lines as the failure reason if it does not.

The SSH credential is held only in memory for this one call and is never written to the database or logged. The join token is written to a root-only (`0600`) environment file on the target host. Tokens minted for SSH enrollment are valid for 30 minutes, and for cloud provisioning 60 minutes, regardless of `APP_NODE_JOIN_TOKEN_TTL`.

### Host key verification

Credentials only go to a server whose host key the operator has seen (trust on first use with explicit confirmation).

<Steps>
<Step title="Read the host keys">

`POST /api/v1/nodes/ssh-provision/preflight` with `{"host": "...", "port": 22}` reads the server's host keys without authenticating: the handshake is aborted right after key exchange, so no username, password or key is sent. It returns each key's type and SHA256 fingerprint, the same format `ssh-keygen -l` prints:

```json
{"host_keys": [{"type": "ssh-ed25519", "fingerprint": "SHA256:..."}]}
```

</Step>
<Step title="Compare with the server">

```bash
ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub
```

Or use your provider's console output.

</Step>
<Step title="Send the confirmed value back">

Pass it as `host_key_fingerprint` on `POST /api/v1/nodes/ssh-provision`. The install refuses to authenticate to a server presenting any other key, and the failure names both the expected and observed fingerprints. A fingerprint you already know (pinned out of band) works without calling the preflight.

</Step>
</Steps>

The request is rejected with `400` when it carries neither a fingerprint nor `insecure_skip_host_key_check: true`. That explicit opt out accepts any key, returns a warning in `warnings`, and writes a `node_ssh_provision.host_key_skipped` audit entry. The pinned fingerprint is stored with the provision (`host_key_fingerprint`, `host_key_verified`). Both routes need the `root` ability and are audited.

## Node health and heartbeat

![Levelrail nodes list showing node health and placement](assets/screenshots/nodes.png)

```bash
levelrail-cli nodes health <id>
```

This calls `GET /api/v1/nodes/{id}/health`: it returns the stored `Heartbeat` condition and re-checks any node-scoped alerts (patch status, disk space, resource usage). A node that enrolled but never connected shows `NeverConnected`, not an error.

### How heartbeats work

A connected agent sends a `Heartbeat` frame up its Session stream at a regular interval (default 15 seconds, `APP_NODE_HEARTBEAT_INTERVAL`, read agent-side). The control plane only touches `last_seen_at` when one arrives, never merely because the stream is still open.

| Failure | What happens |
| --- | --- |
| Graceful disconnect | The agent exits cleanly and the node flips to `offline` immediately. |
| Hard disconnect | The `internal/reconcile/nodehealth` controller compares `last_seen_at` against `APP_NODE_HEARTBEAT_TIMEOUT` (default 45 seconds) on every pass. A node past it that is still `online` flips to `offline` with a `Heartbeat` condition explaining why. |
| Frozen process (for example `SIGSTOP`) | The TCP and TLS connection can stay open indefinitely. The HTTP/2 PING keepalive is the backstop: the transport notices the peer stopped responding, ends the connection, and the hard-disconnect path follows. |

The keepalive is configured with `APP_NODE_KEEPALIVE_TIME` and `APP_NODE_KEEPALIVE_TIMEOUT` (default 10s/10s on the control plane; the agent mirrors this with env vars of the same name).

### The control plane's local node

On the Network page the control plane's own server is always the first card, **This server (control plane)**, whether or not the mesh is on. Apps and databases with an empty placement, or placed on the control plane's own node ID, belong to it. The **Unplaced** card only appears for resources placed on a node ID the control plane does not know.

The local node uses the same health system: it heartbeats itself rather than through gRPC, and is never permanently `online` by fiat.

<Note>
Cordon is tracked as a separate `schedulable` flag, not a status value. A node can be `online` and cordoned, or `offline` and schedulable.
</Note>

### Connection history

Every node status change (online, offline, cordoned) is recorded, the newest 200 per node. See it on the node detail page's "Connection history" card, or from the CLI:

```bash
levelrail-cli nodes events <id> [--limit N]
```

`GET /api/v1/nodes/{id}/events?limit=N` returns the same list, newest first.

## Cordon, drain, uncordon

**Cordon** marks a node unschedulable for new placements without moving anything already running. **Drain** moves everything off the node, using the same [verified moves](#verified-moves) described below.

```bash
levelrail-cli nodes cordon <id>
levelrail-cli nodes uncordon <id>
levelrail-cli nodes drain <id> [--target <node-id>] [--wait]
levelrail-cli nodes drains <id>
```

### Cordon / uncordon

`POST /api/v1/nodes/{id}/cordon` and `.../uncordon` are idempotent. A cordoned node is excluded from auto-placement and refused as an explicit placement target (`400`).

### Drain

`POST /api/v1/nodes/{id}/drain?target_node_id=` starts a drain job and returns `202` with a `NodeDrain`. The node is cordoned immediately, and a second drain on a node that is already draining returns `409`.

| Aspect | Behavior |
| --- | --- |
| Order | Databases move first, then apps. An app follows its database's node when that node is eligible. |
| With `--target` | Everything goes to the specified node. |
| Without `--target` | Each resource goes to the least loaded eligible node. |
| Execution | Each resource is a verified move, run one after another. |
| On failure | The drain stops at the first failed move and leaves the node cordoned. Fix the cause, then drain again or uncordon. |
| GPU apps | Only move to a node with a working nvidia runtime and enough free GPUs. An app no node can host stays put and is listed under `blocked` with a per-node reason. See [GPU scheduling](/ai-models#gpu-scheduling). |
| Models | Cannot be moved, so any model on the node is always listed as blocked. |

Progress is a job: poll `GET /api/v1/node-drains/{id}`, or use `nodes drain <id> --wait`. `nodes drains <id>` and `GET /api/v1/nodes/{id}/drains` list the newest 20.

A `NodeDrain` carries `status` (`running`, `succeeded`, `failed`), an `error`, the `blocked` list and the `moves` in the order they ran. The node page's **Drain** dialog polls it every couple of seconds and lists each move with its phase; the **Drains** card keeps the history.

### Deleting a node

`DELETE /api/v1/nodes/{id}` (CLI: `nodes delete <id>`)

- Refused with `409` while the node still has anything placed on it. Drain first.
- Otherwise idempotent: deleting an already-deleted node ID succeeds.
- Closes the node's live agent session, so a deleted node's agent is disconnected.

## Simple spread placement (auto-placement)

When you create an app or database without specifying a node, the server decides placement.

- `APP_AUTO_PLACEMENT` (default enabled): if `false`, always place on the local node.
- If enabled, `autoPlaceNode` picks the schedulable, online node with the fewest resources (apps plus databases). Ties go to the lexicographically smallest node ID.
- With no eligible remote node, it falls back to the local node.
- An explicit `node_id` (or an explicit empty string meaning "local, on purpose") always overrides auto-placement and is validated against cordoned and unknown-node checks.
- Create responses carry `auto_placed: true` with the `node_id` picked, and the dashboard shows a toast, so it is never a silent decision.

<Warning>
This is simple spread counting, not bin-packing. It counts resources only, never CPU, memory or disk headroom.
</Warning>

GPU apps: a new app with `resources.gpu` is only auto-placed on a node with a working nvidia runtime and enough free GPUs (least loaded among those), falling back to the local host if it fits. If none fits, the create is refused with `409` and the reason per node; pass `node_id` to override. See [GPU scheduling](/ai-models#gpu-scheduling).

## Verified moves

A move relocates one app or database to another node, checking each step and rolling back when the target cannot run it.

<Tabs>
<Tab title="CLI">

```bash
levelrail-cli nodes move app web --to <node-id> --wait
levelrail-cli nodes move database main --to <node-id> --wait
```

Omit `--to` to let the scheduler pick the least loaded eligible node.

</Tab>
<Tab title="Dashboard">

The **Move** button on the app or database **Placement** card runs the same move and shows each phase live, with an **Automatic** target option.

</Tab>
<Tab title="API">

`POST /api/v1/apps/{name}/move` and `POST /api/v1/databases/{name}/move` with `{"node_id": "..."}` (an empty `node_id` means automatic). They return `202` with a `NodeMove`. Poll `GET /api/v1/node-moves/{id}`, or list with `GET /api/v1/node-moves?kind=&resource=&drain_id=&status=&limit=`.

</Tab>
</Tabs>

Errors: `400` (same node, label mismatch, cordoned or offline target, no eligible node), `404`, `409` (that resource is already moving) and `501` (not configured).

### Phases

A move walks `preflight`, `stop_source`, `copy_volumes`, `place`, `verify`, `stop_old`, `finalize`, then `done`. If something fails after the source was touched it enters `rollback`. The final `status` is `succeeded`, `failed` or `rolled_back`, and every step records its outcome (`ok`, `retry`, `waiting` or `failed`).

- Stateless (no volumes): stage, verify, switch, stop. The new copy is started and checked on the target before the old one is stopped, so there should be no downtime.
- Stateful (volumes, and every database): stop, copy, start. Downtime lasts until the copy finishes.

While the target comes up, ingress holds incoming requests instead of failing them.

### Tuning and guarantees

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_NODE_MOVE_VERIFY_TIMEOUT` | `5m` | How long the target has to become healthy before the move rolls back. |
| `APP_NODE_MOVE_MAX_ATTEMPTS` | `5` | Attempts for a retryable step. |
| `APP_NODE_MOVE_INTERVAL` | `3s` | Delay between attempts. |

- Moves resume after a control plane restart, and there is one move per resource at a time.
- Apps with host bind mounts are refused, because only Docker named volumes can be copied.
- A database moves with its data volume (and its WAL archive when point-in-time recovery is on). Apps that depend on it are restarted afterwards.

## Labels and placement selectors

Label nodes, then restrict where an app or database may run with a selector.

```bash
levelrail-cli nodes labels <id> disk=ssd
levelrail-cli nodes placement app web disk=ssd
levelrail-cli nodes placement database main disk=ssd
```

- `GET` and `PUT /api/v1/nodes/{id}/labels` read and replace a node's whole label set. Keys are lowercase letters, digits, `.`, `_`, `-` and `/`, 1 to 63 characters. Values are letters, digits, `.`, `_` and `-`, 1 to 63 characters. At most 32 labels. Invalid input returns `400` with the reason.
- `GET` and `PUT /api/v1/apps/{name}/placement` (and the same for `databases`) read and set a `node_selector`. The response lists `eligible_node_ids` and the `rejected` nodes with a reason each. Setting a selector never moves anything by itself.
- The dashboard has a **Labels** card on the node page and a **Placement** card on the app and database pages.

### The spread scheduler

When a move or a drain has no explicit target, the scheduler picks the least loaded eligible node, breaking ties by node id. A node is not eligible when it is cordoned, offline, or missing a label the selector requires.

## Moving an app with its volumes

<Tabs>
<Tab title="Simple move">

`PUT /apps/{name}/node` (CLI: `levelrail-cli apps set-node <name> <node-id>`) changes only `node_id`. The reconciler creates fresh empty volumes on the new node and the old volumes stay behind. Fine for stateless apps, wrong for apps with state.

</Tab>
<Tab title="Move with volumes">

`POST /api/v1/apps/{name}/move-with-volumes` does a proper migration.

- Dashboard: the "Take its volumes with it" checkbox. The dialog previews the plan before you confirm: stop the app, copy each named volume, switch placement, start it on the destination, with the expected downtime and rollback story spelled out.
- CLI: `levelrail-cli apps set-node <name> <node-id> --with-volumes`. It polls `GET /apps/{name}/moves/{id}` until complete, then prints the finished move record (or the failure reason and the step it reached). `apps clear-node <name> [--with-volumes]` is the counterpart for returning to the local node.

</Tab>
</Tabs>

### Steps

1. Stop: the app is suspended and its containers torn down on the current node (synchronously), so nothing writes to volumes during archival.
2. Move each volume: every named volume is tarred from the source node and untarred on the destination, one at a time, over the same transport volume backups use. No S3 bucket is needed, because both nodes' Docker daemons are directly reachable from the control plane.
3. Update placement: only after all volumes copy successfully does `node_id` change.
4. Resume: the suspension clears and the reconciler starts the app on the new node with the already-populated volumes.

### Tracking progress

Every step is recorded on a `store.AppVolumeMove` row. Poll with `GET /apps/{name}/moves/{id}` or check history with `GET /apps/{name}/moves`. If step 2 fails on the second of three volumes, the row shows:

- `stop_app: succeeded`
- `move_volume:app-<name>-cache: succeeded`
- `move_volume:app-<name>-uploads: failed`
- (nothing past that)

<Warning>
This is not atomic and has no automatic health gate or rollback. If any step fails, the app is left suspended (stopped) and `node_id` never changes until all volumes have copied. Retrying is safe: archiving is read-only and restoring is a full overwrite. Clean up unreferenced volumes on the destination by hand if needed.
</Warning>

<Accordion title="Special cases">

- No named volumes, or already on the destination node: takes the simple move path. The response comes back `status: "succeeded"`, no polling needed.
- Bind mounts (`app.yaml` host-path mounts): never moved by this path. Archive and restore primitives exist only for Docker-managed named volumes.

</Accordion>

## Viewing what's placed on a node

There is no dedicated "workloads on this node" list. These show it indirectly:

- Network page and `levelrail-cli nodes topology`: every node with the apps and databases placed on it. See [Network topology](/network-topology).
- App and database detail pages: a Placement card shows the current node, the selector, eligible nodes and the move action.
- Placement names: an app or database with no explicit node shows "This server" plus the hostname rather than a blank. API responses carry `node_name` and `node_is_local` beside `node_id`. "Unassigned" appears only when the stored node no longer exists, with a "Move to this server" action.
- Nodes page: one row per node with status, usage bars, resource count, agent version and last seen, and a menu for cordon, drain and delete (delete asks you to type the node name).
- Drain and delete: both enumerate everything on the node server-side, which is why deleting a node with placements fails instead of orphaning them.

## WireGuard mesh and internal DNS

An app's database connection string is baked into the container environment at creation time, so it does not rewrite itself when a database moves. The fix is DNS names from the start. `internal/reconcile/mesh` keeps the mapping current: every pass it reads node and placement data, distributes WireGuard configuration, and rebuilds the internal DNS zone (`<brand-short-name>.internal`, for example `levelrail.internal`).

When mesh is enabled, database env vars (like `DATABASE_URL`, or any `{ from: postgres.main.url }` reference in `app.yaml`) automatically resolve to the database's mesh DNS name, so an app on one node can connect to a database on another with no app-side changes. Without mesh, they resolve to the database container's Docker name, reachable only within that node's Docker network.

### Enabling the mesh on the control plane

Turn it on from **Network > Mesh networking** in the dashboard, or with the CLI (default: off):

```bash
levelrail-cli network mesh status     # state plus the readiness checks
levelrail-cli network mesh enable     # root only; refused while a check fails
levelrail-cli network mesh disable    # --force when other nodes are enrolled
```

The readiness list checks WireGuard support (kernel module, or wireguard-go with root and `/dev/net/tun`), that the UDP port is free, whether `ufw` is blocking it, and whether a public address is set for agents to dial. A failing check blocks enabling and shows a fix hint. The choice is saved and applies on the next control plane restart. Setting `APP_MESH_ENABLED` on the server pins the mesh on or off and wins over the saved choice (the card then shows it as read-only). A mesh setup failure is logged and the control plane still starts.

Enabling brings up the control plane's own WireGuard device, interface and mesh address, its own DNS server, and a self-peer entry for the local node.

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_MESH_ENABLED` | off | Set to `1` to enable the mesh. |
| `APP_MESH_CIDR` | `10.0.0.0/8` (picked automatically) | WireGuard mesh subnet. An invalid override is logged as a warning and the default is used. |
| `APP_MESH_DNS_ADDR` | `:5390` | Bind address of the internal DNS server. |

<Accordion title="Single-node DNS caveat: containers do not query port 5390">

Docker container DNS accepts only a bare IP address, never a custom port. With the default `:5390`, no container actually queries the mesh DNS server and containers fall back to Docker's embedded resolver. To make containers use it:

1. Set `APP_MESH_DNS_ADDR=:53`.
2. Grant the process permission to bind port 53 (for example via `setcap` on Linux).
3. Restart the control plane.

Mesh failures (disabled, DNS not on 53, and so on) log warnings but never break anything.

</Accordion>

### Multi-node mesh requirements

The control plane sends `ApplyMesh` and `RotateMeshKey` requests to each enrolled agent, and the agent applies them to its own WireGuard device when started with `APP_MESH_ENABLED=1`. This has been exercised against fakes and a single host with a real TUN device, not between two real hosts, so treat cross-host mesh as lightly proven.

<Steps>
<Step title="Give every agent the privileges">

Run as root with `--cap-add NET_ADMIN --device /dev/net/tun -e APP_MESH_ENABLED=1` (the image already runs as root). Nothing else is needed: no extra sysctls and no `ip` binary. Without these the agent keeps serving containers and logs `mesh networking disabled on this node`. Nodes enrolled through the dashboard's SSH or cloud provisioning flows get all of this automatically when the control plane itself runs with `APP_MESH_ENABLED=1`.

</Step>
<Step title="Open the hub">

The control plane is the hub: agents send WireGuard handshakes to `APP_AGENT_ADVERTISE_HOST` on UDP `51820`. Set that variable to the control plane's public host, and allow **inbound UDP 51820** on its firewall and cloud security group. A loopback advertise host leaves agents with no endpoint (`no known endpoint for peer`); `levelrail-cli doctor` reports this as `mesh_hub_endpoint`.

</Step>
<Step title="Match the brand">

The interface name comes from the brand short name. The agent image carries the default `brand.yaml`; override with `APP_BRAND_SHORT_NAME` (or mount your own at `APP_BRAND_FILE`) if you rebrand the control plane.

</Step>
</Steps>

Worker to worker WireGuard endpoints are recorded from where each agent connects, on UDP `51820`, so workers reach each other directly without extra configuration.

### Viewing mesh status

<Tabs>
<Tab title="CLI">

```bash
levelrail-cli nodes mesh
```

</Tab>
<Tab title="API">

```bash
curl -H "Authorization: Bearer $TOKEN" \
  https://control-plane.example.com/api/v1/mesh
```

</Tab>
</Tabs>

| Field | Meaning |
| --- | --- |
| Backend | `kernel` (WireGuard kernel module), `userspace` (wireguard-go fallback), or `disabled`. |
| Interface | The WireGuard device name, derived from the brand short name (for example `levelrail0`), on the control plane and every agent. |
| Mesh address | The local node's assigned IP in the mesh. |
| Public key | The local node's WireGuard public key. |
| Last rotation | Timestamp and state if a key rotation is in progress (confirming or confirmed). |
| Peers | One per enrolled node: mesh address, last handshake, health status, and whether it has a live device entry. |

A peer with `live: false` is registered in the node inventory but the mesh device has no live entry yet (the reconciler has not reached it this pass, or peering has not converged).

### Pair diagnostics

```bash
levelrail-cli nodes connectivity
```

`GET /api/v1/mesh/connectivity` returns every node's WireGuard view of every other node as pairs, each `ok`, `degraded`, `down` or `unknown`, with a per-direction link state (`healthy`, `stale`, `never`, `no_peer`, `unknown`), last handshake, endpoint and a `hint` for a bad pair. It returns `501` when the mesh is off. The Network page shows the same matrix under **Node connectivity**.

### Rotating a node's mesh key

Generates a fresh WireGuard keypair for a node and makes it that node's live mesh identity immediately. Works for any node with a live session, local or remote.

<Tabs>
<Tab title="CLI">

```bash
levelrail-cli nodes rotate-key <node-id>
```

</Tab>
<Tab title="API">

```bash
curl -X POST -H "Authorization: Bearer $TOKEN" \
  https://control-plane.example.com/api/v1/nodes/<node-id>/mesh/rotate-key
```

</Tab>
</Tabs>

- The new public key is recorded and the mesh reconciler propagates it to every peer on its next pass.
- Watch the `rotation` field in `levelrail-cli nodes mesh` to see when every reachable peer has caught up. A brief reconnect blip on mesh traffic is possible until then.
- A remote node with no live agent session fails the rotation. Both routes return `501` when mesh networking is not enabled.

If a peer looks stuck, `levelrail-cli nodes rejoin-mesh <node-id>` (`POST /api/v1/nodes/{id}/mesh/rejoin`) forces an immediate mesh reconcile. It is a fleet-wide resync, not a per-node operation.

### Cross-node database connections

With the mesh on, an app on one node can use a database on another.

- The app's `DATABASE_URL` uses `<db>.<zone>:<mesh port>`.
- The name is pinned in the container's `/etc/hosts` to the database node's mesh address, so no DNS server has to be reachable from the container.
- Mesh ports are allocated from `APP_MESH_DB_PORT_MIN` to `APP_MESH_DB_PORT_MAX` (default `21000` to `21999`).
- A database is published on its node's mesh address only once an app on another node uses it, and never on a wildcard or public address.
- Without the mesh, a move that would split an app from its database is refused, except inside a drain.

### Routing to apps on remote nodes

<Note>
The embedded Caddy ingress ([Domains and ingress](/domains-and-ingress)) runs in the control plane's own process. A worker node has **no public listener**: nothing is bound to 80/443 there. Ingress reaches an app on a worker through that node's mesh address, so its domains and its `<app>.<ip>.sslip.io` hostname work with normal TLS.
</Note>

How a remote app is reached:

1. The app controller publishes the container's main port on the worker's **mesh address** (for example `10.181.0.2:32768`), never on `0.0.0.0` and never on a public interface.
2. The ingress controller dials `<mesh-ip>:<host-port>` and obtains the TLS certificate for the hostname as usual (ACME for custom domains and sslip.io names, or your uploaded certificate).
3. The path counts as healthy when the node has a mesh address and the control plane's WireGuard device has a handshake with it from the last 3 minutes.

How this interacts with `bind_address` and `host_port`:

| App setting on a remote node | Published on the worker as |
| --- | --- |
| `private` (default) | the node's mesh IP while the path is healthy, else `127.0.0.1` |
| `public` | `0.0.0.0` (your explicit choice, also reachable over the mesh) |
| a literal IP | that IP, unchanged (ingress routes only if it is the mesh IP or a wildcard) |
| `host_port` pinned | the pinned port on the same address as above |

Only the app's main port moves. App streams, [managed databases](/managing-databases) and every unpublished container port are untouched: a database is never bound to the mesh address by this feature.

<Warning>
If the mesh path is down (mesh off, node not joined, no recent handshake), the app is **not** routed: a browser gets a TLS handshake failure while the app reports healthy. Levelrail reports this as the `NoMeshIngressPath` reason on the app's `CrossNodeIngress` condition, in `levelrail-cli doctor` (`cross_node_ingress`), in preflight and on the Traffic page, each with the cause and a fix.
</Warning>

```bash
levelrail-cli nodes mesh                                              # see which peer has no handshake
levelrail-cli apps set-node <name> <the control plane's own node id>  # or move it back
```

The condition clears on the next reconcile pass after the mesh is healthy. Typical causes:

| Reason shown | Fix |
| --- | --- |
| `mesh networking is off` | Enable it on the Network page (or `levelrail-cli network mesh enable`) and restart the control plane. |
| `the node has not joined the mesh yet` | Check `levelrail-cli nodes mesh`. |
| `no recent WireGuard handshake` | Open UDP 51820 between the hosts. |
| `publishes its port on loopback only` | Transient: the app controller republishes it on its next pass. |

<Accordion title="The mesh came up after the app was deployed">

The reconciler is level-triggered: when it sees a running container whose main port is still on loopback and the path is now healthy, it removes that container and recreates it with the mesh bind (a brief restart; the app was unreachable through ingress anyway). A mesh flap never recreates a container: once bound, it stays bound while the path is down, and the condition reports the outage.

This does not move databases or app streams onto the mesh, and auto-placement still prefers the control plane's own node for apps created with a domain.

</Accordion>

## Agent certificates: renewal and re-enrollment

Each agent authenticates with a client certificate issued by the control plane's own CA (90 days by default, `APP_AGENT_CERT_VALIDITY` on the control plane). The design and its rejected alternatives are in [ADR 021](https://github.com/glincker/levelrail/blob/main/adr/021-agent-cert-lifecycle.md).

### Seeing expiry

| Where | What you get |
| --- | --- |
| Dashboard | Every node row shows "Cert expires in N days", amber inside the warning window and red once critical, expired or revoked. The node page's Agent card shows expiry, last renewal, key origin, fingerprint, agent version, platform and commit. |
| CLI | `nodes list` has `CERT` and `AGENT` columns; `nodes get` shows the details. |
| API | `GET /api/v1/nodes` and `GET /api/v1/nodes/{id}` carry `cert` (`state` is `ok`, `expiring`, `critical`, `expired`, `revoked` or `unknown`, plus `days_remaining`, `not_after`, `renewed_at`, `generation`, `key_origin`) and `agent` (`version`, `commit`, `os`, `arch`, `outdated`). |
| Alerts | A `node_cert_expiring` rule fires while any node's certificate is inside `APP_NODE_CERT_EXPIRY_WARNING` (default `504h`, 21 days; a rule's `for_duration` overrides it) or has expired. `APP_NODE_CERT_EXPIRY_CRITICAL` (default `168h`) sets when the badge turns red. |
| Attention | Expiring, expired and revoked certificates and outdated agents appear on the status page and in `levelrail-cli attention`. |

Healthy agents renew with about 30 days left, so either alert firing means renewal is failing. Existing nodes get an estimated expiry (enrollment time plus 90 days) until they next connect.

### Automatic renewal

When a certificate is two thirds of the way through its lifetime (`APP_AGENT_CERT_RENEW_FRACTION` on the agent, default `0.67`, plus random jitter so nodes enrolled together do not renew together), the agent generates a fresh key and asks the control plane to sign it over its existing authenticated connection. Nothing needs restarting.

1. The control plane signs the request and records the new certificate. The previous one stays accepted for a grace window (`APP_AGENT_CERT_RENEW_GRACE`, default `24h`), so a lost response or a control plane restart never locks the node out.
2. The agent writes the new identity next to the old one (temp file, fsync, rename, the old one kept as `<identity file>.prev`) and tests it with a separate connection.
3. If the test passes the agent switches over and drops the backup. If it fails it restores the old identity and retries with backoff (1 minute doubling to 1 hour).

An agent stopped mid-renewal settles it on its next start: it keeps whichever identity the control plane accepts.

<Note>
Nodes enrolled before agents generated their own keys keep working, and their first renewal moves them to an agent-generated key. Older agents still enroll against a newer control plane and get a server-generated key. The control plane now refuses that by default. Set `APP_AGENT_REQUIRE_CSR=false` on the control plane to accept legacy agents until they are upgraded.
</Note>

### Re-enrolling a node

A node that was offline past its certificate's expiry, or whose certificate was revoked, cannot renew. Re-enroll it instead; it keeps its node ID, placements and history.

<Steps>
<Step title="Mint a re-enroll token">

Dashboard: open the node and click **Re-enroll node**. CLI: `levelrail-cli nodes reenroll-token <id>`. Both mint a single-use token bound to that node, valid for 15 minutes, and show the command once.

</Step>
<Step title="Run the command on the node">

```bash
APP_CONTROL_PLANE_ADDR=<control-plane-host>:9443 \
APP_REENROLL_TOKEN=<token> \
APP_CA_FINGERPRINT=<fingerprint> \
./levelrail-agent reenroll
```

The agent verifies the control plane against the CA in its existing identity file (or the fingerprint when the file is gone), generates a new key, and saves the new identity. A running agent picks it up at its next reconnect, so no restart is needed; a stopped agent just needs starting.

</Step>
</Steps>

A token minted for one node cannot re-enroll another, and a join token cannot be used to re-enroll. When the agent sees its certificate expired or refused, it logs the exact re-enroll command instead of retrying in a tight loop.

### Revoking a certificate

**Revoke certificate** on the node page, `levelrail-cli nodes revoke-cert <id>`, or `POST /api/v1/nodes/{id}/revoke-cert` makes the control plane refuse the node's certificate and closes its live session immediately.

<Warning>
Workloads already on the node keep running but can no longer be managed. Only a re-enroll token brings the node back.
</Warning>

### Agent version

Every agent reports its version, commit, OS and architecture when its session opens. Set `APP_AGENT_MIN_VERSION` on the control plane (for example `v0.9.0`) to flag older agents, and agents too old to report a version, as outdated in the node list, node page, CLI and attention list. Unset, no agent is flagged. Upgrading agents is still manual: stop the agent, replace the binary, start it again.

## Node-level metrics

```bash
levelrail-cli nodes metrics <id> --metric cpu_percent
```

`GET /api/v1/nodes/{id}/metrics` returns summed container metrics, not host metrics, except for disk.

| Metrics | Source |
| --- | --- |
| `cpu_percent`, `memory_usage_bytes`, `network_rx_bytes`, `network_tx_bytes`, `disk_read_bytes`, `disk_write_bytes` | The *sum* of every placed app service's per-container samples. Not a read of the host's free or total memory or CPU. |
| `disk_used_bytes`, `disk_total_bytes` | Real per-node host readings written by `HostDiskCollector`. |

- `memory_limit_bytes` is excluded because summing unconstrained container limits would multiply the host's memory by N.
- The dashboard labels summed metrics with "summed across N containers".
- `resource_count` is how many placed services contributed a sample in the queried range, a rough occupancy signal.
- Databases placed on a node are excluded from the sum (only app services are included). Including them is separate future work.

### Fleet-wide utilization

```bash
levelrail-cli nodes resource-usage
levelrail-cli nodes capacity-forecast <id>   # projects when disk or memory runs out
```

`GET /api/v1/nodes/resource-usage` is the fleet-wide counterpart: one snapshot with every node's latest CPU, memory and disk reading plus a rollup, read by the node list's CPU/Memory/Disk columns and the dashboard's fleet summary card. The same caveats apply: CPU and memory are summed from containers, and disk is a real host reading that today is populated only for the node running the control plane. See [Observability](/observability) for the response shape.

## OS patch status

```bash
levelrail-cli nodes patch-status <id>
```

`GET /api/v1/nodes/{id}/patch-status` reads the latest OS patch sample. Collection runs every `APP_OS_PATCH_CHECK_INTERVAL` (default 1 hour), with a lookback of up to 48 hours to cover slow or just-restarted collectors.

| Response | Meaning |
| --- | --- |
| `checked: false` | No sample yet, no supported package manager detected, or the collector has not run. |
| `checked: true, total: 0` | Genuinely up to date. |
| `checked: true, total: N` | Updates available, with a separate `security` count (the number worth acting on urgently). |

The dashboard renders this as a single status badge, not a chart, since it is one current fact.

## API reference

Node routes require the `root` ability, not `read` or `write`, because node management is control-plane administration.

| Method | Path | Ability |
| --- | --- | --- |
| `GET` | `/api/v1/nodes` | `root` |
| `GET` | `/api/v1/nodes/{id}` | `root` |
| `DELETE` | `/api/v1/nodes/{id}` | `root` |
| `PUT` | `/api/v1/nodes/{id}/workloads` | `root` |
| `PUT` | `/api/v1/nodes/{id}/region` | `root` |
| `POST` | `/api/v1/nodes/join-tokens` | `root` |
| `POST` | `/api/v1/nodes/provision` | `root` |
| `GET` | `/api/v1/node-provisions` | `root` |
| `GET` | `/api/v1/node-provisions/{id}` | `root` |
| `POST` | `/api/v1/nodes/ssh-provision` | `root` |
| `POST` | `/api/v1/nodes/ssh-provision/preflight` | `root` |
| `GET` | `/api/v1/ssh-node-provisions` | `root` |
| `GET` | `/api/v1/ssh-node-provisions/{id}` | `root` |
| `GET` | `/api/v1/nodes/{id}/health` | `root` |
| `POST` | `/api/v1/nodes/{id}/cordon` | `root` |
| `POST` | `/api/v1/nodes/{id}/uncordon` | `root` |
| `POST` | `/api/v1/nodes/{id}/drain?target_node_id=` | `root` |
| `GET` | `/api/v1/nodes/{id}/metrics?metric=&from=&to=&step=` | `root` |
| `GET` | `/api/v1/nodes/{id}/patch-status` | `root` |
| `GET` | `/api/v1/nodes/{id}/events` | `root` |
| `POST` | `/api/v1/nodes/{id}/mesh/rotate-key` | `root` |
| `POST` | `/api/v1/nodes/{id}/mesh/rejoin` | `root` |
| `GET` | `/api/v1/nodes/resource-usage` | `root` |
| `GET` | `/api/v1/nodes/{id}/capacity-forecast` | `root` |
| `GET` | `/api/v1/network/topology` | `read` |
| `POST` | `/api/v1/nodes/{id}/reenroll-token` | `root` (scoped to `node:{id}`) |
| `POST` | `/api/v1/nodes/{id}/revoke-cert` | `root` (scoped to `node:{id}`) |
| `GET` | `/api/v1/mesh` | `root` |
| `GET` | `/api/v1/mesh/connectivity` | `root` |
| `POST` | `/api/v1/apps/{name}/move` | `root` |
| `POST` | `/api/v1/databases/{name}/move` | `root` |
| `GET` | `/api/v1/node-moves?kind=&resource=&drain_id=&status=&limit=` | `root` |
| `GET` | `/api/v1/node-moves/{id}` | `root` |
| `GET` | `/api/v1/nodes/{id}/drains` | `root` |
| `GET` | `/api/v1/node-drains/{id}` | `root` |
| `GET` | `/api/v1/nodes/{id}/labels` | `root` |
| `PUT` | `/api/v1/nodes/{id}/labels` | `root` |
| `GET` | `/api/v1/apps/{name}/placement` | `root` |
| `PUT` | `/api/v1/apps/{name}/placement` | `root` |
| `GET` | `/api/v1/databases/{name}/placement` | `root` |
| `PUT` | `/api/v1/databases/{name}/placement` | `root` |
| `PUT` | `/api/v1/apps/{name}/node` | `root` |
| `POST` | `/api/v1/apps/{name}/move-with-volumes` | `root` |
| `GET` | `/api/v1/apps/{name}/moves` | `read` |
| `GET` | `/api/v1/apps/{name}/moves/{id}` | `read` |

`GET /api/v1/nodes/{id}` also carries an `alert_status` field when telemetry is configured, a live re-evaluation of that node's patch status, disk space and resource usage alerts. `PUT /api/v1/apps/{name}/node` and `move-with-volumes` are `root` too, because `move-with-volumes` changes placement and does a full-overwrite restore of every named volume.

## CLI

```bash
levelrail-cli nodes list [flags]
levelrail-cli nodes get <id> [flags]
levelrail-cli nodes delete <id> [flags]
levelrail-cli nodes join-token [flags]
levelrail-cli nodes ssh-provision --host ADDR --user NAME (--key-file PATH | --password) --name NAME [--host-key-fingerprint SHA256:... | --insecure-skip-host-key-check] [flags]
levelrail-cli nodes ssh-provisions list|show <id> [flags]
levelrail-cli nodes providers list|set-credential [flags]
levelrail-cli nodes provision --provider NAME --region ID --size ID --name NAME [flags]
levelrail-cli nodes provisions list|show <id> [flags]
levelrail-cli nodes cordon <id> [flags]
levelrail-cli nodes uncordon <id> [flags]
levelrail-cli nodes drain <id> [--target <node-id>] [--wait] [flags]
levelrail-cli nodes drains <id> [flags]
levelrail-cli nodes add [--name NAME] [--mesh] [flags]
levelrail-cli nodes move app|database <name> [--to <node-id>] [--wait] [flags]
levelrail-cli nodes labels <id> [key=value ...] [flags]
levelrail-cli nodes placement app|database <name> [key=value ...] [flags]
levelrail-cli nodes connectivity [flags]
levelrail-cli nodes workloads <id> --accepts-app=BOOL --accepts-build=BOOL [flags]
levelrail-cli nodes health <id> [flags]
levelrail-cli nodes patch-status <id> [flags]
levelrail-cli nodes metrics <id> --metric NAME [--since DURATION | --from TIME --to TIME] [--step DURATION] [flags]
levelrail-cli nodes events <id> [--limit N] [flags]
levelrail-cli nodes resource-usage [flags]
levelrail-cli nodes capacity-forecast <id> [flags]
levelrail-cli nodes topology [flags]
levelrail-cli nodes traffic [flags]
levelrail-cli nodes mesh [flags]
levelrail-cli nodes rotate-key <id> [flags]
levelrail-cli nodes rejoin-mesh <id> [flags]
levelrail-cli nodes reenroll-token <id> [flags]
levelrail-cli nodes revoke-cert <id> [flags]
levelrail-cli apps set-node <name> <node-id> [--with-volumes] [flags]
levelrail-cli apps clear-node <name> [--with-volumes] [flags]
```

<Note>
`nodes workloads` is a full replace of both flags, not a per-field patch. Both `--accepts-app` and `--accepts-build` are required on every call, so one flag can never silently reset to `false`. See [Build node routing](/build-node-routing) for what `accepts_build_workloads` does.
</Note>

## Not built yet

<AccordionGroup>
<Accordion title="Resource-aware scheduling">

The spread scheduler counts placements and filters on cordon, status and labels, never CPU, memory or disk headroom. Resource-aware scheduling is not built. Bin-packing and affinity rules are a deliberate non-goal.

</Accordion>
<Accordion title="Agent self-update">

Upgrading an agent is still manual: stop it, replace the binary or image, start it again. The control plane only flags outdated agents.

</Accordion>
<Accordion title="Node-specific change history">

Cordon, drain and workload changes appear in the platform audit log (`GET /api/v1/audit-log`). Only connection events (online, offline, cordoned) have a per-node history view.

</Accordion>
</AccordionGroup>

## Related pages

<CardGroup cols={2}>
<Card title="Node provisioning" href="/node-provisioning">
Create nodes at a cloud provider.
</Card>
<Card title="Build node routing" href="/build-node-routing">
Choose which node runs builds.
</Card>
<Card title="Network topology" href="/network-topology">
See every node with its apps, databases and connections.
</Card>
<Card title="Domains and ingress" href="/domains-and-ingress">
How the control plane's ingress routes to apps.
</Card>
<Card title="Backups and storage" href="/backups-and-storage">
Move app volumes to another node.
</Card>
<Card title="Observability" href="/observability">
Per-node metrics, resource usage and patch status.
</Card>
</CardGroup>
