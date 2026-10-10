---
description: How the dashboard surfaces things that wait on an operator, including CLI login approvals, deploy approvals, stalled certificate renewals and nodes that never connected.
---

# Attention center

Some states block on a person and used to be visible only if you went looking. The attention center puts them in one place: the bell in the dashboard header, the Status page, `levelrail-cli attention`, and the MCP `get_attention` tool.

## What shows up

| Item | Where it comes from | What to do |
| --- | --- | --- |
| CLI login waiting | `levelrail-cli auth login --device` | Compare the code, then approve or deny |
| Deploy approval pending | A deploy into a protected environment | Open Approvals |
| Certificate renewal stalled | A certificate that should have renewed by now | Open Domains, read the CA error |
| Node joined but never connected | An enrolled node whose agent never reported in | Check the agent logs and network path |

Existing items (failing apps, failed deploys, offline nodes, disk pressure, expiring certificates, doctor warnings, updates) appear in the same list. Items that wait on a person sort ahead of other warnings, and critical items always come first.

## Approving a CLI login

A pending login raises a banner on every page of the dashboard with the code, where the request came from, the device name, and a countdown. The banner cannot be dismissed. It goes away when the login is approved, denied, or expires (10 minutes).

1. Look at the terminal that ran `levelrail-cli auth login --device`.
2. Click **Review and approve**. The code is shown large.
3. Click **The codes match, approve** only if it is exactly the code in your terminal.

Approval is never automatic and never a single click. If the requesting IP address differs from the IP your browser session uses, the banner and dialog show a warning. Only approve a login you started.

Only a person signed in to the dashboard can approve or deny. API tokens, the CLI and the MCP server cannot, so an AI agent that starts a login has to ask you to approve it.

## Running the CLI through a tunnel or proxy

The link printed by `auth login --device` is built from the address the CLI actually reached, so it works behind an SSH tunnel, a reverse proxy, or a custom port. A few rules apply.

- `APP_DASHBOARD_URL`, when set, always wins.
- Otherwise the request's `Host` header is used.
- `X-Forwarded-Host` is honored only when the request arrives from a loopback address or one listed in `APP_INGRESS_TRUSTED_PROXIES`.

## For AI agents

`levelrail-cli auth login --device --json` prints one JSON line to stdout before it blocks:

```json
{"event":"device_login_pending","verification_url":"http://127.0.0.1:28080/settings/cli-access?user_code=ABCD-1234","user_code":"ABCD-1234","expires_in":600,"expires_at":"2026-10-09T12:10:00Z"}
```

Relay `verification_url` and `user_code` to the person. The final token resource follows on stdout once they approve.

## Get notified outside the dashboard

A channel can opt in to a notice when a CLI login is waiting. It is off by default.

```
levelrail-cli channels update <id> --name NAME --kind slack --notify-url URL --notify-device-login
```

You can also toggle **Notify when a CLI login is waiting** when editing a channel in the dashboard. The notice contains a link only, never the code. The link needs `APP_DASHBOARD_URL` (or `APP_PUBLIC_HOST`) to be set, and at most one notice is sent per minute.

## Security notes

- Starting a login is unauthenticated and rate limited per IP (`APP_DEVICE_START_RATE_PER_MINUTE`, default 6, `0` disables).
- Every approve and deny, and every login that expires unapproved, is written to the audit log.
- `GET /api/v1/auth/device/pending-summary` is a read-only view for tools. It never includes a code.
