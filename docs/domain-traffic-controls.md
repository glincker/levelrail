---
description: Per-domain traffic controls in the embedded ingress, covering header rules, path forwarders, country rules, response caching, redirects and raw ports, from the dashboard, CLI or API.
---

# Domain traffic controls

Every domain an app owns has its own set of traffic controls, enforced by the embedded Caddy ingress before a request reaches the container:

| Control | What it does |
| --- | --- |
| [Headers](#headers) | Set, add or remove request and response headers, plus security, CORS and "hide server" presets |
| [Forwarders](#path-forwarders) | Send matching paths to another app, an external URL, or a redirect |
| [Geo](#geolocation) | Allow or deny visitors by country |
| [Cache](#caching) | Keep responses in memory and answer repeat requests from the ingress |
| [Redirects](#redirects) | Force HTTPS, www and apex setup, aliases, trailing slash and lower case host |
| [Ports](#ports) | See the app's raw TCP/UDP ports and open one only to chosen addresses |

None of these needs a second proxy or a container restart. A change is stored, audited, and applied on the next ingress reconcile pass (normally within a second). Deleting a section restores the default behavior for that domain.

## Where to find it

<Tabs :items="['Dashboard', 'CLI', 'API']">
<Tab value="Dashboard">

On the **Domains** page, click a domain name. The domain page has tabs for Overview, Headers, Forwarding, Access, Cache, Redirects, Ports and Danger zone. Each editor keeps an unsaved draft per tab, shows the difference before you save, and a "rules preview" lists in order what happens to a sample request.

</Tab>
<Tab value="CLI">

```bash
levelrail-cli domains headers <verb> <app> <domain>
levelrail-cli domains forwarders <verb> <app> <domain>
levelrail-cli domains geo <verb> <app> <domain>
levelrail-cli domains cache <verb> <app> <domain>
levelrail-cli domains redirects <verb> <app> <domain>
levelrail-cli domains ports <verb> <app> <domain>
```

Run any of them with `-h` for its verbs and flags. Every command accepts `--json` and `-o`.

</Tab>
<Tab value="API">

```text
GET    /api/v1/apps/{name}/domains/{domain}/policies           every section plus status, in one call
POST   /api/v1/apps/{name}/domains/{domain}/policies/preview   walk a request through draft rules
GET/PUT/DELETE /api/v1/apps/{name}/domains/{domain}/headers
GET/PUT/DELETE /api/v1/apps/{name}/domains/{domain}/forwarders
GET/PUT/DELETE /api/v1/apps/{name}/domains/{domain}/geo
GET/PUT/DELETE /api/v1/apps/{name}/domains/{domain}/cache
POST   /api/v1/apps/{name}/domains/{domain}/cache/purge
GET    /api/v1/apps/{name}/domains/{domain}/cache/stats
GET/PUT/DELETE /api/v1/apps/{name}/domains/{domain}/redirects
POST   /api/v1/apps/{name}/domains/{domain}/redirects/canonical
PUT    /api/v1/apps/{name}/domains/{domain}/redirects/aliases
GET    /api/v1/apps/{name}/domains/{domain}/ports
PUT/DELETE /api/v1/apps/{name}/domains/{domain}/ports/{port}/restrict
GET    /api/v1/system/geoip?ip=
```

A rejected change returns `400` with a `fields` list (`{"field": "rules[2].name", "message": "..."}`) so a client can show the error next to the input.

</Tab>
</Tabs>

## Order of evaluation

For a request to a domain with controls configured, the ingress runs, in order:

1. Redirects (lower case host, force HTTPS, trailing slash)
2. Geo rule
3. Headers: request rules, CORS, then response rules and presets
4. The existing WAF, rate limit and basic auth, if configured
5. Cache lookup
6. Forwarders, first match wins
7. The domain's own app

Maintenance mode and a whole-domain redirect still take precedence over everything above, exactly as before.

## Headers

Rules apply in the order you list them. Each rule has a side (`request` or `response`), an operation (`set`, `add` or `remove`), a name and, except for `remove`, a value.

- Request rules change what the app receives.
- Response rules apply after the app answered, so they win over the app's own value. A later rule wins over an earlier one, and any rule wins over a preset.
- Header names must be valid RFC 9110 tokens. Values cannot contain line breaks or control characters, and are capped by `APP_DOMAIN_POLICY_MAX_HEADER_VALUE_LEN`. Operator text is never expanded as a Caddy placeholder, so a value like `{env.SECRET}` is sent literally.
- These names are managed by the ingress and refused: `Host`, `Content-Length`, `Transfer-Encoding`, `Connection`, `Keep-Alive`, `Upgrade`, `TE`, `Trailer`, `Proxy-Connection`, `Forwarded`, `X-Real-IP`, `Proxy-Authorization` and every `X-Forwarded-*` header. `X-Forwarded-Prefix` has its own option, `forwarded_prefix`. `X-Cache` and `Cache-Status` are reserved for the cache.

### Presets

| Preset | Adds |
| --- | --- |
| Security headers | `X-Content-Type-Options: nosniff`, `X-Frame-Options: SAMEORIGIN`, `Referrer-Policy: strict-origin-when-cross-origin`, a basic `Permissions-Policy`, and `Strict-Transport-Security` |
| CORS | `Access-Control-Allow-*` for the listed origins, and (unless turned off) an ingress answer to `OPTIONS` preflights |
| Hide server | Removes `Server` and `X-Powered-By` from every response |

HSTS is only accepted when the domain has a publicly trusted certificate: ACME enabled, an uploaded certificate, or TLS terminated by your own proxy. Sending HSTS with a self-signed certificate would lock visitors out, so the API refuses it. When the ingress terminates TLS, HSTS is only sent on HTTPS responses.

CORS echoes the request's `Origin` back only when it is in the list, and adds `Vary: Origin`. `*` allows any origin but cannot be combined with credentials.

```bash
levelrail-cli domains headers preset my-app example.com security
levelrail-cli domains headers preset my-app api.example.com cors --origin https://app.example.com --credentials
levelrail-cli domains headers add my-app example.com --side response --op set --name Cache-Control --value "public, max-age=60"
```

## Path forwarders

A forwarder sends requests that match a path somewhere other than the domain's app. Rules are checked in order, the first match wins, and a request no rule matches goes to the app as usual.

| Field | Meaning |
| --- | --- |
| `match.kind` | `prefix` (segment aware: `/api` matches `/api` and `/api/x`, never `/apix`), `exact`, or `regex` (Go RE2, capped by `APP_DOMAIN_POLICY_MAX_REGEX_LEN`) |
| `match.methods` | Optional method list; empty matches every method |
| `action` | `app` (another Levelrail app by name), `url` (an external `http(s)` URL), or `redirect` |
| `strip_prefix` | Remove the matched prefix before forwarding (prefix rules only) |
| `rewrite_prefix` | Prepend a path; an external URL's own path is used when this is empty |
| `host` | `preserve` (default for apps), `upstream` (default for URLs, sends the target's own host) or `custom` with `host_value` |
| `websocket` | `false` refuses WebSocket upgrades on this path; upgrades pass through by default |
| `timeout_seconds` | Response header timeout; 0 uses `APP_FORWARDER_TIMEOUT` |

An `app` target uses the same upstream the app's own domain routes to; if that app is not running, the rule answers `502` with a plain explanation rather than falling back to the default app.

### Outbound safety

An external URL is resolved and every address it resolves to is checked, both when you save and again on every ingress pass (cached for a minute), so a DNS record changed later to point at an internal address is caught. Refused targets:

- loopback, unless `APP_FORWARDER_ALLOW_LOOPBACK=true`
- private and carrier grade NAT ranges, unless `APP_FORWARDER_ALLOW_PRIVATE_NETWORKS=true`
- link-local and cloud metadata addresses (`169.254.0.0/16`, `fe80::/10`, `fd00:ec2::/32`, `100.100.100.200`), always

The ingress dials the checked address directly (with the original name for TLS), so a lookup at request time cannot be steered elsewhere. A rule whose target is refused answers `502`.

A forwarder to an external URL whose host is the domain itself is refused, and so is a redirect to a path the same rule would match again.

```bash
levelrail-cli domains forwarders add my-app example.com --path /api --app my-api --strip-prefix
levelrail-cli domains forwarders add my-app example.com --path /docs --url https://docs.example.org --host upstream
levelrail-cli domains forwarders add my-app example.com --path /old --kind exact --redirect https://example.com/new --redirect-status 308
```

## Geolocation

A geo rule is an allow list (only these countries) or a deny list (everyone but these) of ISO 3166-1 alpha-2 codes, plus what to do with a blocked visitor: a plain `403` (or `404`, `451`), a redirect, or your own HTML page.

- Private, loopback and link-local addresses are never blocked by country.
- Addresses and ranges in `exempt` always pass.
- `unknown` decides requests whose country cannot be determined: `allow` (default) or `block`.

### Country sources

The ingress needs a country source. Without one, geo rules are saved but inactive, and the dashboard and `GET .../policies` say so.

| Source | How it works |
| --- | --- |
| Proxy header | A CDN or proxy in front adds the visitor's country, for example Cloudflare's `CF-IPCountry`. The header is honoured **only** when the connection comes from an address in `APP_INGRESS_TRUSTED_PROXIES`; anyone else could send it. Set the name with `APP_GEOIP_COUNTRY_HEADER` (default `CF-IPCountry`). |
| Local database | Point `APP_GEOIP_DB` at an `.mmdb` country or city database. Nothing is downloaded by Levelrail; refresh the file yourself and restart. |
| None | No rule is enforced. |

`APP_GEOIP_SOURCE` picks `auto` (default: the header when trusted proxies are configured, the database when it loads, header first), `header`, `mmdb` or `none`.

Two databases work out of the box:

- **MaxMind GeoLite2 Country**: free, but needs a MaxMind account and licence key to download, and its EULA applies (including keeping the file updated).
- **DB-IP IP to Country Lite**: free under CC BY 4.0, no account. Attribution is required wherever you show the data, for example "IP geolocation by DB-IP".

The database is read with [`oschwald/maxminddb-golang`](https://github.com/oschwald/maxminddb-golang) (ISC licence).

Check what a lookup returns:

```bash
levelrail-cli domains geo lookup 81.2.69.160
curl -H "Authorization: Bearer $APP_API_TOKEN" "$APP_API_URL/api/v1/system/geoip?ip=81.2.69.160"
```

The lookup tool only consults the database; the header source depends on the visitor's proxy and cannot be simulated.

```bash
levelrail-cli domains geo set my-app example.com --mode deny --countries RU,KP --exempt 203.0.113.0/24
levelrail-cli domains geo set my-app shop.example.com --mode allow --countries US,CA --action redirect --redirect-url https://example.com/unavailable
```

## Caching

Caching is per domain and off by default. Each rule matches a path (prefix, exact or regex) and says how long to keep a response.

- Only `GET` and `HEAD` are cached; a `HEAD` can be answered from a stored `GET`.
- **Never cached**: requests with `Authorization`, requests with a `Cookie` (unless the rule sets `cache_with_cookies`), range requests, upgrades, and `Cache-Control: no-store` requests. Responses with `Set-Cookie`, `Cache-Control: private`, `no-store` or `no-cache`, `Vary: *`, a `Vary` on a header the rule does not list, a partial content range, or a status outside the rule's list (default `200`).
- **TTL**: the app's `s-maxage`, then `max-age`, then the rule's `ttl_seconds`. With `override_upstream`, the rule's TTL always wins. A TTL of 0 means "do not store". TTLs are capped by `APP_INGRESS_CACHE_MAX_TTL`.
- **Stale while revalidate**: after an entry expires, the first request fetches a fresh copy from the app, and requests arriving while that fetch is running get the expired copy, marked `STALE`, for up to `stale_while_revalidate_seconds`.
- **Key**: the request host, path and query, `Accept-Encoding`, and the rule's `vary` headers. Entries from one host are never served for another.
- Every response on a cached path carries `X-Cache: HIT`, `MISS`, `STALE` or `BYPASS`, and a hit has an `Age` header. A hit answers `If-None-Match` with `304` when the ETag matches.

The cache lives in the control plane's memory: a least recently used store capped by `APP_INGRESS_CACHE_MAX_BYTES` (default 64 MiB) across every domain, with single responses capped by `APP_INGRESS_CACHE_MAX_OBJECT_BYTES` (default 2 MiB) or the domain's own lower `max_object_bytes`. It is per instance, starts empty after a restart, and is not shared between nodes.

```bash
levelrail-cli domains cache add my-app example.com --path /static --ttl 3600
levelrail-cli domains cache purge my-app example.com --prefix /static
levelrail-cli domains cache purge my-app example.com --url "/pricing?plan=pro"
levelrail-cli domains cache stats my-app example.com
```

## Redirects

Whole-domain redirects (one host to a target URL) still use [domain redirects](domains-and-ingress.md#domain-redirects); the controls here build on them.

### Force HTTPS

How plain HTTP is handled depends on where TLS ends, and the Redirects tab says which case applies:

| Setup | Behavior |
| --- | --- |
| The ingress terminates TLS and listens on port 80 | Every domain is already redirected to HTTPS with `308` ("Handled here"). |
| A proxy in front terminates TLS (`tls_terminated_upstream`) | With force HTTPS off, redirecting is up to your proxy ("Handled by your proxy"). With it on, the ingress redirects requests whose `X-Forwarded-Proto` is `http`, trusting that header only from `APP_INGRESS_TRUSTED_PROXIES`. |
| The ingress does not listen for plain HTTP | Nothing to redirect. |

The redirect goes to the public HTTPS port (`public_https_port`), so an internal port such as `:8443` never leaks into a URL. The status is `308` by default, which keeps the method and body, or `301`.

### www and apex

Three presets: **Redirect www to apex**, **Redirect apex to www**, or **Serve both**. Choosing a redirect preset:

1. Adds the other host to the app if it is missing (this part needs the `write` ability, like adding any domain), and shows the DNS record to create for it.
2. Installs a redirect from the non canonical host to the canonical one that keeps path and query.

A preset is refused when the other host belongs to another app, when either host is a wildcard, when it would create a loop, or when either host already redirects somewhere else (pass `replace` to overwrite). A host in maintenance mode keeps answering `503` before any redirect, and the page warns about it.

### Aliases

Pick one of the app's domains as the primary and mark others as aliases: each alias redirects to the primary with path and query kept, using `301`, `302`, `307` or `308`. Domains you unmark stop redirecting to the primary.

### Normalisation

- **Trailing slash**: `add` (paths without a file extension get a slash), `remove`, or `off` (default). Uses `308`.
- **Lower case host**: redirects `Example.COM` to `example.com`. Off by default.

The tab walks `http://www.example.com/a?x=1`, `http://example.com/a` and `https://www.example.com/a` through your settings and shows each hop with its status code before you save.

```bash
levelrail-cli domains redirects force-https my-app example.com on
levelrail-cli domains redirects canonical my-app example.com www-to-apex
levelrail-cli domains redirects alias my-app example.com --alias example.net --alias example.org --status 308
levelrail-cli domains redirect set my-app example.com --preset apex-to-www
```

## Ports

Raw TCP and UDP port forwards are app streams ([raw TCP streams](domains-and-ingress.md#raw-tcp-streams-forwarding-a-non-http-port)); the Ports tab and `domains ports show` list them for the domain's app with their target, who may reach them, and conflicts: another stream on the same port, the HTTP ingress port, or a container that already publishes the port.

"Open this port only to these IPs" writes [firewall rules](domains-and-ingress.md#firewall-ports-80-and-443): one allow rule per address or range, then a deny for everyone else, all labelled `domain-ports:PORT/PROTO`. Removing the restriction deletes exactly those rules.

```bash
levelrail-cli domains ports restrict my-app example.com 5432 --source 203.0.113.10 --source 198.51.100.0/24
levelrail-cli domains ports unrestrict my-app example.com 5432
```

## Limits

Every cap has a default and an environment override on the control plane.

| Variable | Default | Caps |
| --- | --- | --- |
| `APP_DOMAIN_POLICY_MAX_HEADER_RULES` | 50 | Header rules per domain |
| `APP_DOMAIN_POLICY_MAX_FORWARDERS` | 50 | Forwarders per domain |
| `APP_DOMAIN_POLICY_MAX_CACHE_RULES` | 20 | Cache rules per domain |
| `APP_DOMAIN_POLICY_MAX_REGEX_LEN` | 256 | Bytes in a path or regex |
| `APP_DOMAIN_POLICY_MAX_HEADER_VALUE_LEN` | 4096 | Bytes in a header value |
| `APP_DOMAIN_POLICY_MAX_GEO_EXEMPT` | 256 | Geo exempt entries |
| `APP_DOMAIN_POLICY_MAX_BODY_BYTES` | 65536 | Custom geo page size |
| `APP_FORWARDER_TIMEOUT` | 30s | Default forwarder response header timeout |
| `APP_FORWARDER_MAX_TIMEOUT` | 10m | Largest per-rule timeout |
| `APP_FORWARDER_DIAL_TIMEOUT` | 5s | Forwarder connect timeout |
| `APP_INGRESS_CACHE_MAX_BYTES` | 67108864 | Whole in-memory cache |
| `APP_INGRESS_CACHE_MAX_OBJECT_BYTES` | 2097152 | One cached response |
| `APP_INGRESS_CACHE_MAX_TTL` | 168h | Any cache TTL |

## Permissions and audit

| Action | Ability |
| --- | --- |
| Read any section, preview, cache stats, geo lookup | `read` |
| Change or delete headers, forwarders, geo, cache, redirect settings, canonical and aliases; purge the cache | `deploy`, the same tier as other per-domain routing settings |
| Attach the www or apex counterpart while applying a preset | additionally `write` |
| Restrict or unrestrict a port | `write:sensitive`, the same tier as firewall rules |

Every change is recorded in the audit log like any other API write.

## Next steps

<CardGroup :cols="2">
<Card title="Domains and ingress" href="/domains-and-ingress">

TLS, WAF, maintenance mode and the rest of the ingress.

</Card>
<Card title="Load balancing" href="/load-balancing">

Balance traffic across an app's replicas.

</Card>
</CardGroup>
