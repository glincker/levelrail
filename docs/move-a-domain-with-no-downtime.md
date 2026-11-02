---
title: Move a domain with no downtime
description: "Test a domain on its new Levelrail server before touching DNS, get its certificate ahead of time, then switch with a checklist that shows each step."
"og:url": "https://docs.levelrail.com/move-a-domain-with-no-downtime"
---

Changing DNS first and fixing things after is where downtime comes from. The new server has no certificate for the name until DNS points at it, caches keep sending visitors to the old place, and nothing tells you which of the two is the problem. The cutover checklist reverses the order: prove the new server works while the old one still serves traffic, then switch.

Open the domain's page and look for the **Cutover** card, or run:

```bash
levelrail-cli domains cutover shop.example.com
```

## The six steps

| Step | What it checks | Needs DNS to move? |
| --- | --- | --- |
| 1. The app answers on this server | Levelrail asks its own ingress for `https://shop.example.com`, like `curl --resolve`. DNS is not involved. | No |
| 2. The certificate is ready | A trusted certificate is stored and served for the name. | No, with a connected DNS provider |
| 3. Current DNS and TTL | Where the name points now (previous server, Cloudflare proxy, or already here) and its TTL. | No |
| 4. Switch DNS | The exact record to create or change. | Yes, this is the switch |
| 5. Verified from authoritative servers | The zone's own name servers all answer with this server. | After the switch |
| 6. Verified end to end | A real request over the public name gets an answer with a trusted certificate. | After the switch |

Each step shows pass, wait or fail, and an estimate of how long a waiting step usually takes.

The downtime verdict is green (**no downtime expected**) only when steps 1 and 2 pass. If either does not, the card says downtime is likely and shows which step to fix first.

## Get the certificate before switching

Without a certificate, the first visitors after the switch see a browser warning until HTTP-01 succeeds, and HTTP-01 can only succeed once DNS points at the new server. With a DNS provider connected, skip that wait.

1. Connect Cloudflare, Route53 or Google Cloud DNS (see [Domains and ingress](/domains-and-ingress#wildcard-domains-dns-01-providers)).
2. On the Cutover card press **Get certificate before switching DNS**, or run `levelrail-cli domains cutover shop.example.com --get-certificate`.
3. Wait for step 2 to turn green. The certificate is requested through a DNS-01 challenge, which only needs a TXT record in your zone, so the old server keeps serving traffic meanwhile.

Only this domain switches to DNS-01. Every other host on the server keeps its own challenge, and the `ingress.dns01_all_hosts` setting is not touched, so a broken DNS credential later cannot stop renewals for hosts you never moved. It needs a contact email in the ingress settings and a provider that manages the domain's zone. If the CA refuses, the card shows its message; see the [ACME verification runbook](/acme-verification-runbook) for what each message means.

Once DNS points at this server and the certificate exists, press **Switch back to HTTP-01 for this domain** (or run `levelrail-cli domains cutover shop.example.com --revert-challenge`). The issued certificate stays; later renewals use HTTP-01 again. The same button appears if the DNS-01 request failed. If the DNS provider is disconnected while the domain is still on DNS-01, the domain falls back to HTTP-01 and shows a warning instead of breaking the other hosts.

### No DNS provider connected

HTTP-01 works only after the switch, so reduce how long caches hold the old answer:

1. Lower the record's TTL to the value the card shows (300 seconds by default), at least one old TTL before the switch. If the old TTL was one day, do this a day ahead.
2. Switch the record at a quiet time.
3. Watch steps 5 and 6. The certificate is issued as soon as the CA can reach the name.

## Cloudflare proxied names

If the name answers from Cloudflare addresses, the card says so. The cutover works with the proxy on or off: switch the record's target, keep the orange cloud as it is, and step 6 is verified through Cloudflare. If you turn the proxy off, set SSL mode to Full (strict) once the certificate is issued. See [CDN and caching](/cdn-and-caching).

## Verify after the switch

`levelrail-cli domains check --cutover <domain>...` (or `GET /api/v1/domains/{domain}/verify`, read ability) checks one or many domains that are moving here and ends with a PASS, WARN or FAIL per domain. Each line carries the next step: DNS against this host's public addresses (`--expect-ip` overrides them, for a host behind NAT), a proxy such as Cloudflare in front, the http to https redirect, certificate issuer and days left, HSTS, response status and time to first byte as seen from this host, and whether the www or apex partner points at the same place. The command only reads, accepts only domains this server already serves, and requests the name only once DNS points here or at a known proxy. The exit code is 1 when any domain has a FAIL, so it can gate a script.

The thresholds are runtime settings (Settings > Runtime in the dashboard, or `levelrail-cli settings runtime set <key> <value>`). A saved value wins, the environment variable only seeds the default, and a change applies to the next check with no restart.

| Setting | Environment seed | Default | Meaning |
| --- | --- | --- | --- |
| `cutover.verify_cert_warn_days` | `APP_CUTOVER_VERIFY_CERT_WARN_DAYS` | 21 | Days left at or below which the certificate line warns (1 to 365) |
| `cutover.verify_cert_fail_days` | `APP_CUTOVER_VERIFY_CERT_FAIL_DAYS` | 7 | Days left at or below which the certificate line fails (1 to 365) |
| `cutover.verify_slow_ttfb` | `APP_CUTOVER_VERIFY_SLOW_TTFB` | 1.5s | Time to first byte above which the response line warns (100ms to 30s) |
| `cutover.verify_hsts_min_age` | `APP_CUTOVER_VERIFY_HSTS_MIN_AGE_SECONDS` | 15552000 | Smallest HSTS max-age that counts as enabled (3600 to 63072000) |
| `cutover.verify_timeout` | `APP_CUTOVER_VERIFY_REQUEST_TIMEOUT` | 5s | Timeout of each request (1s to 1m) |

## API

`GET /api/v1/domains/{domain}/cutover` (read ability) returns `risk` (`none`, `likely` or `unknown`), `ready_to_switch`, the `steps`, the current `dns` view with the exact `records`, and the `certificate` state. `POST /api/v1/domains/{domain}/cutover/certificate` (root ability) requests this domain's certificate through the connected DNS provider, and `DELETE` on the same path switches the domain back to HTTP-01. The probe only dials this server's own ingress until the name already points here or at Cloudflare, and results are cached for a few seconds per domain.

| Setting | Default | Meaning |
| --- | --- | --- |
| `APP_CUTOVER_LOW_TTL_SECONDS` | 300 | TTL at or below which the switch counts as quick, and the value advised |
| `APP_CUTOVER_DEFAULT_ETA_SECONDS` | 1800 | Propagation estimate when the TTL is unknown |
| `APP_CUTOVER_CERT_ETA_SECONDS` | 120 | Usual time for the certificate request |
| `APP_CUTOVER_MIN_INTERVAL` | 3s | Minimum time between probe runs for one domain |
| `APP_CUTOVER_NS_LIMIT` | 4 | Name servers asked in step 5 |

The per-domain DNS-01 path is covered by tests of the generated configuration but not yet verified against a real CA and DNS zone. If it fails for your zone, switch back to HTTP-01 and fall back to lowering the TTL.
