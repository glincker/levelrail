---
title: "Run a node on macOS"
tag: "Beta"
sidebarTitle: "macOS node agent"
description: "Install the Levelrail node agent on a Mac with Homebrew or a downloaded binary, run it under launchd, and know what a Mac node can and cannot do."
"og:url": "https://docs.levelrail.com/macos-agent"
---

The node agent is published for macOS on Apple silicon and Intel. A Mac can join a Levelrail control plane as an extra node, which suits build jobs and light workloads on hardware you already own. The control plane itself still runs on Linux.

<Warning>
This feature is beta. It was verified locally on an Apple silicon Mac against a Linux control plane in a Lima VM: enrol, a Homebrew install from a local tap, `brew services` start, restart and stop, and a container deploy through the Lima Docker socket. It has not been run from a published release, with Docker Desktop or OrbStack, or on Intel.
</Warning>

## Requirements

- macOS on Apple silicon (arm64) or Intel (amd64).
- A container runtime with a Docker compatible socket: Docker Desktop, OrbStack or Lima. Start it before the agent.
- A control plane that this Mac can reach on its agent port, and a join command from **Nodes**, then **Add node**.

## Install with Homebrew

```bash
brew install glincker/tap/levelrail-agent
```

The formula downloads the binary for your Mac, checks its sha256 and installs `levelrail-agent`. It is not notarized by Apple.

Newer Homebrew versions may ask you to trust a third party tap first (`brew trust glincker/tap`). Not verified for this tap.

<Steps>
<Step title="Enrol once in the foreground">

Copy the address, token and CA fingerprint from the join command in the dashboard (the join token expires after 15 minutes), then run:

```bash
APP_CONTROL_PLANE_ADDR=<host:port> APP_JOIN_TOKEN=<token> \
APP_CA_FINGERPRINT=<fingerprint> \
APP_AGENT_IDENTITY_FILE="$(brew --prefix)/var/levelrail-agent/identity.json" \
levelrail-agent
```

Stop it with Ctrl-C when the log prints `enrolled node_id=...` and the node shows as online in the dashboard. The identity is saved, so the token is not needed again.

Do this before you start the service. Without a saved identity and a join token the service exits with status 78 and launchd respawns it about every 10 seconds (every 30 seconds with the launchd template below).

</Step>
<Step title="Write the service settings">

Create `$(brew --prefix)/etc/levelrail-agent.env` with one `KEY=value` per line:

```bash
APP_CONTROL_PLANE_ADDR=<host:port>
APP_CA_FINGERPRINT=<fingerprint>
```

The agent finds the runtime socket on its own (see [Container socket](#container-socket)). Add a `DOCKER_HOST=unix:///path/to/docker.sock` line only to force a different one. Do not store the join token here.

</Step>
<Step title="Start it at login">

```bash
brew services start levelrail-agent
```

Logs go to `$(brew --prefix)/var/log/levelrail-agent.log`. The launchd job is named `sh.brew.levelrail-agent`.

</Step>
</Steps>

## Install from the tarball or binary

Download `levelrail-agent-darwin-arm64` or `levelrail-agent-darwin-amd64` from the release page, with `checksums.txt`, and verify it:

```bash
shasum -a 256 -c --ignore-missing checksums.txt
```

The binary is not signed with an Apple developer certificate or notarized. A downloaded file carries the quarantine attribute, and macOS can hang silently on the first run instead of showing a dialog. Remove the attribute before the first run (the command errors harmlessly if the attribute is absent):

```bash
chmod +x levelrail-agent-darwin-arm64
xattr -d com.apple.quarantine levelrail-agent-darwin-arm64
sudo install -m 0755 levelrail-agent-darwin-arm64 /usr/local/bin/levelrail-agent
```

Enrol once in the foreground as shown above, using `APP_AGENT_IDENTITY_FILE=$HOME/Library/Application Support/levelrail-agent/identity.json`. Without that variable the agent writes `levelrail-agent-identity.json` in the current directory.

## Run under launchd without Homebrew

The repository ships a template at `deploy/packaging/launchd/com.levelrail.agent.plist`. Copy it, replace the `@PLACEHOLDERS@` (binary path, control plane address, CA fingerprint, state and log directories), then check it. The state and log directories must exist before the job starts, otherwise it exits with status 78 and writes no log:

```bash
mkdir -p "$HOME/Library/Application Support/levelrail-agent" "$HOME/Library/Logs/levelrail-agent"
plutil -lint ~/Library/LaunchAgents/com.levelrail.agent.plist
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.levelrail.agent.plist
```

The state directory must contain the identity file from the foreground enrol. The template sets `ThrottleInterval` to 30 seconds (a placeholder value, tune it) so an unenrolled start does not respawn every 10 seconds. A LaunchAgent runs while you are logged in.

## Container socket

On macOS the agent picks the first of these that applies:

1. `APP_CONTAINER_RUNTIME_SOCKET`, then `DOCKER_HOST`.
2. The first socket that exists and answers, in this order: `~/.docker/run/docker.sock` (Docker Desktop), `~/.colima/default/docker.sock`, `~/.orbstack/run/docker.sock` (OrbStack), `~/.rd/docker.sock` (Rancher Desktop), `~/.lima/<instance>/sock/docker.sock` (Lima, instances in name order), then `/var/run/docker.sock`.

Docker Desktop's real default is `~/.docker/run/docker.sock`. `/var/run/docker.sock` exists only as a symlink when you enabled that option in Docker Desktop. Discovery skips sockets that do not answer, so a stale Docker Desktop socket left after quitting no longer hides a live Lima or OrbStack one. If no candidate answers, the agent uses the first one that exists, or falls back to `/var/run/docker.sock`.

The same socket is used for builds. On Linux the agent keeps using `DOCKER_HOST` or `/var/run/docker.sock` as before. The order is covered by unit tests with temporary directories; it has not been run against a real Docker Desktop, OrbStack or Lima.

## Disk space

The control plane refuses a deploy to a node whose free disk is under the critical threshold (7 percent by default) with `this node is out of room (disk space is down to X percent free)`. A nearly full Mac volume reaches that easily. Free space on the Mac, or lower the thresholds on the control plane with `APP_NODE_DISK_CRIT_FREE_PCT` and `APP_NODE_DISK_WARN_FREE_PCT` (defaults 7 and 15 percent). These are control plane environment variables, not runtime settings.

## Debug logs

Set `APP_LOG_LEVEL=debug` in the agent's environment (for example in the launchd plist or the service environment) and restart it. At debug the agent also logs each readiness probe attempt with its kind, target, outcome and elapsed milliseconds.

## Stop and uninstall

```bash
brew services stop levelrail-agent
brew uninstall levelrail-agent
# without Homebrew:
launchctl bootout gui/$(id -u)/com.levelrail.agent
```

Then remove the node in the dashboard, and delete the state directory (`$(brew --prefix)/var/levelrail-agent` or the one you chose) to drop the saved identity. Containers the node started stay in your runtime until you remove them there.

## Limits

- Workloads run through Docker Desktop, OrbStack or Lima only. There is no native macOS container runtime.
- The node suits builds best. A laptop that sleeps or changes network is a poor home for always-on apps.
- Features that rely on Linux kernel facilities are not available on macOS: cgroup resource limits, host firewall rules, the kernel WireGuard module (the mesh is off unless `APP_MESH_ENABLED=1`, then uses the userspace implementation and needs root), NVIDIA GPU access and `/proc` host stats. The agent logs `not supported on macOS: <feature>` once at start and keeps running. Memory, swap and process size fall back to macOS or Go runtime values.
- The agent reports `darwin` and the CPU architecture to the control plane. The control plane has no scheduling rule that keeps production workloads off a Mac node by default, so place workloads deliberately. Not verified end to end.
- Docker runs in a VM on macOS, so host networking and ports published on `127.0.0.1` belong to the VM or the runtime's forwarder, not to the Mac. Not verified for every runtime.
- Without the mesh, an app on a Mac node is not reachable through the control plane's ingress: the app status shows `CrossNodeIngress False NoMeshIngressPath`. Only the port published on the Mac's loopback address works.
- Stopping the service marks the node offline at once. Deleting an app while the agent is stopped leaves its container until the agent reconnects, then it is removed.
- Verified locally: the Lima path with a Homebrew install from a local tap, `brew services` and a foreground enrol, on Apple silicon. Not verified: a published release download, Docker Desktop, OrbStack, Intel Macs, and Gatekeeper on every macOS version.
- Gatekeeper behaviour on a downloaded binary follows the quarantine steps above and is not verified on every macOS version.
