---
title: "Desktop runtime (managed VM)"
icon: "/assets/icons/download-simple.svg"
sidebarTitle: "Desktop runtime"
description: Run Levelrail in a private Linux VM on your Mac. Experimental, Apple silicon first. Apps run isolated from your files, with no host Docker install.
"og:url": "https://docs.levelrail.com/desktop-runtime"
---

The managed VM is a private, lightweight Linux virtual machine on your Mac that runs Levelrail, so the apps you deploy run isolated from your files. Widget-style apps such as OpenClaw are the intended use, and the VM has no access to your home directory by default.

This is experimental and not yet proven outside one test Mac. Every number and claim on this page is unverified beyond the test runs described below.

## Requirements

- macOS on Apple silicon (arm64). Intel Macs are not tested.
- Lima installed, with `limactl` on your `PATH`. The pinned Lima download is not in this release yet, so `vm init` uses the copy it finds.
- Hardware virtualization and a macOS version that supports the `vz` backend.
- An 8 GB laptop is the smallest budget we plan for. The default VM memory is 3 GiB (3072 MiB), which leaves macOS about 5 GB.

Docker does not need to be installed on the Mac. The Docker engine runs inside the VM.

## Turn it on

The `vm` commands stay closed until you set the experimental flag.

```bash
export APP_VM_EXPERIMENTAL=on
```

Without it, every `vm` command stops with a message that names the flag.

## First run

Create the VM from the pinned guest image. Init boots it, installs Docker inside it, runs the isolation self-check and copies the control plane in. The control plane must be a Linux build for arm64. Pass its path with `--control-plane-bin`.

```bash
export APP_VM_EXPERIMENTAL=on
levelrail-cli vm init --control-plane-bin /path/to/levelrail-linux-arm64
```

Init creates these files and folders, all under `~/Library/Application Support/Levelrail/vm/`:

- `settings.json`: the VM settings (memory, CPUs, disk, ports).
- `admin-credentials`: the first admin user and password, mode `0600`.
- `levelrail.yaml`: the VM template, mode `0600`.
- `lima/`: a private Lima home. The VM instance lives in `lima/levelrail`.

The first admin password is printed once, in the init output, in the form `First admin: admin / <password>`. The same password is kept in `admin-credentials`, which only your user can read. The file is written once, so running init again does not print it again.

When the dashboard answers, `vm start` and `vm status` print its address:

```text
http://levelrail.localhost:<port>/
```

The port is the first free loopback port at init, unless you set one. The host name is `levelrail.localhost` on purpose. Desktop mode serves only that host name, so the session cookie stays host-only and does not leak to other apps on other localhost ports. Browsers resolve `*.localhost` to your own machine, so no DNS is needed.

## Daily use

```bash
levelrail-cli vm start
levelrail-cli vm status
levelrail-cli vm status --json
levelrail-cli vm shell
levelrail-cli vm stop
```

`vm shell` opens a shell inside the guest. The guest user has passwordless sudo, because the VM is the security boundary, not the user inside it.

## Resources and limits

Settings live in `settings.json`. Environment variables only seed that file at init. After init, change settings with `vm set`.

```bash
levelrail-cli vm set memory 3072
levelrail-cli vm set cpus 2
levelrail-cli vm set disk 40
levelrail-cli vm set dashboard-port 8090
levelrail-cli vm set ingress-port 8891
```

| Setting | Default | Limit | Env (seeds at init) |
| --- | --- | --- | --- |
| memory | 3072 MiB | at least 2048 MiB, at most 50% of host memory | `APP_VM_MEMORY_MIB`, `APP_VM_MEMORY_MIN_MIB`, `APP_VM_MEMORY_MAX_PERCENT` |
| cpus | 2 | 1 up to the host CPU count | `APP_VM_CPUS` |
| disk | 30 GiB | 10 to 1024 GiB, and it can only grow | `APP_VM_DISK_GIB` |
| dashboard-port | first free loopback port | a loopback port that is not in use | `APP_VM_DASHBOARD_PORT` |
| ingress-port | first free loopback port from 8880 | a loopback port that is not in use | `APP_VM_INGRESS_PORT` |

A change to memory, CPUs or ports takes effect on the next start. `vm set` says so when the VM is running. The boot timeout defaults to 180 seconds and is set with `APP_VM_BOOT_TIMEOUT`.

Disk grows only. Shrinking is refused, and a disk increase is applied on the next start. The disk grow was tested once, from 10 GiB to 12 GiB.

### Measured on the test Mac

These numbers come from one Apple silicon Mac with a 2048 MiB, 2 CPU, 10 GiB VM. The 3 GiB default has not been measured.

- Fresh `vm init` took about 154 to 158 seconds with the guest image cached. The first run without a cached image took about 193 seconds.
- A stop and start of an existing VM took about 20 to 24 seconds.
- Idle with Docker running, the guest had 1950 MiB total and about 1525 MiB available. Used memory was about 424 MiB. The control plane used about 264 MB.
- Guest disk use after the first boot was about 3.1 GB.

Real use with apps such as OpenClaw, and use on an 8 GB machine, is not measured.

## What the isolation guarantees

On every start, the guest runs a nine-check self-check, and `vm status` shows the result. The checks are `rules_loaded`, `publish_loopback`, `no_wildcard_publish`, `no_docker_tcp`, `app_pool_allowed`, `host_mounts`, `host_unreachable`, `lan_unreachable` and `dns_allowed`. A failing check stops the VM.

- **No host files.** Nothing from your Mac is mounted, and your home directory is not visible in the guest.
- **Shared folders are off.** `vm status` prints `Shares: none`. No share command exists in this release.
- **Only two ports leave the VM.** The dashboard and the app ingress are forwarded, both to loopback on your Mac. Other guest ports are not forwarded automatically.
- **No Docker socket on the host.** The host needs no Docker installation.
- **No host clipboard or sound device.**
- **The guest cannot reach your Mac.** Guest and container traffic to the Mac's loopback is blocked, including through the gateway address and from containers that use host networking. The guest also cannot reach private and LAN addresses. Internet access works, and DNS works through the gateway.
- **App networks stay reachable.** The control plane unit sets `APP_NETWORK_POOL=172.30.0.0/15`, inside the Docker range the firewall allows. A pool in a dropped range (for example the default `10.88.0.0/14`) would make every app time out, so `app_pool_allowed` fails the check if `/etc/levelrail/controlplane.env` overrides it outside that range. `vm upgrade` rewrites the unit, so existing VMs pick the pool up.
- **Direct app access.** Apps are reached through ingress at `<app>.localhost`, with the normal ingress shield and authentication.

### What it does not guarantee

- **Passwordless sudo in the guest** is accepted, because the VM is the boundary. Anything running as the guest user can become root there.
- **Host-network containers are blocked at the firewall, but not rejected at deploy time.** Rejecting `network_mode: host` in the deployer is still a follow-up.
- **The guest sshd listens on all addresses,** on port 22. It is protected only by the guest input firewall.
- **Shared folders are not available yet.** When they ship, they are planned as explicit, read-only by default and opt-in, one folder at a time.
- **Lima overrides are refused.** Init and start refuse to run if the private Lima home has `default.yaml`, `override.yaml` or `always.yaml`, because Lima merges them into every instance.
- **Not a host for other people.** The desktop runtime is for running apps for yourself. It is not a way to host public apps.
- **Linux and Windows have no VM and make no isolation claim.**

## Troubleshooting

### Isolation status is stale or unreadable

`vm status` shows `Isolation: no result`, `unreadable` or `fail`. The status file is read only for the current boot, so an old file never counts. Restart the VM first.

```bash
levelrail-cli vm stop
levelrail-cli vm start
levelrail-cli vm status
```

If the status still says `fail`, read the `check <name>` lines it prints and do not use the VM until they pass. A failed check on start stops the VM for you.

### The home path is too long for Lima

`limactl create` can fail with a socket error. The instance keeps a Unix socket (`ssh.sock`) under `~/Library/Application Support/Levelrail/vm/lima/levelrail`, and the path must stay under 104 characters. A long macOS user name is enough to break it.

No setting moves the Lima home yet. In testing, the workaround was a short `HOME` for every `vm` command. This is unverified for everyday use.

```bash
HOME=/private/tmp/lv levelrail-cli vm init
HOME=/private/tmp/lv levelrail-cli vm status
```

### A port is taken

At start, a taken port is replaced by a free loopback port, and the CLI prints the new numbers. The dashboard address changes with it. To pick a port yourself, set it and start again.

```bash
levelrail-cli vm set dashboard-port 8090
levelrail-cli vm stop
levelrail-cli vm start
```

Changing a port on a running VM was not tested.

### The disk fills up

`vm status` prints `Disk use`, which is the guest root filesystem. Grow the disk on a stopped VM, then start it.

```bash
levelrail-cli vm stop
levelrail-cli vm set disk 40
levelrail-cli vm start
```

### Other messages

- `not initialised; run levelrail vm init first`: run `vm init` with the experimental flag set.
- `the VM already exists`: use `vm set` to change settings.
- `the VM is not running`: run `vm start` before `vm shell`.

## Commands planned, not yet in the CLI

These are planned. They do not exist in the `vm` command yet: `vm export`, `vm reset`, `vm upgrade`, `vm uninstall` and `vm token`.

## Not yet proven

- Intel Macs.
- Windows, and WSL2, which is design only.
- Linux desktops, which have no VM and no isolation claim.
- Shared folders, which are not implemented.
- Upgrading an existing VM. The old guest unit stays until provisioning runs again, and only a fresh `vm init` was tested.
- Amd64 images, and Rosetta.
- Rejecting `network_mode: host` at deploy time.
- Laptop sleep and wake, and guest clock sync.
- Real memory use with apps, at the 3 GiB default and on an 8 GB machine.
- Changing ports on a running VM.
- Repeated disk growth. Growing from 10 GiB to 12 GiB was tested once.

For the regular local setup without a VM, see [Run it on your laptop](/local-development).
