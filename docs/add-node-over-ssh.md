---
title: "Add a server over SSH"
tag: "Beta"
sidebarTitle: "Add a server over SSH"
description: "Adopt a Linux machine you already have by letting the control plane SSH in, install Docker if needed and install the node agent. Commands, host key checks and fixes for common failures."
"og:url": "https://docs.levelrail.com/add-node-over-ssh"
---

Use this when you have a server you can SSH into and you do not want to copy a join command onto it yourself. The control plane connects to the machine, installs the node agent as a systemd service and waits for it to enroll. After that the machine is a normal node. To copy the command by hand instead, see [Add a node anywhere](/add-node-anywhere). For the full multi-node picture see [Multi-node](/multi-node).

<Warning>
Not yet verified on a real host. The flow is covered by tests with a fake SSH server, not by a run against a real VPS or home server.
</Warning>

## What you need

- A Linux server with systemd, on amd64 (x86_64) or arm64 (aarch64). Any other OS or CPU fails before anything is changed.
- SSH access as `root`, or as a user that can run every command as root without a prompt. The installer writes under `/etc` and `/var/lib` and runs `systemctl`.
- The server must reach the control plane's agent port (9443 by default) outbound, and pull the agent image from the registry. The control plane must reach the server's SSH port.
- Your control plane address as the server sees it. The CLI defaults to the host of `--api-url` with port 9443. Pass `--control-plane-addr` if that is not reachable from the server.

## Add the server

<Tabs>
<Tab title="Dashboard">

Open **Nodes**, then **Add node**, then **Connect over SSH**. Enter the host and port. The **Verify server** step reads the server's host keys without sending any credential. Compare a fingerprint with the server, confirm it, then enter the username and a key or password. The wizard shows each stage and a live log tail.

</Tab>
<Tab title="CLI">

```bash
levelrail-cli nodes ssh-provision \
  --host 192.0.2.10 --user root --key-file ~/.ssh/id_ed25519 \
  --name home-server --control-plane-addr controlplane.example.com:9443
```

Without a pin flag the command prints the server's host key fingerprints and asks you to confirm before it reads any credential. In a script, pin the fingerprint:

```bash
levelrail-cli nodes ssh-provision --host 192.0.2.10 --user root \
  --key-file ~/.ssh/id_ed25519 --name home-server \
  --host-key-fingerprint SHA256:REPLACE_WITH_FINGERPRINT
```

| Flag | Meaning |
| --- | --- |
| `--host`, `--user`, `--name` | Required. The name is lowercase letters, digits and hyphens. |
| `--port` | SSH port. Default 22. |
| `--key-file` | Private key for key auth. Add `--passphrase` when the key is encrypted. |
| `--password` | Password auth. The value is prompted without echo and never taken as a flag value. |
| `--role` | `general` (default) or `build`. |
| `--control-plane-addr` | `host:port` the server dials. Default: the `--api-url` host on port 9443. |
| `--host-key-fingerprint` | Pin the host key (`SHA256:...`) and skip the prompt. |
| `--insecure-skip-host-key-check` | Accept any host key. Unsafe, warned and audited. |

The command returns a provision id right away. The install runs on the control plane in the background. Follow it:

```bash
levelrail-cli nodes ssh-provisions list
levelrail-cli nodes ssh-provisions show PROVISION_ID
```

</Tab>
<Tab title="API">

`POST /api/v1/nodes/ssh-provision` takes the host, port, username, a private key or a password, and the host key fingerprint. Poll `GET /api/v1/ssh-node-provisions/{id}` for the status, the install log and the detected OS and architecture. Both calls need the `root` ability. Status moves through `connecting`, `detecting`, `installing`, `enrolling` and ends at `ready` or `failed` with a reason.

</Tab>
</Tabs>

## Host key handling

Credentials only go to a server whose host key you have seen. `POST /api/v1/nodes/ssh-provision/preflight` with `{"host": "...", "port": 22}` reads the host keys and aborts the handshake right after key exchange, so no username, password or key is sent. It returns each key type and its SHA256 fingerprint. Compare one with the server:

```bash
ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub
```

The install then refuses to authenticate to a server that presents any other key. The failure names the expected and the observed fingerprint. A request with neither a fingerprint nor `insecure_skip_host_key_check` is rejected with `400`. Skipping the check is recorded in the audit log as `node_ssh_provision.host_key_skipped`.

## What it installs

1. Connects and detects the OS, CPU architecture, Docker and systemd. This step changes nothing on the server.
2. Installs Docker with `curl -fsSL https://get.docker.com | sh` and `systemctl enable --now docker` when Docker is missing.
3. Creates `/var/lib/levelrail-agent-data` (root only), writes `/etc/levelrail-agent.env` with mode `0600` (it holds the join token) and writes `/etc/systemd/system/levelrail-agent.service`. The unit runs the agent image as a container.
4. Runs `systemctl daemon-reload && systemctl enable --now levelrail-agent`, waits two seconds and checks the service is `active`. If it is not, the last lines of `journalctl -u levelrail-agent` become the failure reason.

The SSH credential stays in memory for this one run and is never stored or logged. The join token is valid for 30 minutes. Afterwards the agent dials out to the control plane over mTLS, and the node shows up with its [preflight checks](/add-node-anywhere#the-flow).

## When it fails

| Message | Fix |
| --- | --- |
| `a host key fingerprint is required` | Pass `--host-key-fingerprint`, or confirm the prompt in a terminal. |
| Expected and observed fingerprints differ | The server presents a different key than the one you pinned. Re-read the fingerprint on the server. If the machine was reinstalled, the key changed legitimately. |
| `unsupported OS` or `unsupported CPU architecture` | Only Linux on amd64 or arm64 is supported. |
| `this host has no systemd` | Use a systemd distribution. There is no workaround. |
| `install Docker` fails | Check outbound access to `get.docker.com`, or install Docker yourself and run the command again. An existing Docker is detected and left alone. |
| `the agent service did not stay active after starting` | Read the journal lines in the reason. Common causes are a blocked outbound port 9443 or a `--control-plane-addr` the server cannot resolve. |
| The provision stays at `enrolling` | The agent started but cannot reach the control plane. Open outbound TCP to the agent port and check the address. |
| A connection timeout in the first 15 seconds | The control plane cannot reach the SSH port. Check the firewall and the port. |

To undo an adoption, remove the node from **Nodes**, then on the server run `systemctl disable --now levelrail-agent` and delete the three paths above.

## Not yet verified on a real host

- No run of this flow against a real VPS, home server or Raspberry Pi is recorded yet.
- Servers where the SSH user is not root and needs `sudo` are not covered. Use `root`.
- Docker installed from a distribution package instead of `get.docker.com` is detected, but the combinations have not been tested.
