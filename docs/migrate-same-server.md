---
title: Migrate on the same server
description: "Move from Coolify, Dokploy or CapRover to Levelrail on the same machine: install in coexist mode, import, verify, then take over ports 80 and 443."
"og:url": "https://docs.levelrail.com/migrate-same-server"
---

This is the whole story for one server that runs your old platform today and will run Levelrail tomorrow. Levelrail is installed next to the old proxy first, your apps are staged and verified without touching live traffic, and only then does it take over ports 80 and 443. The takeover is one guided action with a recorded way back.

<Note>
This page is about a single server. To pull apps onto a different server, use [Migrating from Coolify, Dokploy or CapRover](/migrating-from-coolify-dokploy-and-caprover) and its cutover section instead.
</Note>

## The path in five stages

| Stage | What happens | Live traffic affected |
| --- | --- | --- |
| 1. Install in coexist mode | Levelrail starts on ports 8088 and 8443, dashboard on loopback | No |
| 2. Import and stage | Apps are recreated here, stopped until routed, old containers keep running | No |
| 3. Verify | Each staged app is built, started and probed through Levelrail's own ingress | No |
| 4. Take over ports | The old proxy stops, Levelrail serves 80 and 443, every domain is verified | Yes, a short pause |
| 5. Keep or roll back | The old proxy container stays stopped, never removed, so one action undoes it | Only on rollback |

## 1. Install in coexist mode

On a server where Coolify's Traefik (or another proxy container) holds ports 80 and 443, the installer notices and switches to coexist mode by itself. See [Server already runs a proxy](/installing#server-already-runs-a-proxy) for the full list of what changes.

```bash
curl -fsSL https://levelrail.com/install.sh | sudo sh -s -- --coexist --domain apps.example.com
```

In coexist mode the ingress listens on `8088` and `8443`, and the managed routes live in the old proxy's dynamic configuration (see [Run behind an existing proxy](/behind-an-existing-proxy)). The old platform keeps serving everything.

## 2. Import and stage

Open **Migrate** in the dashboard and follow the wizard: connect to the old platform with a read-only token, pick what to import, preflight, stage, move images, build and verify, copy volumes. Every step is described in [Migrating from Coolify, Dokploy or CapRover](/migrating-from-coolify-dokploy-and-caprover). Nothing on the old platform is written to.

## 3. Verify the staged apps

Enable routing for each app, then check each domain through the old proxy:

```bash
levelrail-cli proxy verify
```

### Two copies of one domain

An app imported from Coolify keeps its Traefik labels on the old container, and those labels register a router for the same hostname Levelrail now routes. Traefik can pick the old router, so a domain looks fine while it still reaches the old container. The proxy page flags this as "the proxy answers, but not through this server's ingress". It matters only while the old proxy runs: after the takeover the old proxy is stopped and only Levelrail answers. The preflight lists every domain that is claimed twice so you know which old containers to stop on the old platform after you are happy.

## 4. Take over this server's ports

Take over ports is the last step of the Migrate wizard and a tab on the Migrate page. It is root only.

<Steps>
<Step title="Read the preflight">
The checklist shows who holds ports 80 and 443 and what routes it still serves, whether every app is healthy here, whether DNS for each domain points at this server, whether certificates are uploaded or can be issued, whether automatic certificates are on, whether disk and memory have headroom, and whether the control plane can restart itself. Every blocker has a typed fix: a button, a copy command or a link, plus **Re-check**.
</Step>
<Step title="Read the plan">
The plan lists the exact steps and what rollback restores. Nothing changes until you confirm.
</Step>
<Step title="Run it">
Click **Take over ports**, type `takeover` and confirm. The run records the way back, stops the old proxy container (stopped, never removed), lets this server terminate TLS, stores the new listen addresses, restarts the control plane once and verifies every domain over real HTTPS. The page shows each step live and reconnects by itself after the restart.
</Step>
</Steps>

### From the CLI

```bash
levelrail-cli import takeover plan
levelrail-cli import takeover run --dry-run
levelrail-cli import takeover run --confirm takeover
levelrail-cli import takeover status
levelrail-cli import takeover rollback
```

`plan` and `run --dry-run` print the preflight and the steps and change nothing. `run --confirm takeover` waits through the restart until the run settles. Add `--json` to any of them for machine output and `--accept-unmanaged` to accept that domains the old proxy serves, and Levelrail does not manage, go offline.

### From the API

| Route | Does |
| --- | --- |
| `GET /api/v1/migration/takeover/plan` | Preflight and plan, plus the latest run |
| `POST /api/v1/migration/takeover/runs` | `{}` is a dry run; `{"confirm": true}` starts a run |
| `GET /api/v1/migration/takeover/runs` | Run history |
| `GET /api/v1/migration/takeover/runs/{run}` | One run with its steps and per-domain results |
| `POST /api/v1/migration/takeover/runs/{run}/rollback` | Undo it |

All five need the root ability. Repeating `POST .../runs` while a run is in flight or done returns that run instead of starting another.

### What the run does, exactly

1. **Records the way back.** The previous listen addresses, TLS settings and the names of the containers it will stop are stored in the control plane database. No secrets are stored.
2. **Stops the old proxy** through the Docker Engine API. The container is stopped, not removed. Nothing else on the old platform is touched.
3. **Switches TLS to this server.** The setting that says a proxy in front terminates TLS is turned off, so the ingress serves HTTPS and requests certificates.
4. **Stores the new listen addresses** `:80` and `:443` in the database. The control plane reads them at start and they win over `APP_INGRESS_HTTP_ADDR`, `APP_INGRESS_HTTPS_ADDR` and systemd socket activation. The systemd drop-in is not edited: the control plane never writes host files, so the drop-in values stay as the baseline rollback returns to.
5. **Restarts the control plane** once, using the same supervised restart as the dashboard restart button.
6. **Verifies each domain** from the control plane over real HTTPS on its own listener, with the domain as SNI and Host header. A domain passes only with a trusted certificate and the reply that only this ingress can give, so an old proxy that came back and answers instead is caught.

### The verification window

If verification has not succeeded for every domain within the window, the run rolls back by itself. The default is 5 minutes. If the control plane restarts again before verification finishes (a crash on bind counts), the next start rolls back instead of trying again.

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_TAKEOVER_WINDOW` | `5m` | How long verification may take before the automatic rollback |
| `APP_TAKEOVER_VERIFY_INTERVAL` | `10s` | Pause between verification passes |
| `APP_TAKEOVER_MIN_DISK_GB` | `2` | Free disk below which the preflight blocks |
| `APP_TAKEOVER_MIN_MEMORY_MB` | `256` | Total memory below which the preflight blocks |

## 5. Rolling back

Click **Roll back** on the run, or run `levelrail-cli import takeover rollback`. Rollback records the intent, restarts the control plane so the previous listen addresses apply, restores the TLS settings, then starts the old proxy container again once the ports are free (it retries while they are released). It is the same path the automatic rollback takes, and repeating it is harmless.

If the old proxy cannot be started, the run ends as "Needs attention" with the container names and the `docker start` command to run by hand. Every step of a run and of a rollback is written to the audit log.

## What can go wrong

| Problem | What you see | What to do |
| --- | --- | --- |
| A host service (nginx, Apache) holds a port | Blocker naming the process | Stop that service, then **Re-check**. The run never stops host services |
| A container that is not a recognised proxy holds a port | Blocker | Stop it or remove its port mapping, then **Re-check** |
| Certificates are issued on first use (HTTP-01) | Warning | Issuance needs port 80 free and DNS correct, and the old proxy's certificates are not reused, so the first visits can fail until it succeeds. Upload a certificate for important domains first, or use DNS-01 |
| Automatic certificates are off | Blocker | Turn on ACME with a contact email in the ingress settings |
| A domain does not resolve here | Blocker or warning | Point the A record at this server. Behind a CDN, a different address is expected |
| The old platform starts its proxy again | Warning, or an automatic rollback | Coolify can restart its proxy when it sees it stopped. Stop the proxy from the Coolify dashboard as well |
| The old proxy has restart policy `always` | Warning | Docker would start it again after a reboot. Set `unless-stopped` |
| The old proxy serves domains Levelrail does not manage | Blocker | Import and route them first, or accept that they go offline |
| systemd holds the ingress sockets | Warning | The control plane ignores them and binds 80 and 443 itself, so a restart on those ports is no longer seamless. Rollback returns to the sockets |
| nginx or Caddy as the old proxy | Warning "Saved, not enforced" | For these two Levelrail only saves configuration snippets for you to apply, it never loads them. After the takeover only Levelrail's own routes answer |

<Warning>
HTTP-01 reachability is the real risk of a same-server takeover: the certificate authority validates over port 80 only after the old proxy has stopped. The preflight cannot prove it ahead of time. Prefer uploaded certificates for critical domains and keep the verification window long enough for issuance.
</Warning>

## Related

- [Run behind an existing proxy](/behind-an-existing-proxy) for coexist mode and managed routes
- [Domains and ingress](/domains-and-ingress) for certificates and the restart-safe sockets
- [Environment variables](/environment-variables)
