---
title: "Security advisories"
description: Every security issue Levelrail has found and fixed, with a stable ID, severity, affected versions, the fix, and how we disclose.
"og:url": "https://docs.levelrail.com/security-advisories"
---

This page lists security issues found in Levelrail and fixed, one row each, with a stable ID you can cite. We publish an entry when the fix has shipped, not before. Entries are never removed or renumbered.

For how to report something, see [SECURITY.md](https://github.com/glincker/levelrail/blob/main/.github/SECURITY.md). For what the platform defends and where it does not yet, see the [threat model](/threat-model).

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
| [LRSA-2026-0006](#lrsa-2026-0006) | High | Resource names in request bodies bypassed per-app, per-environment and per-node authorization | before 0.9 | next release | internal audit |
| [LRSA-2026-0007](#lrsa-2026-0007) | Medium | The dashboard assistant ran every tool with a permanent root token, and chat sessions had no owner | before 0.9 | next release | internal audit |
| [LRSA-2026-0008](#lrsa-2026-0008) | Low | Notification channel credentials were stored in plaintext | before 0.9 | next release | internal audit |
| [LRSA-2026-0009](#lrsa-2026-0009) | Low | Session-link tokens traveled in a GET URL path | before 0.9 | next release | internal audit |
| [LRSA-2026-0010](#lrsa-2026-0010) | Low | The OAuth redirect URI was derived from the Host header | before 0.9 | next release | internal audit |
| [LRSA-2026-0011](#lrsa-2026-0011) | Low | Infrastructure-as-code export could include secret-looking plain values | before 0.9 | next release | internal audit |
| [LRSA-2026-0012](#lrsa-2026-0012) | Low | Failed-signature webhook deliveries were stored without a cap | before 0.9 | next release | internal audit |
| [LRSA-2026-0013](#lrsa-2026-0013) | Medium | Upgrades fell back to a same-origin checksum when a release signature was missing | before 0.9 | next release | internal audit |

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

### LRSA-2026-0006

**Resource names in request bodies bypassed per-app, per-environment and per-node authorization.** High.

Routes that act on a resource named in the URL were authorized against that resource. Several routes instead took a resource name from the request body, a spec or a query string and only authorized the caller for the app in the URL. A caller with write access to one app could overwrite another team's service by name through a compose or spec deploy, point a git source at other apps so every push deployed into them, promote into an app they were denied, move workloads into another team's environment, or copy apps they were denied with environment clone.

**Fix.** One reference authorization helper now checks app, database, environment and node references with the same engine as path routes (IAM Deny, environment visibility, team ownership). A refused reference answers exactly like a missing one. A guard test fails when a handler decodes a resource-shaped field without authorizing it.

**Action for operators.** Upgrade. If you run several teams on one instance, review recent audit log entries for deploys and environment changes made by tokens with narrow IAM scopes.

### LRSA-2026-0007

**The dashboard assistant ran every tool with a permanent root token, and chat sessions had no owner.** Medium.

The assistant executed tool calls through one persistent token with the `root` ability, whoever was chatting. Text the assistant read from logs or commit messages could therefore drive actions with more authority than the person asking. Chat sessions were not tied to a user, so another root principal could read or continue them.

**Fix.** Each message and each approval now runs with a short-lived token owned by the person who sent it, carrying only their abilities and IAM policies, and revoked when the turn ends. Audit entries record who the action was for. Chat sessions belong to their creator; anyone else gets a not-found response. Sessions created before the upgrade have no owner and are no longer visible.

### LRSA-2026-0008

**Notification channel credentials were stored in plaintext.** Low.

Webhook URLs with embedded tokens, bot tokens and interactive signing secrets sat in plaintext in `alerting.db`, so a copy of that file disclosed them.

**Fix.** Channel credentials are envelope-encrypted with the master key and bound to their channel. API responses return a masked value for everyone. A startup migration encrypts existing rows and verifies by decrypting before clearing the plaintext.

### LRSA-2026-0009

**Session-link tokens traveled in a GET URL path.** Low.

A session link carried its token in the path, so it could appear in proxy and access logs and browser history before it was used.

**Fix.** The token is now redeemed with a POST body. The old GET returns `405` with a pointer to the new method.

### LRSA-2026-0010

**The OAuth redirect URI was derived from the Host header.** Low.

The redirect URI for provider sign-in came from the request's `Host` header and an unauthenticated `X-Forwarded-Proto`. Identity providers only honor registered redirect URIs, which limited the impact to misconfigured providers.

**Fix.** The redirect URI is built from the configured dashboard URL. Without one, forwarding headers are trusted only from loopback or `APP_INGRESS_TRUSTED_PROXIES`, and other callers get a clear `400`.

### LRSA-2026-0011

**Infrastructure-as-code export could include secret-looking plain values.** Low.

The export used a narrower secret heuristic than the rest of the product and defaulted to including environment values, so a plain variable that looked like a credential could be written into a shareable file by a caller with only `read`.

**Fix.** Export uses the same classifier as the environment read surface, and values are included only for callers holding `read:sensitive`.

### LRSA-2026-0012

**Failed-signature webhook deliveries were stored without a cap.** Low.

Deliveries rejected for a bad signature were stored in full with no per-app limit, so unauthenticated traffic could grow the database.

**Fix.** Unsigned deliveries are still recorded for debugging, capped to the newest 20 per app name, with bodies truncated to 2 KiB (both configurable).

### LRSA-2026-0013

**Upgrades fell back to a same-origin checksum when a release signature was missing.** Medium.

Release binaries are signed, but both the installer and the self-upgrade path accepted a release with no signature bundle, or a host that could not check it, and continued with only a checksum fetched from the same place as the binary. An attacker able to alter a release or its download path could strip the signature and still pass verification.

**Fix.** The first time a release signature verifies, the host is pinned. From then on a missing bundle, missing trust data or a downgrade to an unsigned release is refused with a message that names the explicit, logged override. Before the pin, the dashboard and `levelrail-cli doctor` show the host as "not signature verified" with the one-line fix.

**Action for operators.** Upgrade once so the host verifies a signature and becomes pinned. The server verifies signatures itself, so no extra tool is needed. Hosts that have never verified a signature keep the previous behavior until then.

### Hardening changes (no advisory)

These shipped alongside the fixes and are listed for completeness: agents must generate their own keys, the dashboard listens on loopback by default when fronted by HTTPS outside a container, the container list shows only managed containers to non-root callers, brand SVG is sanitized, bind mounts refuse executable directories and a custom data directory, `APP_MASTER_KEY_FILE` lets you keep the master key outside the data directory, and the Go toolchain and `golang.org/x/net` moved to releases that fix 13 reachable advisories.

## Disclosure policy

- **Reporting.** Use a private [GitHub security advisory](https://github.com/glincker/levelrail/security/advisories/new). Please do not open a public issue.
- **Acknowledgement.** We aim to acknowledge within three business days. There is no formal fix SLA yet; we say so rather than promise one.
- **Credit.** Reporters are credited here unless they ask not to be.
- **CVEs.** Issues that are remotely exploitable in a default install get a GitHub advisory and a CVE request. Hardening items and issues that need an already privileged account get an `LRSA` entry only.
- **No silent fixes.** A security fix always references its `LRSA` ID in the changelog entry.

## What this page is not

It is not a penetration-test report and it does not claim the absence of other issues. The 0.9 entries came from an internal, AI-assisted source review of the control plane, agent, installer and CI, followed by fixes with regression tests. Areas that review did not cover are listed in the [threat model](/threat-model). An independent review is planned before a stable release; its report will be linked here.

## Advisories about the images you run

Separate from the Levelrail advisories above, the [advisory feed](/advisory-feed) matches the images your own apps run against a feed you provide. The feed is verified only when `APP_ADVISORY_FEED_PUBKEY` is set; a local file without a key is shown as `unsigned`. `levelrail-cli security advisories match` lists each app, image digest, advisory, severity and fixed-in version, and `levelrail-cli security advisories refresh <app>` redeploys a matched app on the newest digest of its tag through the normal deploy approvals.
