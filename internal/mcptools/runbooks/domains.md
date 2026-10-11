# Runbook: domains and TLS

## Domain does not load

1. domain_doctor for the app and domain: ordered checks for DNS, routing, TLS and upstream, each with a suggested fix. It changes nothing.
2. check_domain_dns: does the record point at this server.
3. get_domain_tls_cert_status and list_certificates: issuance state, expiry, last error.
4. get_app_status and failure_context: a healthy domain in front of a crashing app still fails.

## Common causes

- DNS not yet pointing here, or a proxy in front hiding the real record.
- Port 80 not reachable from the internet, so the ACME challenge cannot complete.
- Domain already used by another app (the change is refused and nothing is modified).

## Go live

`levelrail-cli domains go-live <app> <domain> --plan` previews, then `--wait` applies and waits. `levelrail-cli domains go-live undo <run-id>` reverses a run.

`levelrail-cli domains doctor <app> <domain>` prints the same report as domain_doctor.
