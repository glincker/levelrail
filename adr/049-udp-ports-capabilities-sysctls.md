# ADR 049: UDP ports, a capability allowlist and namespaced sysctls

Status: Proposed

Date: 2026-10-27

## Context

VPN and remote access apps (wg-easy, RustDesk server, Headscale STUN,
DNS on UDP 53, game servers) cannot run today. Verified on
`release/v0.8.9`:

1. **UDP is rejected, not dropped.** Short form `51820:51820/udp` fails in
   `parsePort` (`internal/compose/yaml.go:277`, `Atoi` on `51820/udp`), long
   form `protocol: udp` fails at `yaml.go:247`. `compose.Port`
   (`internal/compose/compose.go:163`) has no protocol, and the package doc
   says "no UDP" (`compose.go:11`). `store.DesiredService` carries one
   `Port` plus optional `HostPort` (`internal/store/service.go:216`), no
   protocol, and the reconciler publishes exactly that one TCP binding
   (`internal/reconcile/application/controller.go:2731`).
2. **Lower layers already speak UDP.** `docker.PortBinding.Protocol`
   (`internal/docker/runtime.go:36`) feeds `nat.NewPort(proto, ...)`
   (`internal/docker/client.go:790`). Host exposure restrictions accept
   udp (`internal/exposure/manager.go:122`), node firewall rules carry
   `Protocol` (`internal/reconcile/firewall/controller.go:135`). App streams
   (Caddy layer4, `internal/ingress/layer4.go`) store a protocol column but
   the API refuses anything but tcp (`internal/api/app_streams.go:65`,
   `internal/store/app_stream.go:14`). The edge firewall
   (`internal/api/firewall_edge_view.go`, `internal/edgefw`) inspects HTTP
   requests only and has no UDP view.
3. **`cap_add` is rejected.** `validateHostAccess` rejects any `cap_add`
   (`internal/compose/security.go:69`), citing the socket proxy rule
   `cap_add_disallowed`. That proxy already has a narrow grant path:
   `dockerguard.GrantableCaps = NET_ADMIN, NET_RAW`
   (`internal/dockerguard/policy.go:14`) allowed only for a create declared
   with them (`validate.go:83`), used today by the platform's own egress
   sidecar (`internal/reconcile/application/egress.go:214`), which shares
   the app container's network namespace. The default profile drops ALL
   and adds back `MinimalCapabilities` (`internal/docker/hardening.go:48`),
   with no-new-privileges and a PID limit. `APP_CONTAINER_HARDENING_CAP_ADD`
   adds caps to every container on the host.
4. **`sysctls` is ignored with a notice**, not rejected
   (`internal/compose/unsupported.go:51`). `docker.ContainerSpec` has no
   sysctl field and dockerguard has no sysctl rule.

No cloud firewall ADR exists on origin; this ADR does not depend on one.

## Options

**(a) UDP publishing end to end.** Add `PublishedPorts []{ContainerPort,
HostPort, Protocol}` to the spec and `DesiredService` (new table or JSON
column), parse `/udp` and `protocol: udp` in compose, emit one
`docker.PortBinding` per entry with the same `bindaddr.Resolve` treatment
the TCP port gets (default `private`). UDP goes straight to a host port,
not through Caddy: the HTTP proxy cannot carry it, and a layer4 UDP proxy
on the control plane would hide client source addresses that WireGuard
roaming and RustDesk hole punching depend on, and would only serve the
control plane host. Health checks stay on the TCP port or a container
healthcheck; a UDP port gets no reachability probe, only "bound" status
from the observed `Ports`. Dashboard, CLI and API show `port/udp`
everywhere a host port shows today, and a public UDP bind gets the same
exposure warning and firewall allow rule flow as a public TCP host port.

**(b) Capability allowlist.** A named set, starting with `NET_ADMIN` and
`NET_RAW`, read from env (`APP_GRANTABLE_CAPS`, default those two, values
outside the set dockerguard grants are refused at startup). Granted per
template or per app, never `privileged`, never `SYS_ADMIN`, `SYS_MODULE`
or `SYS_PTRACE`. A grant requires an owner or admin confirmation at deploy
time with a visible warning, is written to audit with the actor, is shown
on the app page and in `app show`, and is denied by default for imported
compose files, docker run imports and PR previews (previews inherit no
grant). Rejected alternative: the host wide
`APP_CONTAINER_HARDENING_CAP_ADD`, which grants every container.

**(c) Sysctls.** Only namespaced net sysctls, allowlisted from env
(`APP_ALLOWED_SYSCTLS`, default `net.ipv4.ip_forward`,
`net.ipv4.conf.all.src_valid_mark`). Anything else stays a blocking error
rather than today's silent notice. Needs `ContainerSpec.Sysctls` and a
dockerguard rule that rejects sysctls outside the list.

**(d) Host networking** stays out of scope. `network_mode: host`
remains rejected (`security.go:53`).

**(e) Template coverage.**

| Template | Needs | New risk |
| --- | --- | --- |
| RustDesk server | (a) only: 21116/udp plus TCP 21115 to 21119 | Public relay, abuse traffic |
| Headscale with STUN | (a) only: 3478/udp | STUN reflection, small |
| DNS servers | (a), UDP 53 | Open resolver amplification if public |
| wg-easy | (a), (b) NET_ADMIN, (c) both sysctls, host `wireguard` kernel module | Routes traffic into app network and node LAN |

## Recommendation

Ship (a) first, alone. It adds no privilege, unblocks RustDesk, Headscale
STUN and DNS, and reuses existing bind address, firewall and exposure
paths. Ship (b) and (c) together later, gated on the threat model below,
since wg-easy is the only target that needs them.

Ports: TCP host port ranges stay out of scope; RustDesk needs five
single TCP ports, which `PublishedPorts` covers.

## Security analysis

- **UDP:** no session, so spoofed sources and amplification matter.
  Default bind stays `private`; a `public` UDP bind needs the same
  confirmation a public TCP host port gets. Docker published ports bypass
  the host INPUT chain, so restrictions must land in DOCKER-USER, which
  `internal/exposure` already manages; UDP there is untested.
- **NET_ADMIN** is the real container escape surface: it exposes netfilter
  and nf_tables, the source of several kernel privilege escalations. It is
  scoped to the container's own network namespace, but a kernel bug is
  not. Required before shipping: a minimum kernel version check from env,
  default seccomp profile kept, no-new-privileges kept, and documentation
  that NET_ADMIN trusts the image author with the kernel.
- **Egress conflict:** the egress sidecar enforces policy with iptables in
  the app's network namespace. An app with NET_ADMIN can flush those rules,
  so an egress policy and a NET_ADMIN grant must be mutually exclusive, or
  egress must be reported as unenforced.
- **NET_RAW** allows spoofed packets and ARP tricks against other
  containers on the same bridge network.
- **Who may grant:** owner or admin role only; developers may request. A
  template marks the grant as required, it never applies it silently.

## Compatibility and migration

Existing apps keep their single `Port`. `PublishedPorts` is additive, empty
by default, and its hash joins the applied config hash so a change
recreates the container. Compose files that failed on `/udp` now parse.
`sysctls` moves from notice to error only for keys off the allowlist,
which is a visible change for imported files that set them today; those
get a clear message, not a silent drop.

## Implementation slices

1. **S1, M:** `PublishedPorts` in spec, store migration, compose parse of
   udp, reconciler bindings, applied hash. Files: `internal/compose`,
   `internal/spec`, `internal/store`, `internal/reconcile/application`.
2. **S2, S:** API, CLI, dashboard display, exposure warning and firewall
   allow rule for UDP host ports. Files: `internal/api`, `cmd/levelrail`,
   `web/src`.
3. **S3, S:** RustDesk and Headscale STUN templates. Files:
   `internal/catalog`.
4. **S4, M:** threat model note, kernel version gate, `ContainerSpec.Sysctls`,
   dockerguard sysctl rule. Files: `internal/docker`, `internal/dockerguard`.
5. **S5, M:** grant model, approval flow, audit, app page badge, preview and
   import denial, egress exclusivity. Files: `internal/api`, `internal/store`,
   `internal/compose`, `internal/preview`, `web/src`.
6. **S6, S:** wg-easy template.

## Testing plan

Unit tests per slice: compose udp parsing, binding protocol, applied hash,
grant denial for previews and imports, dockerguard refusing caps and
sysctls outside the env lists. One real server run per release line: a
UDP echo from outside the node, DOCKER-USER restriction blocking it, and
a wg-easy client handshake.

## What cannot be verified locally

UDP through Docker Desktop on macOS, DOCKER-USER behavior for UDP on
nftables engines (Podman with netavark), the wireguard kernel module, and
real kernel exposure of NET_ADMIN.

## Consequences

The VPN and remote access category opens in two steps, with the privileged
step behind an explicit, audited human decision. The platform takes on a
second publishing path (direct UDP host ports) beside Caddy.

## Open questions

- Should `net.ipv6.conf.all.forwarding` join the default sysctl list?
- Does dockerguard currently pass an unknown `HostConfig.Sysctls` through?
  Not verified; S4 must check.
- Minimum kernel version for NET_ADMIN, and who maintains it.
- Should UDP host ports work on remote nodes in S1, given node firewall
  rules are local only today (`internal/reconcile/firewall/controller.go:128`)?
