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

This is the same script linked from the root [README](../README.md).
It downloads the right `linux/amd64` or `linux/arm64` binary from the
[latest GitHub release](https://github.com/glincker/levelrail/releases),
verifies its checksum, installs Docker via `get.docker.com` if it's
missing, writes a `levelrail.service` systemd unit, starts it, and
waits (up to 60 seconds) for the control plane to answer
`GET /api/v1/brand` before declaring success. Requires `curl`,
`systemd`, and root.

`install.sh` reads these environment variables, all optional:

| Variable | Default | What it does |
| --- | --- | --- |
| `LEVELRAIL_VERSION` | latest release | Pin a specific release tag instead of resolving the newest one |
| `LEVELRAIL_INSTALL_DIR` | `/usr/local/bin` | Where the `levelrail` binary is installed |
| `LEVELRAIL_DATA_DIR` | `/var/lib/levelrail-data` | Control plane data directory (SQLite database, generated `brand.yaml`) |
| `LEVELRAIL_CONFIGURE_UFW` | unset (off) | Set to `1` to have the script configure `ufw`: allow SSH, then 80/443, then enable it if it wasn't already active. Off by default, the script never touches your firewall otherwise. |

Examples:

```
# Pin a specific release
curl -fsSL https://raw.githubusercontent.com/glincker/levelrail/main/install.sh \
  | sudo LEVELRAIL_VERSION=v0.1.0 sh

# Install the binary somewhere other than /usr/local/bin
curl -fsSL https://raw.githubusercontent.com/glincker/levelrail/main/install.sh \
  | sudo LEVELRAIL_INSTALL_DIR=/opt/levelrail/bin sh

# Use a custom data directory
curl -fsSL https://raw.githubusercontent.com/glincker/levelrail/main/install.sh \
  | sudo LEVELRAIL_DATA_DIR=/data/levelrail sh

# Let the script open 80/443 (and SSH, if not already allowed) via ufw
curl -fsSL https://raw.githubusercontent.com/glincker/levelrail/main/install.sh \
  | sudo LEVELRAIL_CONFIGURE_UFW=1 sh
```

Multiple overrides combine normally, e.g.
`sudo LEVELRAIL_VERSION=v0.1.0 LEVELRAIL_DATA_DIR=/data/levelrail sh`.

## Option 2: Docker

Already running everything else as containers? See
[docs/docker.md](docker.md) for `docker run` and `docker-compose.yml`
examples for both `ghcr.io/glincker/levelrail` (control plane) and
`ghcr.io/glincker/levelrail-agent` (node agent), published for
`linux/amd64` and `linux/arm64` on every tagged release. `install.sh`
remains the recommended path for a real single-node deployment since
it also provisions Docker and a systemd unit for you; the Docker image
is for operators who want Levelrail to fit into an existing
container-only setup instead.

## Option 3: build from source

See [docs/getting-started.md](getting-started.md#build-and-run) for
the `go build` commands. This is the path for contributors and anyone
who wants to run an unreleased commit rather than a tagged version.

## Verifying the install

`install.sh` already fails loudly if the control plane doesn't come up
healthy within 60 seconds, but you can re-check any time:

```
# same check install.sh itself polls, unauthenticated
curl -fsS http://127.0.0.1:8080/api/v1/brand

# systemd's view of the service
systemctl status levelrail

# tail the control plane's own logs
journalctl -u levelrail -f
```

Once you've created an admin account and minted an API token (see
[docs/getting-started.md](getting-started.md#deploy-your-first-app)),
`levelrail-cli` gives you two more direct checks:

```
levelrail-cli version   # running version vs. the latest published release
levelrail-cli status    # local Docker reachability, secrets/telemetry/alerts config
```

Both talk to the control plane's own API (`GET /api/v1/updates` and
`GET /api/v1/system/status`), not the local binary, so they also
confirm the control plane is reachable and authenticating correctly.

## Upgrading

**install.sh:** re-run the same one-liner. Per its own header comment,
it's safe to re-run: it overwrites the binary and unit file, then
restarts the service, so re-running it is how you upgrade. Set
`LEVELRAIL_VERSION` to move to a specific release instead of whatever
is newest.

**Docker:** pull the new tag and recreate the container, e.g.
`docker pull ghcr.io/glincker/levelrail:latest` followed by
`docker compose up -d` (or `docker rm -f levelrail` and re-run the
`docker run` command) from [docs/docker.md](docker.md). The named
volume holding `/var/lib/levelrail-data` persists across the
recreate.

## Uninstalling

There is no uninstall script today. To remove an `install.sh`-based
install by hand:

```
sudo systemctl stop levelrail
sudo systemctl disable levelrail
sudo rm /etc/systemd/system/levelrail.service
sudo systemctl daemon-reload

sudo rm "$(command -v levelrail)"   # or your LEVELRAIL_INSTALL_DIR path
sudo rm -rf /var/lib/levelrail-data # or your LEVELRAIL_DATA_DIR path, deletes all app/deploy state
```

This does not touch Docker itself, or any containers, images, or
volumes Levelrail created for your deployed apps: remove those
separately if you also want them gone.

For a Docker-based install, remove the containers and volume instead:

```
docker rm -f levelrail levelrail-agent
docker volume rm levelrail-data
```
