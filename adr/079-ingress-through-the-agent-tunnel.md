# ADR 079: Ingress through the agent tunnel

Status: Proposed

## Context

A node without a usable mesh path, such as a Mac running the agent, could not serve public domains: ingress needs a route to the app's port and the port stays on loopback. The status showed `CrossNodeIngress False NoMeshIngressPath`.

## Decision

Behind the `agent-ingress-tunnel` flag (default off), when a node has no usable mesh path and its agent advertises `ingress_tunnel`, the ingress controller points the route at a local unix socket owned by a tunnel gateway. Each connection to the socket becomes one tunnel over the agent's existing outbound connection, and the agent resolves the local port of its own service. One socket exists per (node, service, container port) in a private directory under the data dir. The controller registers sockets for routed targets each pass and removes the rest. Caddy dials `unix//<absolute path>`, checked against the pinned Caddy parser. The mesh stays preferred whenever its path is usable. With the flag off or an older agent, behaviour is unchanged. The status reports `CrossNodeIngress True AgentTunnel` when the route through the agent tunnel is in place. The agent sends traffic only to a running, Levelrail managed container of the service that is not failing its healthcheck and is not the older release of a healthy successor, rotating replicas per tunnel open. The tunnel lives for the control plane's absolute lifetime, not the open handshake timeout.

## Consequences

Mesh-less nodes serve public domains without inbound ports. The control plane carries every byte for those apps, so its uplink is the limit. The flag is read at server start. Real-host behaviour (NAT, sleep and wake, WebSockets) is not yet verified.
