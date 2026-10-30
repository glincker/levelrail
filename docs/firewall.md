---
title: "Firewall"
description: "Filter web requests at the embedded ingress with ordered rules, IP lists, automatic bans, a browser challenge, bot handling and attack mode."
"og:url": "https://docs.levelrail.com/firewall"
---

The firewall runs in the embedded ingress, in front of every host this control plane serves, before a request reaches an app. Rules apply the moment you save them: there is no redeploy and no ingress reload. Open **Firewall** in the sidebar, or use `levelrail-cli firewall ingress`.

This is separate from the **host firewall** under Settings, which manages ports on the server itself (`ufw`), and from the per-domain limits described in [Domain traffic controls](domain-traffic-controls#bot-and-abuse-shield), which now live in Firewall under Rules when a host is selected.

## The four views

| View | What it shows |
| --- | --- |
| Overview | State card, requests by outcome over time (allowed, denied, challenged, logged, rate limited), alerts, the busiest rules, temporary bans and denied IPs |
| Traffic | Top IP addresses, denied IPs, user agents, request paths and hosts for the last hour, with a row menu: block an IP, always allow it, or create a rule from the row |
| Rules | Custom rules, IP blocking, always-allow list, temporary bans, bot management, attack mode, and the per-host limits when a host is selected |
| Audit log | Every firewall change and who made it |

Pick a host in the scope selector to see one host, or leave it on All hosts. The time range applies to the Overview.

## Evaluation order

For every request, in this order:

1. The per-domain shield allowlist (`allow` on the domain's limits) skips the per-domain shield only; the firewall below still runs.
2. The challenge endpoint (`/.well-known/lr-challenge`) is handled by the firewall itself.
3. **Rules**, lowest position first, only those enabled and either global or scoped to this host. The first rule that decides ends the check:
   - `allow` (shown as Bypass): the request skips everything below, including bans and bot checks.
   - `deny`: 403.
   - `challenge`: the browser check, unless the client already holds a valid pass.
   - `rate_limit`: counts matches per client IP in a fixed window. Under the limit the request continues to the next rule; over it the answer is 429 with `Retry-After`.
   - `log`: counts the request as logged and continues.
4. **Temporary bans**: a banned address is denied.
5. **Attack mode** with challenge: every visitor without a pass gets the browser check.
6. **Bot lists**: bad bots, then AI crawlers, each set to off, log, challenge or block.
7. **Attack mode limits**: while attack mode is on, a per-client rate and connection limit applies to every host.
8. The per-domain shield (rate, connections, body size, domain bot list) runs last, on routes that have one.

Each request is classified exactly once as allowed, denied, challenged, logged or rate limited. Priority when several apply: denied, challenged, rate limited, logged, allowed.

## Rules

A rule has a name, an optional note, a scope (all hosts or one host), an enabled switch, an order, one or more conditions that must all match, and an action. For OR, add several rules.

| Field | Operators |
| --- | --- |
| Request path | equals, starts with, contains, matches pattern |
| Host | equals, contains, matches pattern |
| Method | equals |
| Header (by name) | equals, starts with, contains, matches pattern, exists |
| Query parameter (by name) | equals, starts with, contains, exists |
| User agent | equals, starts with, contains, matches pattern |
| IP address or range | is in |
| Country | equals (only offered when a GeoIP source is configured) |

A pattern uses `*` for any run of characters, including slashes, and `?` for one character, at most 8 wildcards per value. Paths and query values compare case sensitively; hosts, user agents and header values do not. Query parameters are compared as sent, without decoding. Rules are compiled when saved, so matching costs a few comparisons per rule and allocates nothing per request. At most `APP_FIREWALL_MAX_RULES` rules (default 500) can exist.

The **IP blocking** and **Always allow** lists are views over the same rules: a rule with a single IP condition and the deny or allow action. Allow entries are placed first and blocks right after them, so an allow always wins. Blocking an address that includes your own is refused unless you confirm it, because it would lock you out of this control plane.

Every rule shows how many requests it matched since the control plane started, with a one-hour sparkline.

The rule editor shows the rule exactly as stored under **View code**. The same checks run in the browser and on the server, so errors appear next to the field either way.

## Temporary bans (persistent actions)

A client that is denied or rate limited `APP_FIREWALL_BAN_STRIKES` times (default 20) within `APP_FIREWALL_BAN_WINDOW` (default 5m) is denied everywhere for `APP_FIREWALL_BAN_DURATION` (default 1h). A client that matches an allow rule is never banned. The list shows the address, why, how many requests were blocked, when it started and when it ends, with a Lift button. Turn the feature off with the Automatic bans switch under Bot management. Bans are held in memory only, capped by `APP_FIREWALL_MAX_BANS`, and end on restart.

## Browser challenge

A challenge serves a small page that needs no external service: JavaScript finds a counter so that the SHA-256 of a signed token and the counter has `APP_FIREWALL_CHALLENGE_BITS` leading zero bits (default 16, about a fraction of a second on a laptop), then posts the proof. The server checks the signature, expiry and proof and sets `__lr_pass`, an HttpOnly, SameSite=Lax cookie valid for `APP_FIREWALL_PASS_TTL` (default 30m).

- The token and the cookie are HMAC-SHA256 signed with a key derived from the control plane master key, bound to the client's network (the /24 for IPv4, the /64 for IPv6), and the token expires after `APP_FIREWALL_CHALLENGE_TTL` (default 2m). Without a master key a random per-process key is used and passes reset on restart.
- The client address comes from the same trusted-proxy handling as the rest of the ingress, so the challenge works behind a CDN or load balancer. The cookie is marked Secure when the request arrived over TLS or with `X-Forwarded-Proto: https`.
- The page sets a strict Content-Security-Policy (only its own script, same-origin posts), is marked noindex and no-store, uses system colors so it follows the visitor's light or dark setting, announces progress with `role="status"`, and shows a retry button when the check fails.
- A challenge answers 403 with the page. It suits browsers navigating by GET. API clients and non-browser agents cannot pass it, so give them a bypass rule or an Always allow entry.
- A challenge proves a browser ran some JavaScript. It does not verify that a crawler is who it says it is. Verified-bot detection by reverse DNS is not implemented.

## Bot management and AI crawlers

Each list has a mode: off, log only, challenge or block. The **bad bot** list is the curated list shipped with the shield (`internal/trafficpolicy/shield_bots.txt`, replaceable with `APP_SHIELD_BOT_LIST_FILE`). The **AI crawler** list is shipped as data (`internal/edgefw/ai_bots.txt`) and is editable under Edit list: one case-insensitive user agent substring per line, at most `APP_FIREWALL_MAX_AI_BOT_AGENTS` (default 256). An empty custom list means the shipped one is in use. Matching is by user agent text only.

## Attack mode

Danger zone, Attack mode: for a chosen duration (up to `APP_SHIELD_ATTACK_MAX_DURATION`) every host gets a low per-client request rate and connection cap (`APP_SHIELD_ATTACK_RATE_PER_MINUTE`, `APP_SHIELD_ATTACK_BURST`, `APP_SHIELD_ATTACK_MAX_CONCURRENT`) and the bad bot list is refused. Optionally every visitor must also pass the browser check. It ends on its own and can be turned off early. Per-domain under attack mode on the domain limits still works and is independent.

When denied, challenged and rate limited requests average `APP_FIREWALL_SUGGEST_ATTACK_PER_MINUTE` (default 120) or more per minute over `APP_FIREWALL_SUGGEST_ATTACK_MINUTES` (default 3) minutes and attack mode is off, the Overview shows an alert with a Turn on attack mode button, and an item appears in Attention for operators with the deploy ability. The test is a plain threshold, nothing else.

## What is stored, and for how long

| Data | Where | Kept |
| --- | --- | --- |
| Rules, IP list entries, bot modes, the AI crawler list, attack mode end time | SQLite (`edge_firewall_rules`, `edge_firewall_settings`) | Until you change or delete them |
| Rule changes, IP blocks and allows, ban lifts, bot setting changes, attack mode | Audit log, action names starting `firewall.` | Audit log retention |
| Outcome counts per minute, all hosts and per host (up to `APP_FIREWALL_MAX_STAT_HOSTS`, default 64) | Memory | 24 hours, lost on restart |
| Reason counts per hour | Memory | 24 hours, lost on restart |
| Rule match counts and an hourly sparkline | Memory | Since start (sparkline: 1 hour) |
| Top IPs, denied IPs, user agents (first 120 bytes), paths and hosts | Memory, approximate (space saving counters, `APP_FIREWALL_TOP_CAPACITY` keys per table per 10 minutes) | Last hour, lost on restart |
| Temporary bans and strike counters | Memory, capped | Until they end, lost on restart |
| Per-client rate counters | Memory, capped by `APP_FIREWALL_MAX_RATE_ENTRIES` per rule | One window |

Query strings are never stored. Paths in the top table are reduced to a route template with identifiers replaced, at most three segments deep. Client addresses exist only in the capped, time-bounded in-memory tables above and in the rules you create; they are never written to a log or to the database by the firewall. Set `APP_FIREWALL_TOP_SAMPLE` to N to feed the top tables from every Nth request on very busy instances. Requests the firewall denies never reach the per-app request statistics, which only see requests that were let through.

Reading client addresses (Traffic, Denied IPs, bans) needs the `read:sensitive` ability. Changing anything needs `deploy`. Reading counts, rules, settings and the audit view needs `read`.

## Command line

```bash
levelrail-cli firewall ingress status [--scope HOST] [--range 1h|6h|24h]
levelrail-cli firewall ingress rules list
levelrail-cli firewall ingress rules add --name "Block admin" --action deny --when "path starts_with /admin" --when "method equals POST"
levelrail-cli firewall ingress rules update <id> --disabled
levelrail-cli firewall ingress rules move <id> --to 1
levelrail-cli firewall ingress rules delete <id>
levelrail-cli firewall ingress block 203.0.113.0/24 --note scanner
levelrail-cli firewall ingress bypass 198.51.100.7
levelrail-cli firewall ingress bans list
levelrail-cli firewall ingress bans lift 203.0.113.9
levelrail-cli firewall ingress bots set --bots block --ai-bots challenge
levelrail-cli firewall ingress attack-mode on --minutes 60 --challenge
levelrail-cli firewall ingress traffic
levelrail-cli firewall ingress audit
```

A rate limit rule takes `--action rate_limit --rate 100 --window 60`. A condition is `FIELD OP VALUE`, with `header:NAME` or `query:NAME` for named fields.

## API

```
GET    /api/v1/firewall/edge/status?scope=&range=
GET    /api/v1/firewall/edge/series?scope=&range=
GET    /api/v1/firewall/edge/traffic?scope=
GET    /api/v1/firewall/edge/rules
POST   /api/v1/firewall/edge/rules
POST   /api/v1/firewall/edge/rules/order
PUT    /api/v1/firewall/edge/rules/{id}
DELETE /api/v1/firewall/edge/rules/{id}
POST   /api/v1/firewall/edge/block
POST   /api/v1/firewall/edge/allow
GET    /api/v1/firewall/edge/bans
DELETE /api/v1/firewall/edge/bans/{ip}
GET    /api/v1/firewall/edge/settings
PUT    /api/v1/firewall/edge/settings
POST   /api/v1/firewall/edge/attack-mode
DELETE /api/v1/firewall/edge/attack-mode
GET    /api/v1/firewall/edge/audit
```

Validation errors are `400` with `{"error": "...", "fields": [{"field": "conditions[0].value", "message": "..."}]}`. A rule that would block the caller's own address answers `409` with `"code": "self_lockout"`; repeat with `?force=true` to confirm. The app spec (`app.yaml`) does not carry firewall rules.

## Not yet

- **Managed rulesets** such as the OWASP Core Rule Set. See the Coraza notes below.
- **Countries and flags in the tables, ASN and network views, client fingerprints (JA3, JA4).** Country appears in rules only when a [GeoIP source](domain-traffic-controls#geolocation) is configured (`APP_GEOIP_DB` or a trusted country header). Tables list addresses only. Showing flags needs the same source on the server side of the table; networks and fingerprints need an ASN database and TLS fingerprinting in the ingress, which Levelrail does not ship.
- **Verified bots** (reverse DNS and published ranges). Matching is by user agent.
- **Hit counters and charts across restarts.** Everything in memory resets when the control plane restarts. A seven day view needs persisted aggregates.
- **Describe a rule in words.** The rule editor is complete; a box that drafts a rule from a sentence with the configured AI provider, limited to the same field and action catalog and never saved without your review, is planned. No AI is ever involved in handling a request.
- **Match preview** ("matches about N requests in the last hour") needs per-rule dry-run counters that are not collected yet.

### Could Coraza run as a Caddy module?

[Coraza](https://coraza.io) is a Go WAF engine that reads the OWASP Core Rule Set and has a Caddy module (`coraza-caddy`). Embedding it would be viable because the ingress already imports Caddy as a library, but the cost is real:

- **Binary and build**: one more module and its dependencies in the single static binary, and a pinned CRS version to ship and update.
- **Latency and memory**: CRS inspects headers and bodies with hundreds of regular expressions per request. Expect a measurable per-request cost and a body buffering limit, which interacts with streaming uploads and WebSockets.
- **False positives**: CRS needs per-app tuning (paranoia level, exclusions). Without a tuning workflow in the UI, a default install would block legitimate traffic. Detection-only mode with a review screen would have to ship first.
- **Fit**: the firewall's counters, row menus and audit trail would need a second source of events from the WAF engine.

The honest plan is a detection-only trial behind a feature flag, measured with `levelrail-cli bench`, before any claim. Nothing of this is implemented.

## Environment variables

| Variable | Default | Effect |
| --- | --- | --- |
| `APP_FIREWALL_MAX_RULES` | 500 | Most rules that can exist |
| `APP_FIREWALL_MAX_RATE_ENTRIES` | 10000 | Clients tracked per rate limit rule |
| `APP_FIREWALL_BAN_STRIKES` | 20 | Denials within the window that cause a ban |
| `APP_FIREWALL_BAN_WINDOW` | 5m | Window for counting denials |
| `APP_FIREWALL_BAN_DURATION` | 1h | How long a ban lasts |
| `APP_FIREWALL_MAX_BANS` | 10000 | Bans (and strike counters) kept |
| `APP_FIREWALL_CHALLENGE_BITS` | 16 | Proof of work difficulty, at most 28 |
| `APP_FIREWALL_CHALLENGE_TTL` | 2m | How long a challenge can be solved |
| `APP_FIREWALL_PASS_TTL` | 30m | How long a solved challenge lasts |
| `APP_FIREWALL_TOP_CAPACITY` | 20 | Keys kept per top table per 10 minutes |
| `APP_FIREWALL_MAX_STAT_HOSTS` | 64 | Hosts that get their own counters |
| `APP_FIREWALL_TOP_SAMPLE` | 1 | Feed the top tables from every Nth request |
| `APP_FIREWALL_SUGGEST_ATTACK_PER_MINUTE` | 120 | Blocked requests per minute that suggest attack mode |
| `APP_FIREWALL_SUGGEST_ATTACK_MINUTES` | 3 | Averaging window for the suggestion |
| `APP_FIREWALL_MAX_AI_BOT_AGENTS` | 256 | Entries in the custom AI crawler list |
