---
description: How Levelrail handles secrets, sessions, TLS, and access control, and where to find the detailed page for each.
---

# Security overview

This page is a map, not a duplicate. Each topic below has its own detailed page; this one explains how the pieces fit together and links out.

## Secrets: envelope encryption

Every secret (an app env var marked `secret: true`, email credentials, API tokens, and so on) gets its own random data encryption key (DEK). Every DEK is wrapped under one master key held in memory by the control plane, never written to disk in plaintext.

::: tip
Rotating the master key re-wraps every DEK without ever exposing plaintext. See [Master key rotation](master-key-rotation.md) for the full procedure and failure modes.
:::

Credentials for backup targets and registry integrations follow the same write-only pattern described in [Backups and storage](backups-and-storage.md#credentials-are-write-only): once saved, the plaintext is never returned by the API again, only a masked placeholder.

## Sessions and tokens

- **Session cookies** are set `Secure`, which means they only round-trip over HTTPS. Embedded Caddy is expected to terminate TLS in front of the control plane; hitting it directly over plain HTTP (common in local dev) means the cookie never comes back on the next request.
- **API tokens** are minted per-user, scoped by ability, and can be issued through a device-code flow for headless environments.
- **Two-factor authentication (TOTP)** is available per user, with recovery codes for account lockout.

Full detail on all three: [Identity and access](identity-and-access.md#principals-a-session-or-a-token).

## Authorization: abilities, roles, and IAM policies

Levelrail layers two permission models:

- **Abilities**: a flat list (`AbilityRead`, `AbilityWrite`, `AbilityDeploy`, `AbilityRoot`, and so on) attached to a user or token. Cheap to check, coarse-grained.
- **IAM policies**: AWS-IAM-shaped Allow/Deny documents scoped to a specific app or database, for when a flat ability isn't precise enough.

Evaluation order, the full ability list, and policy examples: [Identity and access](identity-and-access.md#why-two-permission-models-instead-of-one).

## TLS and network exposure

- Certificates are issued and renewed automatically through embedded Caddy's ACME client. See the [ACME verification runbook](acme-verification-runbook.md) if issuance fails.
- HSTS (`Strict-Transport-Security`) is opt-in, not on by default, because turning it on for a domain that later loses TLS locks users out until the header expires. See [Domains and ingress](domains-and-ingress.md#tls).
- WAF mode and rate limiting are configurable per domain, with a detect-only mode for testing rules before enforcing them. See [Domains and ingress](domains-and-ingress.md#waf-and-rate-limiting).
- The node agent dials **out** to the control plane. No inbound ports need to be open on a managed server for enrollment or day-to-day operation.

## Fresh-box hardening checklist

A short list for the server itself, independent of anything Levelrail configures:

- **SSH key auth only.** Disable password login (`PasswordAuthentication no` in `sshd_config`) before exposing the box to the internet. Neither `install.sh` nor the agent touch SSH configuration; this is standard server hygiene, not something Levelrail does for you.
- **Only the control-plane node needs inbound ports.** `80/tcp` and `443/tcp` for ingress (the ACME HTTP-01 challenge plus HTTPS traffic), your SSH port, and nothing else. See [Installing: requirements](installing.md#requirements) for the full list and `install.sh`'s optional `ufw` setup.
- **Agent-only nodes should accept no inbound traffic at all.** The node agent dials out to the control plane over mTLS; the control plane never initiates a connection (see the root `CLAUDE.md` section 4.3). A server running only the agent has nothing that needs to accept a connection, so leave its firewall closed by default rather than opening anything speculatively.
- **Don't expose anything Levelrail didn't ask you to.** Docker's daemon socket, the SQLite database file, and the mesh DNS listener (`:5390` by default, see [Multi-node](multi-node.md#wireguard-mesh-and-internal-dns)) are all meant to stay local to the box.
- **Keep the OS patched.** The `patch_status` alert (see [Observability](observability.md)) can warn you about pending security patches on a managed node, but it only reports, it doesn't apply anything; that's still on you (`unattended-upgrades`, `dnf-automatic`, or your distro's equivalent).

## Audit log

Every mutating API call from an authenticated principal is recorded: who, what, when, and the outcome. The log is queryable and exportable as CSV. See [Identity and access](identity-and-access.md#audit-log).

## Reporting a vulnerability

Levelrail does not yet have a dedicated security disclosure address. Until one exists, open a private security advisory on the [GitHub repository](https://github.com/glincker/levelrail/security/advisories/new) rather than a public issue. The repository's [SECURITY.md](../SECURITY.md) has the full policy, including what to expect after reporting.

## What this does not cover yet

- No SSO/SAML, only local password auth and OAuth sign-in.
- No secret scanning of an app's own source repository.
- No container-escape hardening beyond what Docker itself provides; rootless Docker and Podman support are open questions (see the root `CLAUDE.md`'s open decisions).

## See also

- [Master key rotation](master-key-rotation.md) - How to rotate the encryption key that protects all secrets
- [Identity and access](identity-and-access.md) - Users, tokens, roles, and IAM policies
- [Domains and ingress](domains-and-ingress.md) - TLS certificates and domain configuration
- [Architecture](architecture.md) - How security layers integrate with the core platform
