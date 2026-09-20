---
description: Get the Levelrail control plane running on Linux with install.sh, Docker, or source build.
---

# Installing Levelrail

Levelrail ships as two static Go binaries (`levelrail`, the control
plane, and `levelrail-agent`, the node agent) plus a CLI
(`levelrail-cli`). This page covers every supported way to get the
control plane running on a real Linux host, how to verify it worked,
and how to upgrade or remove it afterward.

## Option 1: install.sh (recommended)

```
curl -fsSL https://raw.githubusercontent.com/glincker/levelrail/main/install.sh | sudo sh
```

This is the same script linked from the root [README](../README.md). It handles the full setup automatically:

- Downloads the right `linux/amd64` or `linux/arm64` binary from the [latest GitHub release](https://github.com/glincker/levelrail/releases)
- Verifies the checksum
- Installs Docker via `get.docker.com` if missing
- Writes a `levelrail.service` systemd unit
- Starts the service
- Waits up to 60 seconds for the control plane to answer `GET /api/v1/brand`

Requires `curl`, `systemd`, and root access.

::: details Optional environment variables

The `install.sh` script reads these environment variables, all optional:

| Variable | Default | What it does |
| --- | --- | --- |
| `LEVELRAIL_VERSION` | latest release | Pin a specific release tag instead of resolving the newest one |
| `LEVELRAIL_INSTALL_DIR` | `/usr/local/bin` | Where the `levelrail` binary is installed |
| `LEVELRAIL_DATA_DIR` | `/var/lib/levelrail-data` | Control plane data directory (SQLite database, generated `brand.yaml`) |
| `LEVELRAIL_CONFIGURE_UFW` | unset (off) | Set to `1` to have the script configure `ufw`: allow SSH, then 80/443, then enable it if it wasn't already active. Off by default, the script never touches your firewall otherwise. |

:::

Common scenarios:

::: code-group
```bash [Pin a specific release]
curl -fsSL https://raw.githubusercontent.com/glincker/levelrail/main/install.sh \
  | sudo LEVELRAIL_VERSION=v0.1.0 sh
```

```bash [Custom install directory]
curl -fsSL https://raw.githubusercontent.com/glincker/levelrail/main/install.sh \
  | sudo LEVELRAIL_INSTALL_DIR=/opt/levelrail/bin sh
```

```bash [Custom data directory]
curl -fsSL https://raw.githubusercontent.com/glincker/levelrail/main/install.sh \
  | sudo LEVELRAIL_DATA_DIR=/data/levelrail sh
```

```bash [Configure UFW automatically]
curl -fsSL https://raw.githubusercontent.com/glincker/levelrail/main/install.sh \
  | sudo LEVELRAIL_CONFIGURE_UFW=1 sh
```

```bash [Multiple overrides]
sudo LEVELRAIL_VERSION=v0.1.0 LEVELRAIL_DATA_DIR=/data/levelrail sh
```
:::


## Option 2: Docker

Already running everything else as containers? See [docs/docker.md](docker.md) for `docker run` and `docker-compose.yml` examples.

Images available:
- `ghcr.io/glincker/levelrail` (control plane)
- `ghcr.io/glincker/levelrail-agent` (node agent)

Both are published for `linux/amd64` and `linux/arm64` on every tagged release.

::: tip
`install.sh` remains the recommended path for a real single-node deployment since it also provisions Docker and a systemd unit for you. The Docker image is for operators who want Levelrail to fit into an existing container-only setup.
:::

## Option 3: build from source

See [docs/getting-started.md](getting-started.md#build-and-run) for
the `go build` commands. This is the path for contributors and anyone
who wants to run an unreleased commit rather than a tagged version.

## Verifying the install

`install.sh` already fails loudly if the control plane doesn't come up healthy within 60 seconds. You can re-check the installation at any time:

**Quick checks (unauthenticated):**

```bash
# Health check the control plane responds
curl -fsS http://127.0.0.1:8080/api/v1/brand

# Check systemd service status
systemctl status levelrail

# View logs in real time
journalctl -u levelrail -f
```

**Authenticated checks (after initial setup):**

Once you've created an admin account and minted an API token (see [docs/getting-started.md](getting-started.md#deploy-your-first-app)), you can use `levelrail-cli` for deeper validation:

```bash
levelrail-cli version   # compare running vs. latest published release
levelrail-cli status    # check Docker reachability and system config
```

Both commands talk to the control plane's API (`GET /api/v1/updates` and `GET /api/v1/system/status`), confirming it is reachable and authenticating correctly.

## Upgrading

**If you used install.sh:**

Re-run the same one-liner. It's safe to re-run: it overwrites the binary and unit file, then restarts the service.

```bash
curl -fsSL https://raw.githubusercontent.com/glincker/levelrail/main/install.sh | sudo sh
```

To pin a specific release instead of the latest:

```bash
curl -fsSL https://raw.githubusercontent.com/glincker/levelrail/main/install.sh \
  | sudo LEVELRAIL_VERSION=v0.1.0 sh
```

**If you used Docker:**

Pull the new tag and recreate the container:

```bash
docker pull ghcr.io/glincker/levelrail:latest
docker compose up -d
```

Or if using `docker run`:

```bash
docker rm -f levelrail
# Re-run the docker run command from docs/docker.md
```

The named volume holding `/var/lib/levelrail-data` persists across recreation.

## Uninstalling

There is no uninstall script today.

**If you used install.sh:**

```bash
sudo systemctl stop levelrail
sudo systemctl disable levelrail
sudo rm /etc/systemd/system/levelrail.service
sudo systemctl daemon-reload

sudo rm "$(command -v levelrail)"        # or your LEVELRAIL_INSTALL_DIR path
sudo rm -rf /var/lib/levelrail-data      # or your LEVELRAIL_DATA_DIR path
```

::: warning
Removing `/var/lib/levelrail-data` deletes all app and deploy state.
:::

This does not touch Docker itself or any containers, images, or volumes Levelrail created for your deployed apps. Remove those separately if needed.

**If you used Docker:**

```bash
docker rm -f levelrail levelrail-agent
docker volume rm levelrail-data
```

## See also

- [Docker installation](docker.md) - Running Levelrail as containers
- [Getting started](getting-started.md) - Deploy your first app after installation
- [Upgrading](installing.md#upgrading) - How to keep Levelrail up to date
