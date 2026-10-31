# DNS zones and nameserver delegation

A zone at Cloudflare, Route 53 or Google Cloud DNS only answers for your domain once the registrar points the domain's nameservers at it. The DNS page, `GET /api/v1/dns/zones` and `levelrail-cli domains dns zone <id>` show the nameservers the provider assigned and compare them with the NS set public resolvers return for the zone apex (case and trailing dots ignored).

| `delegated` | Meaning | What to do |
| --- | --- | --- |
| `yes` | Every public nameserver is one the provider assigned. | Nothing. Records are live. |
| `no` | None of them are. `current_nameservers` shows where the domain points today. | At your registrar (see `registrar_hint`), replace the nameservers with the zone's `nameservers`. |
| `partial` | Some public nameservers are the provider's, some are not. | Remove the old nameservers at the registrar so every resolver gets the same answers. |
| `unknown` | The lookup failed, or the zone is private (`private_zone`) and is never checked. | Retry later; private zones need no delegation. |

Records added to a zone that is `no` or `partial` are still saved, but the DNS host detection, the record status, the mail DNS plan and the app go-live plan carry `delegation_warning: true` with a plain reason until the registrar change propagates (usually minutes, up to 48 hours).

Lookups go only to public resolvers (`APP_DNS_PUBLIC_RESOLVERS=off` uses the system resolver). Tuning: `APP_DNS_DELEGATION_TIMEOUT_MS`, `APP_DNS_DELEGATION_CACHE_SECONDS`, `APP_DNS_DELEGATION_CONCURRENCY`. The registrar hint comes from public RDAP data (`APP_DNS_RDAP_TIMEOUT_MS`, `APP_DNS_RDAP_CACHE_SECONDS`, `APP_DNS_RDAP_MAX_BYTES`) and is left out when RDAP does not answer cleanly.
