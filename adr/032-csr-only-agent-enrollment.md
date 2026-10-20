# ADR 032: Agent keys are only ever generated on the agent

Status: accepted. Supersedes the compatibility note in ADR 021 decision 1.

## Context

ADR 021 had agents send a CSR, but kept a legacy path for the beta: an
enrollment with no CSR made the control plane generate the key and return it
(`APP_AGENT_REQUIRE_CSR=false`), and nodes enrolled that way were to switch
at their next renewal. Levelrail has no stable release, so the path only kept
a way for a private key to cross the wire.

## Decision

1. `Enroll` and `Reenroll` require `csr_der`. A request without one is
   refused with a static "upgrade the agent" error before the join token is
   redeemed (the token stays usable) and is recorded as a rejected security
   event, reason `agent_upgrade_required`.
2. `EnrollResponse.client_key_pem` is removed; field 3 and its name are
   `reserved`. `APP_AGENT_REQUIRE_CSR` and the control plane's client key
   generator are deleted. The one remaining key the control plane makes is
   its own listener key.
3. Migration 0970 moves nodes whose certificate key the control plane made
   (`cert_key_origin = 'server'`) to `reenroll_required`. Renewal for such a
   node is refused with a message asking the operator to re-enroll, and the
   node page, API and CLI say so. Re-enrollment installs an agent key and
   clears the mark. No runtime shim reads the old value.

## Rejected

- Renewing such nodes and swapping the key silently: the CSR would carry a
  new key, but the node would look healthy while still running a certificate
  whose key the control plane saw.
- Dropping the node at once: it keeps its session until the certificate
  expires, which gives the operator time to re-enroll.
