# 075. Capability allowlist for compose and app specs

Status: proposed

Date: 2026-11-08

## Context

Compose files and app specs refuse `cap_add` today (`internal/compose/security.go`, `internal/spec/runtime.go`). [UDP published ports](/udp-ports#not-yet) lists `cap_add` as not built, so a VPN server cannot run as an app, and tools that trace or reshape traffic cannot run either. ADR 074 kept `cap_add`, `privileged`, `devices` and `security_opt` refused and left a later decision open. ADR 049 sketched a grant for `NET_ADMIN` and `NET_RAW`. The socket proxy already has a narrow path for the platform's own sidecar (`dockerguard.GrantableCaps`), and the default hardening profile drops all capabilities.

## Decision

1. `privileged: true` and any `security_opt` other than `no-new-privileges` stay refused for every role, with an error naming the key.
2. `devices` and `device_cgroup_rules` stay refused. No device path is allowlisted.
3. `cap_add` accepts only names on a grantable list. The list ships as `NET_ADMIN` and `NET_RAW`, the same set as `dockerguard.GrantableCaps`, and is a runtime setting (applies live, env only seeds the default). The setting validator rejects any name the proxy does not grant, so the shipped default always passes it. A name outside the list is refused. `SYS_PTRACE` (as in ADR 049), `SYS_ADMIN`, `SYS_MODULE` and `ALL` can never be put on the list, and `dockerguard.GrantableCaps` does not change.
4. A grant is per app and per capability, not per host. It is written on the app, shown on the app page, in `app show` and in the API, and removed when the key leaves the spec.
5. Only a root user can approve a grant. App writers and tokens can request it by deploying a spec that contains it; the deploy then waits.
6. A grant is bound to the resolved image digest, always, with no setting to turn it off. Any image or digest change on an app holding a grant needs a new approval.
7. An app cannot hold an egress policy and a `NET_ADMIN` grant together, because the app can flush the sidecar's iptables rules in its own network namespace. The deploy is refused with `cap_grant_egress_conflict`. A host that cannot refuse reports egress as unenforced instead, never as enforced.
8. A minimum kernel version for any grant is read from a runtime setting. A node below it fails the deploy closed, and the docs state that `NET_ADMIN` trusts the image author with the kernel's netfilter code.
9. Grants are denied by default for imported compose files, docker run imports and PR previews. Previews inherit no grant, and an import that carries `cap_add` is refused with `cap_add_not_approved` until a root user approves it on the saved app.

## Guard rule names

Existing ids are never renamed. Existing: `privileged`, `devices_not_granted`, `device_cgroup_rules`, `security_opt_weakened`, `cap_add_disallowed`. New ids, added to `AllRules`: `cap_add_not_approved` (listed name, no approval record), `cap_add_not_listed` (name outside the list), `cap_grant_scope_mismatch` (create asks for more than the approved set or a different image digest), `cap_grant_egress_conflict` (grant and egress policy together). The compose and spec validators return the same ids so one doc table covers both.

## Audit event

`app.capability_grant` on approve, `app.capability_revoke` on removal or deploy without the key, and `app.capability_denied` on a refused request. Each row carries actor, app ID, capability names, approval ID and the guard rule id. No spec body, env values or secrets are logged.

## Approval flow

1. A deploy whose spec has a listed `cap_add` creates a capability approval record, as deploy approvals do (`internal/api/deploy_approvals.go`), and the deploy is held.
2. The requester cannot approve their own request. A different root user approves or rejects, in the dashboard, CLI or API, after seeing the capability names and a plain warning of what each one allows.
3. Approval is bound to the app ID and the exact capability set. Adding a name or changing the image digest needs a new approval. Rejection or expiry fails the deploy closed.
4. Agents advertise a capability for the field; an older agent fails the deploy instead of dropping the key.

## Cost

One small table of approval rows and a list lookup at spec validation. No new process, goroutine or per-request work.

## Test plan

Table test per validator (compose and spec): privileged, devices and `security_opt` refused; unlisted, `SYS_ADMIN` and `ALL` refused; listed name held without approval. Approval test: self-approval refused, non-root refused, scope mismatch and image change refused, egress policy with `NET_ADMIN` refused, imports and previews denied, revoke on removal. Audit test checks each event and that no spec content appears. Setting test rejects an ungrantable name. A real run on a host with `NET_ADMIN` (a UDP VPN container) is required before the status moves to accepted.

## Status of the change

This ADR records the decision only. Nothing is built, and the pages that say `cap_add` is rejected stay correct until the code lands.
