# ADR 025: Close the control plane restart gap with systemd socket activation

Status: Accepted

Date: 2026-10-05

## Context

ADR 005 embeds Caddy in the control plane process. A restart or upgrade of the
control plane therefore closes ports 80 and 443: measured at about 4s on a real
droplet and about 1.7s locally, during which every domain refuses connections
although the containers never stopped. A site that faces real traffic cannot
accept that on every upgrade.

## Decision

Let systemd own the listening sockets. `install.sh` (opt-in with
`LEVELRAIL_SOCKET_ACTIVATION=1`) writes `levelrail-http.socket` and
`levelrail-https.socket`. The control plane reads `LISTEN_FDS` and
`LISTEN_FDNAMES`, configures Caddy with `fd/N` listen addresses, serves the
HTTP to HTTPS redirect from its own server on the inherited port 80, and gives
in-flight requests a 3s grace period. While the process is down the kernel keeps
accepting into the socket backlog, so a restart delays connections instead of
refusing them.

It is opt-in because switching an existing install stops the service once and
changes who owns ports 80 and 443. Hosts without systemd, Docker deployments and
dev runs are unaffected: with no `LISTEN_FDS` the control plane binds the ports
itself as before.

## Consequences

- HTTP/3 is not available in this mode (an inherited TCP socket cannot carry
  QUIC; a UDP socket would need its own fd and was not built).
- ACME HTTP-01 is disabled in this mode (Caddy attaches its challenge handler by
  port number, which an fd address does not have). TLS-ALPN-01 on 443 and DNS-01
  still issue certificates.
- A handful of TLS handshakes in progress at the instant the old process stops
  can still fail. Measured: 0, 3, 28 and 35 of about 2700 requests at 200 rps,
  against 335 refused of about 5000 without sockets.
- The sockets stay open while the service is stopped on purpose, and a
  connection to them starts the service again.
- Validated end to end on Ubuntu 24.04 under real systemd by
  `scripts/test-install-socket-activation.sh`: systemd holds 80 and 443, two
  restarts under a connect probe refused nothing, and switching back and
  uninstalling remove the units.

## Rejected alternatives

- **SO_REUSEPORT with two overlapping processes.** The new process binds the
  same port, the old drains. Connections already in the old socket's accept queue
  are reset when it closes, which is the failure we are removing, and it needs a
  supervisor that starts the new process before stopping the old, which systemd
  restart does not do.
- **Graceful drain on SIGTERM that keeps accepting while the new process
  starts.** Needs two processes alive at once (same supervisor problem) or the
  old one to stay up longer, and still leaves the listener gap between the old
  closing and the new binding.
- **Moving Caddy out into a sibling process or container.** Removes the gap and
  also the single binary, in-process admin API and shared certificate storage
  that ADR 005 chose. Too large a change for a gap of a few seconds.
- **Caddy's own `caddy reload` in place.** Applies config without closing
  listeners, but only for config changes inside a running process, not for a
  process restart or binary upgrade, which is the case that matters.
- **Making it the default.** Rejected for now: changes who owns 80 and 443 on
  existing installs and could not be exercised on the shared QA servers here.
