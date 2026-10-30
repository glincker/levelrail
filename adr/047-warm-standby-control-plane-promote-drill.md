# ADR 047: Warm standby control plane with a manual promote drill

Status: Proposed

Date: 2026-10-25

Builds on ADR 037 (snapshot replica) and ADR 039 (WAL shipping chosen,
`internal/walship` prototype). Does not supersede either.

## Context

The control plane is one machine. Running workloads survive its loss (ADR
025, docs/resilience.md), but deploys, ingress changes, the dashboard and the
API do not. Today recovery is the disaster recovery runbook: provision a new
machine, open escrow, restore the newest off-box backup or replica, start.
That works and is tested, but RPO is the backup or replica interval and RTO
is however long a human takes to do it from scratch.

The next bounded step is a second machine that is already provisioned and
already holds recent state, plus one command that turns it into the primary.
This ADR scopes that for v0.8.6 as a drill only: no automatic failover.

What the code does today (verified on this branch):

- `internal/walship` ships WAL frames to a `Sink` (only `DirSink` exists),
  restores a consistent prefix, and is imported by nothing outside its own
  package. No setting starts it.
- The agent reads one address, `APP_CONTROL_PLANE_ADDR` (`host:port`), in
  `cmd/levelrail-agent/main.go`. There is no address list. Each reconnect
  calls `agent.RunSession`, which builds a new `grpc.NewClient(addr, ...)`,
  so a DNS name is re-resolved on every reconnect (gRPC's default `dns`
  resolver). An IP address is fixed for the agent's lifetime.
- Session TLS uses `RootCAs` from the agent's identity file (the agent CA)
  with normal hostname verification. The control plane's listener
  certificate is issued at boot by the agent CA for
  `APP_AGENT_ADVERTISE_HOST` (`cmd/levelrail/boot_agent.go`,
  `internal/agent/pki.go`). The fingerprint pin skips hostname checks, but
  only during enrollment and re-enrollment.
- The agent CA lives in the data directory (`agent-ca.crt.pem`,
  `agent-ca.key.pem`), not in the database. It travels by the escrow bundle.
  If a control plane boots without it, it generates a new CA and every agent
  must re-enroll.
- The master key is never in the database or a backup (`master.key` or
  `APP_MASTER_KEY`). `alerting.db` and the telemetry database are not in any
  backup today (docs/control-plane-backup.md).
- The mesh hub endpoint also comes from `APP_AGENT_ADVERTISE_HOST` and must
  be an IP for WireGuard (`cmd/levelrail/mesh.go` warns on a hostname).

## Decision

Ship a warm standby with a manual `levelrail standby promote` in v0.8.6. The
standby is a second control plane machine in a passive mode: it does not
serve the API, the agent listener or ingress, and it does not reconcile. It
follows the primary's WAL stream and keeps the static key material an
operator copied to it once. Promotion is a human decision, run by a human.
Automatic failover is out of scope until fencing has a witness (see "Not
covered").

### What the standby holds

| State | How it gets there | Freshness |
| --- | --- | --- |
| `levelrail.db` (all desired state, users, token hashes, wrapped secrets, deploy history, ingress config) | WAL stream from the primary through `walship`, restored continuously into a staging file | Seconds (ADR 039 measured p99 226 ms at 100 ms interval) |
| Master key | Operator copies it once at standby setup (from escrow or `APP_MASTER_KEY`) | Static; re-copy after a master key rotation |
| Agent CA cert and key | Operator copies once (escrow bundle carries both) | Static |
| Control plane listener cert | Not copied. Reissued by the agent CA at promote for the standby's advertise host | Fresh at promote |
| Caddy ACME account and certificates | Ride in `levelrail.db` if they are stored there (ADR 005 says cert storage is in the database); slice 1 must verify this and list any file under the data dir that is not | As fresh as the WAL stream, or re-issued by ACME after promote |
| `alerting.db` | Not shipped in v0.8.6. Operator copies it by hand, as the backup docs already say | Manual |
| Telemetry database | Not shipped. Metrics and logs are node-local anyway (ADR 008) | Lost for the control plane's own node |
| Age backup identity | Not on the standby. It does not need to decrypt anything: the stream is to a sink both machines can reach and segments are encrypted to a key the standby holds (see Security) | n/a |

The standby also writes a marker, `standby.state` in the data directory,
with the primary's install id, the last applied generation and epoch and
the timestamp of the newest applied segment. The control plane refuses to
start in normal mode while that marker says `standby`, so a reboot of the
standby cannot accidentally make it a second primary.

### Promote sequence

`levelrail standby promote` runs on the standby, offline, the same way
`levelrail restore` does:

1. Read `standby.state`; refuse if the install id does not match the staged
   database or if `--expect-primary-down` was not given (the operator
   asserts the old primary is stopped or unreachable; see fencing).
2. Pull and apply every remaining segment from the sink. Report the newest
   applied commit time. This is the RPO evidence.
3. Write the fence record to the sink (below). Refuse to continue if the
   write fails, unless `--no-fence` is passed, which is printed in red and
   recorded in the audit log.
4. `PRAGMA integrity_check`, schema version check, install id check: the
   existing restore checks from `internal/cpbackup`.
5. Atomically install the database as `levelrail.db`, keeping any old file
   as `.pre-restore`, using the existing restore swap.
6. Bump a `control_plane_epoch` value stored in the database by one and
   record `promoted_at`, the old primary's last segment time and the
   operator.
7. Flip `standby.state` to `promoted` and print the next steps: start the
   service, move DNS, check agents.
8. On start, the control plane reissues its listener certificate for its own
   `APP_AGENT_ADVERTISE_HOST` from the copied agent CA and starts a fresh
   WAL generation of its own to the sink (it is now the shipper).

### Fencing rule (split brain)

Without a witness there is no way to stop a primary that is up but
partitioned. The v0.8.6 rule is cooperative fencing through the sink, which
both machines already must reach:

- Promote writes `fence.json` to the sink: new epoch, promoted install
  identity, time.
- The primary's shipper reads the fence object before each checkpoint and
  at least every `APP_CONTROL_PLANE_FENCE_CHECK_INTERVAL` (default 10s).
  When the fence epoch is greater than its own, the primary enters
  read-only: it stops the shipper, rejects API writes with 503 and a reason,
  stops the reconciler and the agent listener, and raises an alert. It
  never resumes on its own; the operator must decommission it or turn it
  into a standby of the new primary (`levelrail standby init`).
- A primary that cannot reach the sink for longer than
  `APP_CONTROL_PLANE_FENCE_GRACE` (default 2m) keeps serving (the sink
  being down must not take production down) but alerts loudly. This is the
  known split-brain window: a primary partitioned from the sink and from the
  operator, but still reachable by some agents, can accept writes after a
  promote. The operator's `--expect-primary-down` assertion is the only
  guard. This is why promote is manual.
- Agents add a second, independent guard: each session handshake carries
  the control plane epoch, the agent remembers the highest it has seen in
  its identity directory, and refuses a session from a lower epoch. Once an
  agent has talked to the new primary, the old primary cannot drive it
  again even if DNS flaps back.

### How agents re-dial the new address

There is no address list, so the decision is:

- Agents must be configured with a DNS name in `APP_CONTROL_PLANE_ADDR`,
  not an IP, for a promote to work without touching nodes. A DNS name is
  already re-resolved on every reconnect. The operator moves the DNS record
  (or a floating IP behind a stable name) during promote. Low TTL (60s) is
  documented as required.
- Both control planes must set `APP_AGENT_ADVERTISE_HOST` to that same DNS
  name, so each reissued listener certificate carries the name agents
  verify. The standby's own IP can be an additional SAN.
- This conflicts with the mesh hub, which needs an IP. Slice 2 adds
  `APP_AGENT_CERT_HOSTS` (comma separated extra SANs) and
  `APP_MESH_HUB_ENDPOINT` (IP for WireGuard), so the gRPC name and the mesh
  endpoint stop sharing one setting. After promote, the new primary
  publishes its own hub endpoint and agents pick it up through the existing
  peer distribution on their next session.
- An agent installed with an IP address gets a doctor warning
  ("standby promote needs a DNS name here") and a guided fix that rewrites
  the agent's env. A real address list (`APP_CONTROL_PLANE_ADDRS`) is
  deferred: with a fence it adds little, and without one it invites agents
  to talk to both machines.

### Certificate and trust-root implications

- Same agent CA on both machines is required. Then no agent re-enrolls,
  every existing client certificate stays valid, and renewal works against
  the new primary, because certificate issuance state (serials, node
  records, revocations) is in the database.
- A standby that boots without the CA files must refuse promote, not
  generate a new CA. Silent CA regeneration would orphan every node.
- Two machines holding the CA key doubles its exposure; see Security.
- Public ingress certificates: if ACME state is in the database, the new
  primary serves the existing certificates; ingress on worker nodes is not
  affected at all. If the control plane node itself is an ingress node, the
  public DNS for those domains must also move, which is the operator's job.
  ACME re-issuance after a move hits rate limits if done for many domains
  at once; the drill measures whether any re-issue happens.

### RPO and RTO: how we measure, what we publish

- RPO: a probe writes one marker row (a deploy note on a canary app) every
  second on the primary. At the kill, record the newest marker the primary
  committed (read from the dead primary's disk after the drill) and the
  newest marker present after promote. RPO is the time between them.
- RTO: from the moment the primary is killed to the first successful API
  write on the new primary, and separately to the moment the last agent has
  an open session (from the agent connection log). Both reported, because
  "API back" and "fleet back" differ by the DNS TTL.
- Each drill run writes a JSON result (`rpo_ms`, `rto_api_ms`,
  `rto_agents_ms`, database size, segment interval, agent count, sink type)
  under `docs/benchmark-results/`.
- We publish only measured drill results, with the setup next to them, in
  docs/resilience.md, and say plainly that it is a manual promote. No
  "HA" or "no single point of failure" wording until fencing has a witness
  and automatic failover exists.

### Failure modes

| Failure | Effect | Handling |
| --- | --- | --- |
| Sink down | Primary WAL grows (ADR 039), standby falls behind | Size alarm, then drop the hold and start a new generation when the sink returns; standby shows lag |
| Standby falls behind | Larger RPO at promote | `standby status` shows lag; alert past `APP_CONTROL_PLANE_STANDBY_MAX_LAG` |
| Missing segment or sequence jump | Replay stops at a consistent prefix | Standby requests a new generation; promote reports the prefix it has |
| Corrupt segment | SQLite recovery truncates the replay | Same as above; never a corrupt database |
| Promote while primary is alive and reachable by sink | Primary self-fences within the check interval | Writes in that window are lost; reported by promote |
| Promote while primary is partitioned from sink | Split brain until operator intervenes | Documented; agent epoch guard limits it to agents that have not met the new primary |
| DNS not moved or cached | Agents keep dialing the dead machine | RTO includes TTL; doctor on the new primary lists agents not yet connected |
| Agent pinned by IP | Never reaches the new primary | Doctor warning before any drill |
| Master key rotated after standby setup | Secrets unreadable on the new primary | Standby verifies a known-wrapped canary secret on every status run and warns |
| Schema migration on the primary | Standby binary older than the stream | Standby refuses to apply segments past its schema; upgrade standby first |

### What an operator must do by hand

- Set up the standby: install the same release, copy master key and agent
  CA files, point it at the sink, run `levelrail standby init`.
- Keep `alerting.db` copied (until it is replicated).
- Decide that the primary is gone and pass `--expect-primary-down`.
- Move DNS for the agent address, the dashboard and any public domains
  served by the control plane node.
- Decommission or re-init the old primary as a standby. Never just restart
  it.
- Re-copy the master key to the standby after a rotation.

### Security considerations

- The standby holds everything the primary holds: master key, agent CA key,
  the database with token hashes and wrapped secrets. It is a second primary
  for an attacker. Same hardening, same access control, no shared login with
  less trusted machines.
- WAL segments are encrypted per segment to an age recipient whose identity
  only the standby holds (`APP_CONTROL_PLANE_STANDBY_IDENTITY_FILE`), reusing
  the `cpbackup` age code, so the sink sees ciphertext only. This is a
  separate key from the operator's backup identity, so revoking a standby
  does not touch backups.
- The fence object must be authenticated: signed with the agent CA key,
  verified by the primary, so someone with only sink write access cannot
  fence production down. Sink write access still lets an attacker delete
  segments (availability, not confidentiality).
- Revoking a standby: rotate the master key and the agent CA, which today
  means re-enrolling every agent. That cost is stated in the docs.
- Promote is a local command on the standby, never an API call, like
  restore.

## Testing plan

1. Unit and package tests: promote refuses without CA files, without the
   marker, with a mismatched install id; fence epoch read and read-only
   switch; agent epoch guard; listener cert reissue with the configured
   SANs.
2. Two local control plane containers sharing only a sink directory (a
   bind-mounted volume or a local S3 server), one agent container dialing a
   DNS name served by a test resolver or `/etc/hosts` swap. Kill the primary
   with SIGKILL during deploy churn, promote, swap the name, measure RPO and
   RTO, then start the old primary and assert it fences and that the agent
   refuses it. Five runs.
3. Two droplets via `scripts/qa/droplet.sh` plus one worker droplet, a real
   S3 bucket, a DNS record with 60s TTL, real ACME on a test domain. Same
   kill, promote and measurement, plus a partition test (firewall the old
   primary from the sink only). Destroy droplets after.

## Slices

| Slice | Model | Files | Env |
| --- | --- | --- | --- |
| 1. Encrypted object-store sink and wiring of `walship` behind a setting; ACME storage inventory | Sonnet | `internal/walship/*`, `cmd/levelrail/walship_wiring.go` (new) | `APP_CONTROL_PLANE_WAL_SHIP_INTERVAL` (off by default), `APP_CONTROL_PLANE_WAL_CHECKPOINT_BYTES` |
| 2. Split listener SANs from mesh endpoint | Sonnet | `cmd/levelrail/boot_agent.go`, `cmd/levelrail/mesh.go` | `APP_AGENT_CERT_HOSTS`, `APP_MESH_HUB_ENDPOINT` |
| 3. `standby init`, `standby status`, `standby promote`, marker file, epoch | Opus design, Sonnet build | `cmd/levelrail/standby*.go` (new), `internal/cpstandby/` (new), one migration in `internal/store/migrations` | `APP_CONTROL_PLANE_STANDBY_IDENTITY_FILE`, `APP_CONTROL_PLANE_STANDBY_MAX_LAG` |
| 4. Fence check on the primary and agent epoch guard | Opus | `internal/cpstandby/fence.go`, `internal/agent/client.go`, `proto/` session field | `APP_CONTROL_PLANE_FENCE_CHECK_INTERVAL`, `APP_CONTROL_PLANE_FENCE_GRACE` |
| 5. Doctor checks (agent address is an IP, standby lag, master key canary), dashboard card, CLI status | Sonnet | `internal/cphealth/`, `web/src/...` standby card, i18n | none new |
| 6. Two-container drill, then droplet drill, results file, docs | Haiku runs, Sonnet writes docs | `test/e2e/standby_*`, `scripts/qa/`, `docs/disaster-recovery.md`, `docs/resilience.md`, `docs/benchmark-results/` | none new |

Slices 3 and 4 touch the session protocol and secrets handling and must not
run as parallel agents. Slice 4 changes `proto/`, which is serialized with
any other transport work.

## Options rejected

- **Automatic failover now.** Needs a witness or lease to fence a
  partitioned primary; without one it creates split brain on every network
  blip. Later, with a written rule.
- **Agent address list.** Lets agents reach both machines during a partition
  and makes the split brain worse. A single DNS name plus the epoch guard is
  safer.
- **Snapshot replica (ADR 037) as the standby feed.** Works for small
  databases, but ship cost scales with size. Allowed as a fallback feed for
  `standby init` on tiny installs, not the default.
- **Postgres driver.** Rejected in ADR 039; nothing here changes that.
- **New CA on the standby with agents trusting two roots.** More moving
  parts than copying one CA and doubles the trust anchors on every node.

## Consequences

- RPO goes from the backup or replica interval to seconds, and RTO becomes
  a measured number driven mostly by DNS TTL and operator reaction time.
- The standby is a second full-secrets machine; the security cost is real
  and documented.
- Agents installed with an IP address need a one-time env change before a
  promote can help them.
- Walship finally gets wired, so the ADR 039 soak items (WAL growth with the
  sink down, CPU and RSS baseline) become blocking for slice 1, and
  `walship` and the snapshot replica must not run on the same database
  together.

## Not covered

- Automatic failover, health-based promotion, a witness or lease.
- Fencing a primary that is partitioned from the sink. Cooperative fencing
  only.
- More than one standby, or a standby that serves reads.
- Replication of `alerting.db` and the telemetry database.
- Zero RPO. Writes after the last shipped segment are lost.
- Moving public DNS for domains the control plane node serves, and ACME
  rate limits if a mass re-issue happens.
- Agents configured with an IP: they need manual reconfiguration.
- Revoking a compromised standby without re-enrolling every agent.
- Cross-version promote (standby on an older release than the stream).

## Open questions

1. Is Caddy's ACME storage fully in `levelrail.db`, or does any of it live
   on disk under the data directory? Slice 1 answers it.
2. Is the sink the same bucket as off-box backups (simpler) or a separate
   one (blast radius)? Proposed: same destination type, separate prefix,
   separate encryption key.
3. Does the agent epoch guard need a proto change, or can it ride in an
   existing session metadata header?
4. Should promote require the old primary's final segment to be older than
   `APP_CONTROL_PLANE_FENCE_GRACE` as an automatic sanity check on
   `--expect-primary-down`?
