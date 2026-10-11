---
description: Manage DNS zones and records at Cloudflare or Route53, move a domain's name servers, verify delegation and propagation, and serve wildcard subdomains.
---

# DNS zones and records

The **DNS** page (Infrastructure, DNS) manages whole zones at the DNS provider you already connected for certificates: Cloudflare or Amazon Route53. It covers creating a zone, moving a domain's name servers to it, importing what the domain serves today, editing records, and checking that the world sees your changes. The CLI has the same surface under `levelrail-cli dns`, and the API lives under `/api/v1/dns`.

Nothing here is a DNS server of its own. Zones and records live at the provider; the control plane calls its API with the credentials stored under Domains, DNS provider.

<InlineToc default-open />

## Connect a provider

Zone management reuses the DNS-01 credentials from [Domains and ingress](domains-and-ingress.md#wildcard-domains-dns-01-providers). Zone level work needs a little more access than DNS-01 alone.

<Tabs :items="['Cloudflare', 'Route53']">
<Tab value="Cloudflare">

Create an API token (never the global API key) with:

| Permission | Needed for |
| --- | --- |
| Zone, Zone, Read | listing zones and reading their name servers |
| Zone, DNS, Edit | records, imports, templates, wildcard records, DNS-01 |
| Zone, Zone, Edit | creating and deleting zones (skip it if you only edit records) |

```bash
levelrail-cli domains cloudflare-dns set --cf-api-token <token>
```

A new zone is created in the account the token's existing zones belong to. When the token sees several accounts, pass `--account-id` to `dns zones create`.

</Tab>
<Tab value="Route53">

Attach an IAM policy with these actions to the access key:

```json
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Action": [
      "route53:ListHostedZones",
      "route53:GetHostedZone",
      "route53:CreateHostedZone",
      "route53:DeleteHostedZone",
      "route53:ListResourceRecordSets",
      "route53:ChangeResourceRecordSets",
      "route53:GetChange",
      "route53:ListHealthChecks",
      "route53:CreateHealthCheck",
      "route53:DeleteHealthCheck"
    ],
    "Resource": "*"
  }]
}
```

Drop `CreateHostedZone` and `DeleteHostedZone` to allow record edits only, and the three health check actions if you do not use failover records. When the key may not list zones, set the hosted zone ID in the Route53 settings and that one zone is used.

```bash
levelrail-cli domains route53-dns set --aws-access-key-id <id> --aws-secret-access-key <secret>
```

</Tab>
</Tabs>

When both providers are connected, Cloudflare is the default and `--provider route53` (or `?provider=route53`) picks the other one. Tokens are envelope encrypted at rest, never returned by any API, and never written to logs or audit rows.

## Bring a domain onto a zone

The **Add zone** wizard has four steps, and `levelrail-cli dns zones ...` does the same from a terminal.

1. **Domain.** Enter the registrable domain (`example.com`, not `www.example.com`). The zone is created at the provider.
   ```bash
   levelrail-cli dns zones create example.com
   ```
2. **Name servers.** The provider assigns name servers to the zone. Set exactly these at your registrar, replacing the current ones; a mix of old and new answers inconsistently.
   ```bash
   levelrail-cli dns zones nameservers example.com
   ```
3. **Import records.** Before switching, copy what the domain serves today. Zone transfers (AXFR) are almost always refused, so the wizard looks up a list of common names (`@`, `www`, `mail`, `api`, DKIM selectors, `_dmarc`, common SRV names and more) at the domain's current name servers and shows what it found. Untick anything you do not want and add any other names you use. Only the sets you keep are imported.
4. **Verify.** The system resolver, `1.1.1.1` and `8.8.8.8` are asked for the domain's NS records and compared with the zone's assigned servers.
   ```bash
   levelrail-cli dns zones verify example.com
   ```

| State | Meaning |
| --- | --- |
| `delegated` | Every resolver returns only this zone's name servers. |
| `partially_delegated` | Some resolvers return this zone's servers, others still return old ones, or one answer mixes both. Wait for caches, or remove stray servers at the registrar. |
| `delegated_elsewhere` | Resolvers return other name servers only. The list is shown so you can see where the domain still points. |
| `not_delegated` | No resolver returned name servers. The domain may be unregistered or have no name servers set. |

A resolver that returns a subset of the assigned servers counts as a match: a registrar given two of Route53's four servers still delegates correctly. Registries usually publish a name server change within an hour, but resolvers may keep the old answer for up to two days.

## Records

The zone page lists record sets: all values of one name and type together. Search matches names, values and set identifiers; the type filter narrows to one type. Lists longer than 50 sets are virtualized.

Supported types are A, AAAA, CNAME, TXT, MX, CAA, SRV and NS (for delegating a subdomain; apex NS and SOA are provider managed and read only). Each type has its own fields in the record dialog, and every value is checked twice: in the browser for instant feedback and on the server, which parses each value with `miekg/dns` before anything reaches the provider.

```bash
levelrail-cli dns records list example.com --type MX
levelrail-cli dns records add example.com --name www --type A --value 203.0.113.10 --value 203.0.113.11
levelrail-cli dns records add example.com --name @ --type MX --value "10 mail.example.com"
levelrail-cli dns records update example.com --name www --type A --value 203.0.113.12 --ttl 600
levelrail-cli dns records delete example.com --name old --type CNAME
```

Rules the server enforces:

- A CNAME cannot share a name with any other record, in either direction.
- A CNAME at the apex is refused on Route53 (use an A record or a Route53 alias) and allowed with a warning on Cloudflare, which flattens it.
- A CNAME set holds exactly one value.
- `proxied` applies to A, AAAA and CNAME on Cloudflare only. TTL `1` means automatic and is Cloudflare only.
- TTL is otherwise between 30 seconds and 7 days. The default is `APP_DNS_DEFAULT_TTL`, or 300 seconds.
- TXT values are the raw text; splitting into 255 byte strings and quoting is handled for you.

Route53 alias records are shown read only; edit them in the AWS console.

### Routing policies (Route53)

On Route53, a record set can use a routing policy. Several sets then share a name and type and are told apart by a **set identifier**.

| Policy | Fields | Use |
| --- | --- | --- |
| Weighted | weight 0 to 255 | split traffic, canary a new server |
| Failover | role `PRIMARY` or `SECONDARY`, health check | send traffic to a standby when the primary fails its health check |
| Multivalue answer | optional health check | return up to eight healthy addresses |

```bash
levelrail-cli dns health-checks create --type HTTPS --fqdn app.example.com --path /healthz
levelrail-cli dns records add example.com --name app --type A --value 203.0.113.10 \
  --routing failover --set-id primary --failover PRIMARY --health-check <id>
levelrail-cli dns records add example.com --name app --type A --value 198.51.100.20 \
  --routing failover --set-id standby --failover SECONDARY
```

A name cannot mix simple and routed sets of the same type. Cloudflare has no equivalent in plain DNS (its load balancing is a separate paid product), so routing policies are refused there with a clear error.

### Import and export

Import accepts a BIND zone file or the JSON this page exports, as a file or pasted text. It always produces a **plan** first: what will be created, updated, deleted or left alone, with conflicts flagged. Nothing changes until you apply a plan without conflicts.

By default an import never deletes anything, so records the file does not mention stay put. **Replace** deletes record sets missing from the file (never apex NS, SOA or alias sets) and requires typing the zone name.

```bash
levelrail-cli dns records export example.com --format bind --out example.com.zone
levelrail-cli dns records import example.com --file example.com.zone            # preview
levelrail-cli dns records import example.com --file example.com.zone --apply
levelrail-cli dns records import example.com --file full.zone --replace --confirm example.com --apply
```

Routed and alias sets have no zone file form; a BIND export lists them as comments, and the JSON export keeps them.

### Templates

Templates add common setups as a plan you review before applying:

| Template | Records |
| --- | --- |
| `email` | MX to your mail host, SPF, DMARC |
| `dkim` | a DKIM public key under a selector |
| `verification` | a verification TXT token |
| `www-to-apex` | `www` CNAME to the apex |
| `google-workspace` | Google's MX, SPF include, DMARC |
| `microsoft-365` | Exchange Online MX, SPF include, autodiscover, DMARC |
| `caa-letsencrypt` | CAA `issue` and `issuewild` for Let's Encrypt |

TXT values already at the same name are kept (verification tokens survive), except a second SPF or DMARC record, which would break mail authentication.

```bash
levelrail-cli dns records template example.com google-workspace --param policy=quarantine --apply
```

## Health and propagation

The zone overview shows record counts by type, the delegation state, and the last change made through this control plane (who and which action), falling back to the provider's own modified time.

The **Propagation** tab, or `levelrail-cli dns check`, asks one name and type of the system resolver, `1.1.1.1`, `8.8.8.8`, and, when the zone is known, each of the zone's own name servers directly. Each answer shows its values and remaining TTL, whether all answers agree, and whether each matches what the zone holds. A resolver that still differs from the authoritative answer is waiting out its TTL.

```bash
levelrail-cli dns check www.example.com --type A
```

Set `APP_DNS_PUBLIC_RESOLVERS=off` to ask only the system resolver, for example on a network that blocks outbound DNS.

## Wildcard subdomains

An app can serve every name under a domain: add `*.example.com` (or `*.apps.example.com`) as one of its domains.

- Only a single leading `*.` label is allowed. `*.com`, `*.*.example.com` and `a.*.example.com` are refused.
- An exact domain always wins: if another app owns `api.example.com`, requests for it go to that app, and every other name under `*.example.com` goes to the wildcard app. A wildcard matches exactly one label, so `*.example.com` does not match `a.b.example.com`.
- The certificate must come from DNS-01, because HTTP-01 cannot issue wildcard certificates. Adding a wildcard domain is refused until Cloudflare or Route53 is connected; the error links to the Domains page and names the CLI command.
- When the provider manages the domain's zone, a `*` A record pointing at this server is created for you. An existing `*` record is never overwritten.
- The domain check resolves a random name under the wildcard, so it tests the wildcard record rather than a cached exact name. The Domains table marks these domains **Wildcard**.

Per environment and preview environment wildcards are not part of this yet.

## Permissions and audit

Reads (zones, records, delegation, checks, exports) need the `read` ability. Every write against the provider (zones, records, imports, templates, health checks) needs `root`, because it changes live infrastructure outside this server. Each write is audited with an action name (`dns_zone.create`, `dns_zone.delete`, `dns_record.create`, `dns_record.update`, `dns_record.delete`, `dns_record.import`, `dns_record.template`, `dns_health_check.create`, `dns_health_check.delete`); filter the audit log by `dns_record` or `dns_zone`.

Deleting a zone requires typing its name, and a zone that still has records beyond NS and SOA also needs **force** (`--force`). Export it first.

## Limits

- Discovery before delegation is a best effort lookup of common names; names nobody guessed are not found. Add them in the wizard.
- Cloudflare zone listings do not include a record count; Route53 does.
- Route53 private hosted zones are listed but have no delegation to verify.
- Each change is applied as it is planned, one record set at a time; a provider error part way through an import stops it and reports how many changes landed.

## Roadmap

How the Route53 feature set maps onto this page:

| Route53 feature | Status | Why |
| --- | --- | --- |
| Public hosted zones | Supported | Create, list, delete, name servers. |
| Record types and multi value sets | Supported | A, AAAA, CNAME, TXT, MX, CAA, SRV, NS. |
| Weighted, failover, multivalue routing | Supported | Same plumbing: set identifier plus one field. |
| Health checks | Supported | HTTP, HTTPS and TCP checks for failover and multivalue sets. |
| Alias records | Read only | Alias targets are AWS resources this server does not manage. |
| Latency and geolocation routing | Later | Needs region and location pickers; shown read only today. |
| Private hosted zones | Later | Needs VPC association; listed, not created. |
| DNSSEC signing | Later | Needs KMS keys and DS records at the registrar. |
| Reusable delegation sets | Later | Useful for white label name servers across many zones. |
| Query logging | Later | Writes to CloudWatch Logs, outside this platform's log store. |
| Traffic flow policies | Not planned | Priced per policy record and overlaps the routing above. |
| Domain registration and transfer | Not planned | Stays with your registrar. |

Also later: ownership TXT markers (as Kubernetes external-dns does) so imports can tell records this platform created from foreign ones, and Cloudflare load balancing pools.
