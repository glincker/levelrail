# Mailgun email plugin

Emails each alert through the Mailgun HTTP API, for operators who want API delivery instead of the control plane's SMTP or Resend channels.

Kind `notification_channel`, ability `read`, outbound hosts: `api.mailgun.net`, `api.eu.mailgun.net`.

## Config

Plugin-wide, non-secret, passed at install with `--config`:

```json
{"domain": "mg.example.com", "region": "us", "from": "Alerts <alerts@mg.example.com>", "to": "ops@example.com"}
```

| Field | Required | Meaning |
| --- | --- | --- |
| `domain` | yes | Sending domain verified in Mailgun |
| `from` | yes | From address |
| `to` | yes | Recipient address |
| `region` | no | `us` (default) or `eu` |

## Channel target

The channel's target field holds the Mailgun API key. It is sealed like any channel credential and sent per call. The plugin never stores or logs it.

```bash
levelrail-cli plugins install --manifest plugin.yaml --signature plugin.sig --config config.json --approve read
levelrail-cli channels create --name ops-mailgun --kind plugin:mailgun --notify-url key-XXXX
```

## Behaviour

- 401 and 403 are permanent ("authentication failed").
- 429 waits out a short Retry-After and retries, otherwise reports retryable.
- 5xx and network errors retry twice, then report retryable.
- Other 4xx are permanent and the response excerpt is shown with the credential redacted.

Build and sign locally with `scripts/plugins-build.sh all mailgun`. Tests: `go test ./plugins/mailgun`.
