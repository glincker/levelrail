---
title: "Egress dry run (beta)"
description: "See which names an app asked the mesh DNS server for that its egress allowlist would deny, before you rely on the allowlist. Read only, nothing is enforced."
"og:url": "https://docs.levelrail.com/egress-dry-run"
---

**Beta.** The report is read only. It changes no firewall rule and no allowlist.

An app with an egress allowlist can reach only the hosts you list. Before you tighten one, the dry run shows which names the app actually looked up, and which of them the allowlist would refuse.

## What it needs

- [DNS query visibility](/dns-visibility) on (`dns.query_log_enabled`), with privacy `full` or `names_only`. Under `counters_only` no names are stored, so the report says it has no data.
- The app resolves through the mesh DNS server on its node ([Mesh DNS on the bridge gateways](/mesh-dns)). Lookups that never reach that server are not seen.
- An egress allowlist on the app (`levelrail-cli apps egress set`). Without one the report says nothing would be denied.

The log lives in memory and is gone after a restart, so run the app through its normal traffic first.

## Read the report

```sh
levelrail-cli apps egress report web
```

Each row is one name, how many times it was asked, when it was last seen and the verdict. Names the allowlist would refuse come first. Names the mesh server answered from its own zone are left out, they are not outbound traffic.

Example (illustrative, built from the test fixture, not captured on a host):

```text
NAME                     COUNT  LAST SEEN  VERDICT
tracker.bad.test         1      14:02:11   would be denied
api.example.com          2      14:02:11   allowed
cdn.static.example.org   1      14:02:11   allowed
```

Use `--json` for the full body, or call `GET /api/v1/apps/{name}/egress-report`. It needs read access to the app.

## Limit

`dns.egress_report_max_names` (default `500`, range 10 to 10000, live) caps how many distinct names one report lists. When more exist, the output says how many were cut. The default is seeded from `LEVELRAIL_DNS_EGRESS_REPORT_MAX_NAMES`, and the setting can be changed in Settings, the API or the CLI without a restart.

## Limits of the dry run

- It matches names against the allowlist hosts. It does not look at ports or at connections to a bare IP address.
- Enforcement works on resolved addresses, so a name that shares an address with an allowed host is not blocked even if the report lists it as denied.
