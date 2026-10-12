---
title: "Security advisories"
description: Every security issue Levelrail has found and fixed, with a stable ID, severity, affected versions, the fix, and how we disclose.
---

This page lists security issues found in Levelrail and fixed, one row each, with a stable ID you can cite. We publish an entry when the fix has shipped, not before. Entries are never removed or renumbered.

For how to report something, see [SECURITY.md](https://github.com/glincker/levelrail/blob/main/SECURITY.md). For what the platform defends and where it does not yet, see the [threat model](/threat-model).

## How to read an entry

- **ID**: `LRSA-<year>-<number>`. Stable. Referenced from the changelog and from the fixing commit.
- **Severity**: Critical, High, Medium, Low. We rate by what an attacker gets in a default install, not by the weakness class.
- **Affected**: the versions that contain the issue.
- **Fixed in**: the first release with the fix. A row marked `next release` is merged and will carry its version when it ships.
- **Found by**: how it was discovered. We say so plainly, including when it was our own review.

## Found and fixed

| ID | Severity | Summary | Affected | Fixed in | Found by |
| --- | --- | --- | --- | --- | --- |
| [LRSA-2026-0001](#lrsa-2026-0001) | High | An app writer could attach any database to their app | before 0.9 | next release | internal audit |
| [LRSA-2026-0002](#lrsa-2026-0002) | High | One visitor could exhaust login and webhook rate limits for everyone | before 0.9 | next release | internal audit |
| [LRSA-2026-0003](#lrsa-2026-0003) | Medium | Unauthenticated endpoints accepted unbounded request bodies | before 0.9 | next release | internal audit |
| [LRSA-2026-0004](#lrsa-2026-0004) | Medium | Assistant replies could inject HTML attributes through link targets | before 0.9 | next release | internal audit |
| [LRSA-2026-0005](#lrsa-2026-0005) | Low | Client-supplied request IDs were echoed and logged without bounds | before 0.9 | next release | internal audit |

### LRSA-2026-0001

**An app writer could attach any database to their app.** High.

The route that connects a database to an app checked the caller's right to change the app, but not their right to see the database. A user or token with write access to one app, including one explicitly denied a database by an IAM policy or in another team, could attach that database and receive its connection details in their own container. The same gap existed through `app.yaml` `from:` references on a deploy and when saving a git source.

**Fix.** Every database named in a request body or spec is now authorized with the same engine as a `/databases/{name}` route (IAM Deny, environment visibility, team ownership). A refused database answers like an unknown database, so the response does not reveal that it exists. Tests cover the attach, set-database and deploy-spec paths.

**Action for operators.** Upgrade. Review which apps are attached to which databases (`GET /api/v1/apps/{name}/connections`) if you run several teams on one instance.

### LRSA-2026-0002

**One visitor could exhaust login and webhook rate limits for everyone.** High.

Behind the built-in ingress, every request reached the control plane from `127.0.0.1`, and rate limits keyed on that address. Per-visitor budgets therefore became one shared budget. An anonymous client could keep an operator's account locked out of password sign-in, use up the invite and password-reset budget, or cause Git providers' webhook deliveries to be refused with `429`.

**Fix.** The real client address is taken from `X-Forwarded-For`, only when the connection comes from loopback or an address listed in `APP_INGRESS_TRUSTED_PROXIES`, using the right-most untrusted hop. Other peers cannot spoof it.

**Action for operators.** Upgrade. If you put another reverse proxy in front of the ingress, add its address to `APP_INGRESS_TRUSTED_PROXIES`.

### LRSA-2026-0003

**Unauthenticated endpoints accepted unbounded request bodies.** Medium.

Sign-in, registration and second-factor endpoints decoded JSON without a size cap. A large body could exhaust memory on the control plane, which also signs every user out when it restarts.

**Fix.** A 1 MiB cap on routes reachable without a session and a 16 MiB backstop on every other API request (`APP_API_MAX_BODY_BYTES` adjusts the backstop). Handlers that already set a smaller limit keep it.

### LRSA-2026-0004

**Assistant replies could inject HTML attributes through link targets.** Medium.

The dashboard renders the assistant's markdown replies. Link targets were placed into an `href` attribute without escaping, so text echoed from logs, commit messages or pull request titles could add attributes to the link. The Content Security Policy blocked script execution, which limited the impact to beaconing and on-page overlays.

**Fix.** Link targets are attribute-escaped, and non-`http(s)`/`mailto` schemes render as plain text. A regression test fails on the previous behavior.

### LRSA-2026-0005

**Client-supplied request IDs were echoed and logged without bounds.** Low.

An inbound `X-Request-Id` of any length was returned in the response and written to logs, which allowed log bloat and confusing correlation.

**Fix.** Only `[A-Za-z0-9_.-]` up to 128 characters is accepted. Anything else is replaced with a generated ID.

## Disclosure policy

- **Reporting.** Use a private [GitHub security advisory](https://github.com/glincker/levelrail/security/advisories/new). Please do not open a public issue.
- **Acknowledgement.** We aim to acknowledge within three business days. There is no formal fix SLA yet; we say so rather than promise one.
- **Credit.** Reporters are credited here unless they ask not to be.
- **CVEs.** Issues that are remotely exploitable in a default install get a GitHub advisory and a CVE request. Hardening items and issues that need an already privileged account get an `LRSA` entry only.
- **No silent fixes.** A security fix always references its `LRSA` ID in the changelog entry.

## What this page is not

It is not a penetration-test report and it does not claim the absence of other issues. The 0.9 entries came from an internal, AI-assisted source review of the control plane, agent, installer and CI, followed by fixes with regression tests. Areas that review did not cover are listed in the [threat model](/threat-model). An independent review is planned before a stable release; its report will be linked here.
