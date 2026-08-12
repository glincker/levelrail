# ADR 003: Reverse-dialed gRPC agent with mTLS

Status: Accepted
Date: 2026-08-11

## Context

Levelrail needs a way for the control plane to observe and act on containers
running on remote nodes, including nodes the operator doesn't control the
network perimeter of: a home lab box behind residential NAT, a VPS with no
public inbound firewall rule opened, a machine on a network the operator
can't get a port-forward on. Every tool studied in Phase 0
(`docs-local/research/prior-art-*.md`) solves node communication by requiring
*something* to dial *into* the managed node, whether that's an operator's SSH
session or a Swarm cluster member reaching a peer directly. That requirement
is the thing this ADR rejects, not any one specific competitor's
implementation of it.

## Decision

A persistent agent binary runs on every managed node and dials **out** to
the control plane over gRPC secured with mutual TLS. Consequences, per
CLAUDE.md 4.3: no inbound ports open on managed servers, the setup works
behind NAT and residential connections unmodified, and the agent streams
Docker events up to the control plane rather than the control plane polling
down.

Enrollment: the control plane issues a one-time join token; the agent
exchanges it for a client certificate at first connect, with rotation on a
schedule after that. The agent talks to the local Docker socket via the
Engine API directly, the same "never shell the CLI" invariant ADR 002
already establishes for the control plane's own `internal/docker` wrapper,
just enforced on the node side of the connection instead.

Single-node mode (the common case for a while, per Phase 1) runs the agent
in the same process as the control plane, communicating over an in-memory
transport that implements the same interface the gRPC transport implements.
The reconciler and everything above the transport boundary never knows
whether it's talking to a local in-process agent or a remote one over mTLS,
which keeps the code path identical from one node to ten (Phase 3's exit
criterion depends on this holding).

## Rejected alternatives

- **SSH as the primary or sole transport** (Coolify, Dokku, and Kamal, plus
  Dokploy for lifecycle operations even though its core deploy mutation goes
  through a real API client). This is the most heavily evidenced rejection
  in the whole research set:
  - **Kamal's own highest-comment GitHub issue, by a wide margin, is a pure
    transport failure.** `#1619` (76 comments, `prior-art-kamal.md`
    Q6/Implications point 1): `Net::SSH::Disconnect` breaks every single
    Kamal command for affected users, while a manual `ssh` to the same host
    works fine. Nothing about this issue is about deploy logic, build
    correctness, or health checking, it's Kamal's SSHKit/`net-ssh` session
    handling misbehaving under conditions plain OpenSSH doesn't hit. A
    reverse-dialed connection that's established once and held open,
    instead of re-negotiated fresh on every single CLI invocation, removes
    this entire failure category rather than requiring an eventual fix to
    SSHKit's edge cases.
  - **Coolify's SSH multiplexing needs its own cleanup job to stay
    healthy.** Every remote operation in Coolify goes through
    `instant_remote_process()` (`bootstrap/helpers/remoteProcess.php`),
    which builds a literal heredoc shell string and pipes it over
    `SshMultiplexingHelper::generateSshCommand()`
    (`app/Helpers/SshMultiplexingHelper.php:178-217`). Multiplexing
    amortizes but doesn't eliminate per-call SSH overhead, and it leaves
    behind `ControlMaster` sockets that need a dedicated hourly
    `CleanupStaleMultiplexedConnections` job just to reap
    (`prior-art-coolify.md` section 5). That's operational complexity that
    exists purely to manage the transport, not to serve a user-facing
    feature.
  - **Dokku has no persistent process at all**, which is a legitimate design
    choice on its own terms (see ADR 002's rejected-alternatives entry for
    Kamal on the zero-idle-cost trade), but it means Dokku structurally
    cannot maintain a live Docker event stream between commands. Its
    boot-time container recovery is a systemd unit explicitly commented in
    its own source as `# temporary hack for
    https://github.com/dokku/dokku/issues/82`
    (`prior-art-dokku.md` Q5), and SSH-key-as-the-entire-auth-model produces
    its own onboarding failure class (`#1242`, 55 comments, users not
    understanding `authorized_keys` *is* the account system). A reverse-
    dialed agent with certificate-based enrollment is a structurally
    different identity model, not a variation on the same one.
  - **Dokploy is the nuanced case worth being precise about.** Its core
    deploy mutation (`service.update()`/`createService()`) does go through
    a real `dockerode` client over an SSH-tunneled Engine API socket, not
    text-parsed CLI output. But a large surface of other operations,
    lifecycle commands (`docker service scale`, `docker service rm`),
    system/image/volume/builder pruning, and the entire build pipeline
    (git clone plus build command), is a single large shell string executed
    over an `ssh2` exec channel (`prior-art-dokploy.md` section 5,
    `execAsyncRemote`). Whichever path is used, both require the control
    plane to be able to open an outbound TCP connection to port 22 on every
    managed server, exactly the reachability requirement this ADR is
    designed around not needing.
- **Requiring nodes to be mutually reachable, the Swarm-cluster pattern**
  (CapRover, and Dokploy's multi-node mode). This isn't literally an SSH
  rejection, but it's the same underlying rejected pattern: *something* has
  to be able to dial *into* a managed node. CapRover runs `swarmInit()`
  unconditionally (`src/docker/DockerApi.ts:157`) and joins additional nodes
  via Swarm join tokens; Swarm's routing mesh then depends on manager-to-
  worker and worker-to-worker reachability to route a published port to
  whichever node currently runs the task. CapRover's own most-discussed
  networking issue, `#1628` ("Worker node can't reach service on another
  node," 29 comments, `prior-art-caprover.md` section 6), is the direct,
  empirical cost of that requirement: as soon as nodes aren't uniformly
  reachable from each other, the architecture breaks. A reverse-dialed
  agent has no node-to-node or inbound-to-node reachability requirement at
  all, only outbound-to-control-plane, which is the one direction that
  works from behind NAT and on a residential connection by default.

## Consequences

- The control plane has to hold many concurrent gRPC streams open and manage
  reconnection, backpressure, and version negotiation as real, tested
  concerns (explicitly scoped as Phase 3 work: "Real gRPC transport
  replacing the in-process one, with reconnection, backpressure, and
  version negotiation"). This is a standing cost SSH-per-command tools don't
  carry between invocations. `prior-art-kamal.md`'s own framing is the
  right way to think about this trade: "we pay Kamal's exact idle-cost delta
  to get event-driven state and NAT-friendly multi-node," not "our
  architecture is simply better."
- Enrollment needs a join-token UX (Phase 3) instead of "paste in a root SSH
  key," which is a materially higher one-time setup cost in exchange for a
  materially more reliable steady-state transport. That trade needs to be
  designed carefully, since a confusing enrollment flow would reproduce the
  exact onboarding-friction failure class Dokku hits with its SSH-key model.
- Certificate rotation is an ongoing operational surface (key material on
  every node, rotation scheduling, revocation on node removal) that none of
  the SSH-based competitors have to build, because they reuse the
  operator's own SSH keys instead of issuing their own PKI.
- Because the agent and the in-process single-node transport implement the
  same interface, the reconcile loop, the observability pipeline (4.8), and
  the deploy state machine all get tested once against a fake transport and
  don't need a second code path for "how does this behave with ten nodes
  instead of one." Phase 3's exit criterion (moving an app to a second
  server without breaking its database connection) is a test of this
  interface holding, not a new subsystem.
- The agent talking to the Docker socket directly, never shelling the CLI,
  means node-side failures show up as typed Engine API errors instead of
  parsed stderr text, which is the same category of robustness ADR 002
  already committed to for the control plane's own Docker access.
