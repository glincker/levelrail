---
title: "App ports and exposure"
description: Change an app's container port live, choose between proxy-only and direct IP and port access, and test reachability, without port fights between apps.
---

Many apps declare the same container port (3000, 8080). That is fine: Docker gives each container its own host port, and the embedded ingress routes by hostname on ports 80 and 443, so apps never compete for those. The **Network** tab of an app adds the controls for the cases where you want more.

## Modes

| Mode | What it does | URLs |
|---|---|---|
| Through the proxy (default) | The app is only reachable through the ingress, by its domain or the automatic `sslip.io` hostname. Other apps on the server reach it by name. No host port is opened. | `https://app.example.com`, `https://web.1-2-3-4.sslip.io` |
| Direct IP and port | The container is also published on a host port on every interface, and an allow rule for that port is added to the [host firewall](/host-firewall). | `http://203.0.113.10:24100` |

Direct mode is plain HTTP. It bypasses TLS, basic auth and sign-in gates configured on the domain, so use it to check that an app runs, not as its public address. It is only available for apps on the control plane's own node.

## Change the port or mode

In the app's Network tab, edit the container port and pick a mode, then choose **Preview changes**. The preview lists exactly what changes (container port, host port, bind address, firewall rule) and whether the container is recreated. **Apply** writes the desired state and the reconcilers do the rest: the proxy is retargeted, and only the container is recreated (the image is not rebuilt or pulled). A running container whose published port or bind address no longer matches the desired state is removed and recreated on the next reconcile pass (a change of container port, host port or bind address always does this); `PUT .../exposure` answers with `outcome` of `unchanged`, `applied` or `recreating`. Pinned host ports cannot overlap, so a brief gap during the recreate is expected. Creating or updating an app with a `host_port` that another app, the control plane or a local process holds is refused with 409 naming the holder. While a redeploy is in flight, `GET .../network` keeps reporting the serving container's host port with `rollout_in_progress: true`. Every change is recorded in the app's activity. **Switch back to proxy only** reverses it in one click and removes the firewall rule it added.

```bash
levelrail-cli apps exposure web                                 # show mode, URLs, host ports in use
levelrail-cli apps exposure web --port 8000 --preview           # dry run
levelrail-cli apps exposure web --mode direct --host-port 24100 # publish and open the firewall
levelrail-cli apps exposure web --mode proxied                  # back to proxy only
levelrail-cli apps exposure web check                           # probe reachability
```

`apps ports` is an alias of `apps exposure`. The API is `GET|PUT /api/v1/apps/{name}/exposure`, plus `POST .../exposure/preview` and `POST .../exposure/check`.

## Host port choice and conflicts

Leave the host port empty and a free one is picked from `20000-29999` (override with `APP_EXPOSURE_PORT_RANGE=lo-hi`), below Docker's own ephemeral range. The pick avoids ports held by this control plane (80, 443, 8080, 8088, 8443, 9443, the configured ingress and SSH ports), pinned ports and streams of other apps, ports bound by other running containers, and anything the kernel reports as listening on the host. A port you choose that is held is refused with who holds it. The table at the bottom of the panel lists the ports in use by apps and the control plane.

## Test reachability

**Test from here** connects to the container's published port and requests the app's readiness path (or `/`), then does the same through the direct port and through the ingress URL. It reports the status code and latency, and which hop failed:

| Failure | Meaning |
|---|---|
| Container not running | Nothing to reach. Start or redeploy the app. |
| Container not listening | Up, but nothing answers on the published port: wrong container port, or still starting. |
| Firewall closed | The server address does not answer on the direct port: host firewall or cloud security group. |
| Certificate not issued | The ingress serves its temporary certificate while the public CA is retried. |
| Proxy route missing | The ingress answered 404: the domain is not attached to this app. |
| Upstream failing | The route exists but the app returns 502 to 504. |

The probe runs from the control plane host only. It cannot prove a port is open to the whole internet: use an outside check for that.

## Certificates when many apps start at once

Automatic hostnames and domains are issued by the embedded ACME client, which serializes challenges on the single port 80 and 443 listeners and backs off on its own after a failure. If the public CA rejects or throttles a host (a rate limit, closed ports, DNS), the host keeps answering over HTTPS with a certificate from the platform's own CA, and the public CA is retried on the fallback certificate's renewal cycle (`APP_INGRESS_FALLBACK_CERT_LIFETIME`, default `6h`). The reason (`rate_limited`, `unreachable`, `dns`, `caa`) is shown next to the certificate. See [Domains and ingress](/domains-and-ingress#firewall-ports-80-and-443).
