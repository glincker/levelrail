---
title: "Run a node on macOS"
tag: "Beta"
sidebarTitle: "macOS node agent"
description: "Install the Levelrail node agent on a Mac with Homebrew or a downloaded binary, run it under launchd, and know what a Mac node can and cannot do."
"og:url": "https://docs.levelrail.com/macos-agent"
---

The node agent is published for macOS on Apple silicon and Intel. A Mac can join a Levelrail control plane as an extra node, which suits build jobs and light workloads on hardware you already own. The control plane itself still runs on Linux.

<Warning>
This feature is beta. The agent binary is cross-compiled for macOS and the install pieces are checked statically. A full enrol and deploy on a real Mac through a published release has not been verified yet. See [Limits](#limits).
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

<Steps>
<Step title="Enrol once in the foreground">

Copy the address, token and CA fingerprint from the join command in the dashboard, then run:

```bash
APP_CONTROL_PLANE_ADDR=<host:port> APP_JOIN_TOKEN=<token> \
APP_CA_FINGERPRINT=<fingerprint> \
APP_AGENT_IDENTITY_FILE="$(brew --prefix)/var/levelrail-agent/identity.json" \
levelrail-agent
```

Stop it with Ctrl-C when the node shows as connected. The identity is saved, so the token is not needed again.

</Step>
<Step title="Write the service settings">

Create `$(brew --prefix)/etc/levelrail-agent.env` with one `KEY=value` per line:

```bash
APP_CONTROL_PLANE_ADDR=<host:port>
APP_CA_FINGERPRINT=<fingerprint>
```

If your runtime socket is not at `/var/run/docker.sock`, add a `DOCKER_HOST=unix:///path/to/docker.sock` line. Do not store the join token here.

</Step>
<Step title="Start it at login">

```bash
brew services start levelrail-agent
```

Logs go to `$(brew --prefix)/var/log/levelrail-agent.log`.

</Step>
</Steps>

## Install from the tarball or binary

Download `levelrail-agent-darwin-arm64` or `levelrail-agent-darwin-amd64` from the release page, with `checksums.txt`, and verify it:

```bash
shasum -a 256 -c --ignore-missing checksums.txt
```

The binary is not signed with an Apple developer certificate or notarized. Gatekeeper blocks a downloaded binary until you remove the quarantine flag:

```bash
chmod +x levelrail-agent-darwin-arm64
xattr -d com.apple.quarantine levelrail-agent-darwin-arm64
sudo install -m 0755 levelrail-agent-darwin-arm64 /usr/local/bin/levelrail-agent
```

Enrol once in the foreground as shown above, using `APP_AGENT_IDENTITY_FILE=$HOME/Library/Application Support/levelrail-agent/identity.json`. Without that variable the agent writes `levelrail-agent-identity.json` in the current directory.

## Run under launchd without Homebrew

The repository ships a template at `deploy/packaging/launchd/com.levelrail.agent.plist`. Copy it, replace the `@PLACEHOLDERS@` (binary path, control plane address, CA fingerprint, state and log directories), then check it:

```bash
plutil -lint ~/Library/LaunchAgents/com.levelrail.agent.plist
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.levelrail.agent.plist
```

The state directory must contain the identity file from the foreground enrol. A LaunchAgent runs while you are logged in.

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
- Features that rely on Linux kernel facilities are not available on macOS: cgroup resource limits, host firewall rules, the WireGuard mesh, GPU access and some host diagnostics.
- The macOS build is cross-compiled and guarded in CI, but running a node on a real Mac with a published release, `brew install`, `brew services` and the launchd template is not verified.
- Gatekeeper behaviour on a downloaded binary follows the quarantine steps above and is not verified on every macOS version.
