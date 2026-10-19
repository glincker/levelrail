# Cloudflare DNS plugin

Lists zones and manages DNS records in Cloudflare for the control plane, on the public `dns_provider` plugin contract.

Kind `dns_provider`, ability `read`, outbound host `api.cloudflare.com`.

## Config

None. The Cloudflare API token (Zone:DNS:Edit) is the one already stored under Settings for Cloudflare DNS. The control plane hands it to the plugin on every call and the plugin never stores or logs it.

## Use

```bash
scripts/plugins-build.sh all cloudflare-dns
levelrail-cli plugins install --manifest dist/plugins/cloudflare-dns/plugin.yaml --signature dist/plugins/cloudflare-dns/plugin.sig --approve read
levelrail-cli plugins enable cloudflare-dns
levelrail-cli plugins test cloudflare-dns
levelrail-cli plugins dns-provider cloudflare-dns
```

`plugins dns-provider --off` returns DNS writes to the built-in client.

## Behaviour

- Upsert replaces the record with the same name and type, or creates it.
- Delete removes only a record whose name, type and value all match.
- Verify reports whether Cloudflare now serves the exact record.
- 401 and 403 are permanent, 429 and 5xx are reported retryable, and the token is redacted from every message.

Tests: `go test ./plugins/cloudflare-dns`.
