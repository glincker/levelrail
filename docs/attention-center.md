---
description: How the dashboard surfaces things that wait on an operator, including CLI login approvals, expired logins, deploy approvals, expiring tokens, failed data copies, overdue backups and stalled certificate renewals.
---

# Attention center

Some states block on a person and used to be visible only if you went looking. The attention center puts them in one place: the banner and bell in the dashboard header, the Status page, `levelrail-cli attention`, and the MCP `get_attention` tool.

## What shows up

| Item | Where it comes from | What to do |
| --- | --- | --- |
| CLI login waiting | `levelrail-cli auth login --device` | Compare the code, then approve or deny |
| CLI login expired or denied | A login nobody approved in time, or that was denied | Start over from the terminal, or dismiss |
| Deploy approval pending | A deploy into a protected environment | Open Approvals |
| API token expiring or expired | A token with an expiry inside the window | Create a replacement and revoke the old one |
| Data copy failed or stalled | A live data copy into a managed database | Open the database and retry |
| Backup overdue | A scheduled backup later than its schedule allows | Open the database and run a backup |
| Invitations waiting | Pending or expired, unaccepted invitations | Open Users and resend or revoke |
| Certificate renewal stalled | A certificate that should have renewed by now, with the CA's last error | Open Domains, read the CA error |
| Node joined but never connected | An enrolled node whose agent never reported in | Check the agent logs and network path |

Existing items (failing apps, failed deploys, offline nodes, disk pressure, expiring certificates, doctor warnings, updates) appear in the same list. Items that wait on a person sort ahead of other warnings, critical items always come first, and informational items (resolved logins) come last.

Every item has a stable id, a severity (`critical`, `warning` or `info`), a one-line title, a next action and a dashboard link. A token only receives the items its abilities allow, so a read-only token gets a shorter list instead of an error.

## Approving a CLI login

A pending login raises a banner on every page of the dashboard with the code, where the request came from, the device name, and a countdown. The banner cannot be dismissed. It goes away when the login is approved, denied, or expires (10 minutes by default, `APP_DEVICE_CODE_TTL`).

1. Look at the terminal that ran `levelrail-cli auth login --device`.
2. Click **Review and approve**. The code is shown large.
3. Click **The codes match, approve** only if it is exactly the code in your terminal.

Approval is never automatic and never a single click. If the requesting IP address differs from the IP your browser session uses, the banner and dialog show a warning. Only approve a login you started.

Only a person signed in to the dashboard can approve or deny. API tokens, the CLI and the MCP server cannot, so an AI agent that starts a login has to ask you to approve it.

## Expired and denied logins

When a login expires or is denied it stays visible for 24 hours (`APP_ATTENTION_RESOLVED_WINDOW`) as an informational strip under the banner, in the bell and on the Status page. Each shows its state (waiting, expired, denied or approved), the requesting IP, the device name and how long ago it happened.

- **Start over** shows the command to run in your terminal to request a new login.
- **View audit entries** opens the audit log filtered to that one request.
- The cross **dismisses** the item for you only. Dismissal is stored on the server, so it survives a reload and applies on your other devices. It never edits the audit log, and a new request is a new item that appears again.

A waiting login cannot be dismissed. Approved logins need no action and appear only in the audit log.

## Audit trail

Every state change writes exactly one audit entry with a stable action name, shown under **Settings > Audit log** with the **Device login** filter (`?action=device_login`):

| Action | Written when |
| --- | --- |
| `device_login.approved` | A signed-in operator approves |
| `device_login.denied` | A signed-in operator denies |
| `device_login.expired` | A pending request passes its time limit (actor `system`) |
| `device_login.dismissed` | A user dismisses an expired or denied item |

A background sweeper records expiry whether or not the CLI ever polls again. It is idempotent and safe across restarts: an expiry is claimed once in the same transaction that writes its audit entry. Entries carry the request id, never the code. Set the sweep interval with `APP_DEVICE_EXPIRY_SWEEP_INTERVAL` (default 30s) and how long resolved requests are kept with `APP_DEVICE_HISTORY_RETENTION` (default 30 days; audit entries follow the audit log retention instead).

## Sign-in codes and new browsers

When someone asks for a sign-in code for your account, or signs in with your password from a browser you have not trusted while you are signed in here, a banner appears on every page and the attention center lists one `login_code` or `login_approval` item for your account, with a count and the newest requester's IP address, browser and time. Items and the banner never carry the code: **Show code** fetches it on demand, and each reveal is audited. Approve or deny a new browser from the banner or from **Settings > Security**; approving asks you to type the number the waiting browser shows. The same requests are available from the terminal with `levelrail-cli auth code`. See [Identity and access](identity-and-access.md#sign-in-with-a-code).

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

A channel can opt in to a notice when a CLI login is waiting, and separately when a pending login expires. Both are off by default.

```
levelrail-cli channels update <id> --name NAME --kind slack --notify-url URL --notify-device-login --notify-device-login-expired
```

You can also toggle both when editing a channel in the dashboard. A notice contains a link only, never the code. The link needs `APP_DASHBOARD_URL` (or `APP_PUBLIC_HOST`) to be set, and at most one notice of each kind is sent per minute.

## Tuning

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_DEVICE_CODE_TTL` | `10m` | How long a login request stays approvable |
| `APP_DEVICE_EXPIRY_SWEEP_INTERVAL` | `30s` | How often expiry is recorded |
| `APP_ATTENTION_RESOLVED_WINDOW` | `24h` | How long expired and denied logins stay visible |
| `APP_DEVICE_HISTORY_RETENTION` | `720h` | How long resolved requests are kept |
| `APP_ATTENTION_TOKEN_WINDOW` | `168h` | How far ahead token expiry is flagged, and how long after |
| `APP_ATTENTION_BACKUP_GRACE` | `6h` | Slack past a backup's expected interval before it is overdue |

## Security notes

- Starting a login is unauthenticated and rate limited per IP (`APP_DEVICE_START_RATE_PER_MINUTE`, default 6, `0` disables).
- Approve, deny and dismiss are session-only and pass the cross-origin check. The read endpoints (`/api/v1/auth/device/activity`, `/api/v1/attention/feed`, `/api/v1/auth/device/pending-summary`) never include a code.
- Codes never appear in logs, audit entries, notices or MCP output. The MCP server stays read-only and has no approve, deny or dismiss tool.
