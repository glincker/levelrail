# ADR 038: The dns_provider plugin kind and the extracted Cloudflare DNS plugin

Status: Accepted (slice 2 of ADR 034 step 2, 2026-10-19)

Date: 2026-10-19

## Context

ADR 034 names Cloudflare DNS as the first provider to extract behind the plugin
contract, running beside the built-in client until it matches it. The contract
only had `notification_channel` and `panel_provider`. The built-in Cloudflare
path lives in `internal/dnsrecords` and `internal/api/dns_automation.go`, and
every "add the DNS record for this domain" action (the domain Fix button,
go-live, the add-domain `dns` field, the records editor) goes through one
seam, `resolveDNSTarget`, which returns a `dnsrecords.Manager`.

## Decision

1. **New kind `dns_provider`.** `proto/plugin/v1` gains `DnsProviderService`
   with `ListZones`, `ListRecords`, `UpsertRecord`, `DeleteRecord` and
   `VerifyRecord`. Contract version stays 1: the service is additive and a
   plugin registers only the services its kind needs.
2. **Secret per call, never stored.** Every request carries the provider
   credential in `secret` for that one call, exactly like `Send`. The host
   reads it from the existing sealed Cloudflare DNS credential, so there is no
   second place to store it. The plugin config is non-secret (`{}` for
   Cloudflare). Plugin messages are redacted of the secret on the host as well
   as in the plugin.
3. **Abilities.** A `dns_provider` requests `read` like a notification channel
   and nothing more. The write is a decision the host takes (an operator
   action, root-gated at the API); the plugin only executes it with the
   credential it was handed. Each call is audited as an ability use with the
   call name and zone, never the secret.
4. **One seam, one explicit setting.** `pluginDNSTarget` plugs into
   `resolveDNSTarget` and the records editor. A stored setting
   `dns_provider_plugin` (inside the domain automation document, no migration)
   selects the plugin. Unset, which is the default, the built-in client runs
   exactly as before. There is no fallback between the two: with a plugin
   chosen, a plugin failure is reported, not silently retried through the
   built-in path, so an operator always knows which code wrote a record.
5. **Half success is a first-class outcome.** The host adapter verifies every
   write through `VerifyRecord`. A record the plugin accepted but cannot see
   afterwards is reported as unconfirmed and stays in the undo list, so the
   operator can run the fix again or undo it.
6. **Operator surfaces.** Plugins page: test connection and "use for DNS
   records" on a running `dns_provider` plugin. Domains page: a writer select
   in the domain automation card. CLI: `plugins test <name>` and `plugins
   dns-provider [name|--off]`. API: `POST /api/v1/plugins/{name}/test` and
   `GET/PUT /api/v1/settings/dns-provider-plugin`, writes root only.
7. **The built-in client stays** until the plugin has run through a real
   migration of the owner's zones. Removal is a later ADR step, only if it
   removes complexity (ADR 034 point 7).

## Rejected alternatives

- **Handing the plugin a platform API token to call back.** Rejected: the
  plugin would hold a long-lived credential and could act beyond DNS. Per-call
  secrets keep the blast radius to one call.
- **A new ability such as `write:dns`.** Rejected for now: plugin abilities are
  limited to `read` and `read:sensitive` (ADR 034), the host already gates the
  write, and widening the vocabulary belongs to a deliberate ADR.
- **A generic "dns record" RPC that reuses libdns shapes.** Rejected: libdns
  types would leak a Go library into a language-neutral contract.
- **Automatic fallback to the built-in client when the plugin fails.**
  Rejected: it hides which implementation wrote the record and can double
  write after a half success.
- **Per-plugin credential storage inside the plugin.** Rejected: ADR 034
  point 3.

## Consequences

- DNS calls now add a unix socket hop and a failure mode (plugin down, circuit
  open). The Fix flow reports it as a normal DNS error with a clear message.
- The Cloudflare proxied flag is not toggled through the plugin yet; proxy
  integration keeps using the built-in client's extra call and is skipped when
  a plugin writes the record.
- Route53 is not extracted. It follows the same contract when someone needs it.

## What we do not do

- No removal of the built-in Cloudflare client in this change.
- No signing keys, images or marketplace entries are published here.
