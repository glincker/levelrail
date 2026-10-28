---
title: "Domain setup stepper"
description: The four-step guide on the app Domains tab that adds a domain, shows the DNS records, watches DNS resolve and reports the certificate.
---

Open an app, go to Domains and choose Guided setup.

1. **Domain.** Type the hostname. The form validates it, offers the www or apex counterpart, and warns when a wildcard needs a DNS provider.
2. **DNS records.** The exact A, AAAA or CNAME values for this server, each with a Copy button. Pick Cloudflare, Route 53, Namecheap or Other for a short hint. The provider connected to this server is preselected.
3. **Live check.** Expected and resolved values side by side, polled automatically. Press Re-check to look again.
4. **Certificate.** Pending, issued or failed. A failure shows the reason and the suggested next step, with a link to the app logs.

## Tuning

| Variable (dashboard build) | Default | Effect |
| --- | --- | --- |
| `VITE_DOMAIN_CHECK_POLL_MS` | 5000 | Time between automatic DNS checks |
| `VITE_DOMAIN_CHECK_MAX_POLLS` | 24 | Automatic checks before polling stops |

The stepper uses the existing domain, server-address and certificate status endpoints. It adds none.
