---
title: "Mail DNS"
tag: "Experimental"
description: Build, review, apply and check the DNS records a domain needs for email, for your own mail server or an external provider. Add-only by default.
"og:url": "https://docs.levelrail.com/mail-dns"
---

Mail DNS builds the records a domain needs for email and checks them against live DNS. It works with any mail engine: a server you run or a hosted provider. It only touches DNS, so it costs the control plane nothing beyond the DNS provider calls. It is experimental: switch it on with `APP_EXPERIMENTAL=mail` (see [Experimental features](/experimental-features)). While it is off the routes answer `404`.

The zone must live at a connected DNS provider (see [DNS zones and records](/dns)) and its name must equal the domain.

## Plan, apply, check

```
levelrail-cli mail dns plan example.com --ipv4 203.0.113.7
levelrail-cli mail dns apply example.com --ipv4 203.0.113.7 --plan-hash <hash> --confirm
levelrail-cli mail dns check example.com --host mail.example.com --dkim-selector s1
```

`plan` reads the live zone and prints what would be added, left unchanged, replaced or refused. It writes nothing. `apply` needs the hash from the plan and `--confirm`; it plans again and refuses with `409` if the zone or your flags changed since, so a stale plan never applies. Use the same flags on both. Applying writes an audit entry with record names and types only. Records are written one set at a time, not as one atomic batch, but every record is validated before the first write.

API: `POST /api/v1/mail/dns/plan`, `POST /api/v1/mail/dns/apply` (body needs `confirm: true` and `plan_hash`), `GET /api/v1/mail/dns/check?domain=`.

## Add-only by default

Changing mail records can break a live domain. By default nothing existing is overwritten:

- If the domain already has MX records that point elsewhere, the plan is refused with a clear message.
- An existing SPF, DKIM, DMARC or other record with different content is refused, not merged.
- Other TXT values at the same name (such as site verification tokens) are never touched. Replacing SPF edits only the `v=spf1` value.

To replace, list the kinds explicitly: `--replace-existing MX,SPF`. Valid kinds are `MX`, `HOST`, `SPF`, `DKIM`, `DMARC`, `MTASTS`, `TLSRPT` and `AUTOCONFIG`. The plan shows each value it would remove.

## What each record is

| Record | Purpose |
| --- | --- |
| `A` / `AAAA` for the mail host | Where the mail server lives. Built from `--ipv4` and `--ipv6` when the host is inside the zone. |
| `MX` | Where mail for the domain is delivered. |
| `SPF` (TXT at the apex) | Which servers may send. `--spf-mode server` is `v=spf1 mx ip4 ip6 ~all`; `include` lists external senders with `--spf-include`. Never `+all`. |
| `DKIM` (TXT at `<selector>._domainkey`) | The public key receivers use to verify signatures. Without a key a note says it is pending and no record is built. Paste the public key only. |
| `DMARC` (TXT at `_dmarc`) | Policy for mail that fails SPF and DKIM: `none`, `quarantine` or `reject`, with a report address and percentage. Start with `none`. |
| `MTA-STS` (TXT at `_mta-sts`) | Asks senders to use TLS. The policy file must be served over HTTPS at `mta-sts.<domain>`; the plan prints its exact content but does not host it. The `id` is a hash of the policy. |
| `TLS-RPT` (TXT at `_smtp._tls`) | Where senders report TLS delivery failures. |
| Autoconfig | CNAMEs for `autoconfig` and `autodiscover` plus `_submission._tcp` and `_imaps._tcp` SRV records (RFC 6186), so mail apps find the server. |

## Provider presets

`--preset` fills the MX, SPF include and fixed DKIM records of a hosted provider. Included: `google`, `fastmail`, `proton`, `zoho` (US data center), and the sending-only `ses`, `mailgun` and `postmark`. Each preset notes where its values come from and which records still come from the provider's dashboard (per-account DKIM keys, verification tokens). Check them against the provider's current instructions before applying.

## Deliverability reality

DNS is the easy part. For a server you run:

- Many cloud hosts block outbound port 25 by default. Ask the host to open it. `check` reports this as not tested.
- The sending IP needs reverse DNS (PTR) that resolves back to the mail host name. You set PTR at the IP owner, not in your zone. `check` verifies it.
- A new IP has no reputation. Warm it up slowly, keep DMARC reports flowing, and expect early mail to land in spam.
- Residential IP ranges are usually rejected outright.
