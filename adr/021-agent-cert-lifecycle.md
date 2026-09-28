# ADR 021: Agent certificate lifecycle

Status: Proposed
Date: 2026-09-25

## Context

ADR 003 promised "certificate rotation on a schedule" but only the expiry
was built: agents got a 90 day client certificate at enrollment and nothing
renewed it, so every remote node would stop connecting about 90 days after
it enrolled, all at once for nodes enrolled together. The control plane
also generated each agent's private key and returned it in
`EnrollResponse`, a `Session` accepted exactly one certificate fingerprint
per node with no overlap or revoked state, and nothing recorded which agent
build a node runs.

## Decisions

1. **The agent generates its key and sends a CSR.** `EnrollRequest` gains
   `csr_der`; the control plane signs it and never sees the key. The CSR's
   subject, SANs and extensions are ignored: the control plane alone sets
   CN (the node ID) and client-auth usage. ed25519 (what the CA already
   uses) and ECDSA P-256/P-384 are accepted, RSA is not.
   *Compatibility:* an old agent that sends no CSR still gets a
   server-generated key, and a new agent talking to an old control plane
   uses the key the response carries. `APP_AGENT_REQUIRE_CSR=true` closes
   the legacy path once every agent is upgraded. Nodes enrolled before this
   move to an agent-generated key at their first renewal
   (`cert_key_origin` records which).
   *Rejected:* a clean break (strands agents mid-upgrade for no security
   gain the opt-in flag does not also give); keeping server key generation
   (the key crosses the wire and sits in control plane memory).

2. **Renewal is a unary `Renew` RPC authenticated by the current
   certificate**, called on the same connection as the Session. The agent
   renews at two thirds of the lifetime plus up to 5% jitter
   (`APP_AGENT_CERT_RENEW_FRACTION` on the agent), with a fresh key every
   time, retrying 1m doubling to 1h.
   *Rejected:* a Session stream message (the mux only correlates requests
   the control plane starts; a unary call is simpler and independently
   testable); a server-chosen `renew_after` in the response (one more
   persisted field for no current need); renewal after expiry (the TLS
   handshake already rejects expired certificates, and step-ca documents it
   as widening risk; re-enrollment is the recovery path instead).

3. **Overlap, not lockout.** Renewal moves the presented certificate to
   `prev_cert_fingerprint`, accepted until `prev_cert_valid_until`
   (`APP_AGENT_CERT_RENEW_GRACE`, default 24h). If the response is lost or
   the control plane restarts after committing, the agent still holds a
   certificate that works and simply renews again. A retry presenting the
   already-previous certificate keeps the original window instead of
   extending it, and the store update is a compare-and-swap on the current
   fingerprint so concurrent renewals cannot both win. The CSR is signed
   before the store is updated; an issued certificate that never got
   recorded is never trusted.
   *Rejected:* accepting every past certificate until its own expiry (makes
   rotation meaningless); no overlap (one lost response strands the node).

4. **Agent-side persistence is atomic and tested before use.** The new
   identity is written to a temp file, fsynced, renamed over the identity
   file (directory fsynced), with the old one kept as `<file>.prev`. A
   separate connection calls `CheckIdentity`; only if the control plane
   reports the new certificate as current does the agent switch its
   in-memory identity (used by the next connection, no restart) and drop
   the backup. Otherwise it restores the backup. An agent stopped between
   those steps resolves the staged state on its next start. The on-disk
   JSON format is unchanged.

5. **Recovery is re-enrollment with a token bound to the node.**
   `node_join_tokens` gains `purpose` (`enroll` or `reenroll`) and
   `node_id`. `POST /api/v1/nodes/{id}/reenroll-token` mints a single-use,
   15 minute token; the agent's `reenroll` subcommand exchanges it through
   a separate `Reenroll` RPC with a fresh CSR, verifying the control plane
   against the CA in its old identity file (or the pinned fingerprint).
   The node keeps its ID, placements and history, the previous certificate
   is dropped, and a revocation is cleared because minting the token was
   the operator's authorization. A running agent adopts the new identity
   file at its next reconnect.
   *Rejected:* letting `Enroll` take a node ID (an enroll token could then
   take over an existing node); a long-lived per-node recovery secret on
   disk (a second credential to protect with the same exposure as the key).
   No rate limit is added: tokens are 32 random bytes and single use.

6. **Revocation is server-side state.** `cert_revoked_at` makes Session,
   Renew and CheckIdentity refuse the node, and `Server.Disconnect` closes
   its live session. No CRL or OCSP: the control plane is the only relying
   party, so a row check is enough for a fleet this size.

7. **Expiry is visible everywhere from one classifier.**
   `alerting.ClassifyNodeCert` (ok, expiring, critical, expired, revoked,
   unknown) drives the node API, the new platform-wide `node_cert_expiring`
   alert kind, the attention list, the dashboard badge and the CLI, with
   `APP_NODE_CERT_EXPIRY_WARNING` (default 21 days, deliberately below the
   ~30 days left at a healthy renewal) and `APP_NODE_CERT_EXPIRY_CRITICAL`
   (7 days). Revoked nodes do not alert. Existing nodes are backfilled
   with created_at plus 90 days, replaced by the real `NotAfter` the next
   time they connect.

8. **Agents report version, commit, OS and arch in a `Hello` frame**, sent
   once as the first Session message (a new `AgentMessage` oneof field that
   older control planes ignore). `APP_AGENT_MIN_VERSION` flags older agents,
   and agents that never reported, as outdated (badge, CLI, attention).
   *Rejected:* fields on every Heartbeat (repeats static data every 15s);
   exact match with the control plane version (Komodo's approach, which
   alerts on every patch skew). This is the prerequisite for self-update,
   which is not built.

9. The two new node routes use `requireAbilityForResource` with
   `node:{id}`, so a per-node IAM Deny applies to them.

## Consequences

- An agent offline longer than its certificate's remaining life needs an
  operator to mint a re-enroll token. That is intended (Teleport and
  Tailscale make the same call) but it is a manual step.
- CA rotation is still an operational action that re-enrolls every node.
  `RenewResponse.ca_cert_pem` is where a future CA bundle would be
  delivered without a flag day.
- Nodes that stay offline across the upgrade show an estimated expiry
  until they reconnect.
- Everything here was tested in process (real gRPC, real TLS, an in-process
  CA, the real SQLite store), not against a real remote node.
