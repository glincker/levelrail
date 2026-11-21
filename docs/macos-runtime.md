---
title: "macOS runtime"
tag: "Beta"
sidebarTitle: "macOS runtime"
description: "Check a Mac, run Levelrail in a managed Lima VM, keep it running with launchd, and know what is and is not supported."
"og:url": "https://docs.levelrail.com/macos-runtime"
---

A Mac runs Levelrail through a private Linux VM managed by Lima. This page covers readiness checks, VM controls and launchd. For joining a Mac as a node agent see [macOS node agent](/macos-agent).

<Warning>
Experimental. Set `APP_VM_EXPERIMENTAL=on` to use the `vm` commands. The checks and launchd commands below were unit tested with fakes. They have not been run against a real VM from a published release.
</Warning>

## Quick start

```bash
brew install lima
levelrail doctor macos
APP_VM_EXPERIMENTAL=on levelrail vm init
APP_VM_EXPERIMENTAL=on levelrail vm status
```

`doctor macos` works offline and exits 1 when a check fails. It reports:

| Check | Fails when |
| --- | --- |
| CPU architecture | not arm64 or amd64 |
| macOS version | older than `APP_MACOS_MIN_VERSION` (default 13) |
| Lima | `limactl` is not on PATH |
| Virtualization support | the kernel reports no hypervisor |
| Disk space for the VM | warns under `APP_MACOS_MIN_FREE_GIB` (default 40) |
| Loopback ports | warns when the VM dashboard or ingress port is taken |

## VM controls

CPU, memory and disk are settings stored in a host file. The environment only seeds them at `vm init`.

```bash
levelrail vm set memory 4096
levelrail vm set cpus 4
levelrail vm set disk 60
levelrail vm status      # shows values waiting for a restart
levelrail vm logs        # control plane journal from the guest
```

Memory is capped to a share of host memory (`APP_VM_MEMORY_MAX_PERCENT`, default 50). A changed value applies at the next `vm stop` and `vm start`.

## Keep it running with launchd

```bash
levelrail launchd print agent --binary /usr/local/bin/levelrail-agent
levelrail launchd install agent --binary /usr/local/bin/levelrail-agent \
  --env APP_CONTROL_PLANE_ADDR=<host:port>
levelrail launchd status
levelrail launchd uninstall agent
```

Units are per-user, in `~/Library/LaunchAgents`, labelled `com.levelrail.agent` or `com.levelrail.control-plane`. Logs go to `~/Library/Logs/Levelrail`. The respawn gap is `APP_LAUNCHD_THROTTLE_SECONDS` (default 30). Enrol the agent once in the foreground first. Tokens and secrets are refused in `--env`; keep them in the service env file.

## Install

`install-cli.sh` picks the darwin binary for arm64 or amd64 and verifies it against `checksums.txt`, and against the release signature when `cosign` is present.

## Supported and not supported

- Supported: Apple silicon and Intel, macOS 13 or newer, Lima with the vz driver, a node agent under launchd.
- Not supported: running the control plane natively on macOS, Docker Desktop managed by Levelrail, host folder mounts into the VM, Windows or Linux hosts for these commands.
- Binaries are not notarized by Apple.
