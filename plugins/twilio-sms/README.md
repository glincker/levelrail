# Twilio SMS plugin

Sends a short SMS (320 characters at most) for each alert.

Kind `notification_channel`, ability `read`, outbound hosts: `api.twilio.com`.

## Config

Plugin-wide, non-secret, passed at install with `--config`:

```json
{"account_sid": "ACXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX", "from": "+15550001111", "to": "+15552223333"}
```

| Field | Required | Meaning |
| --- | --- | --- |
| `account_sid` | yes | Account SID (`AC` plus 32 hex) |
| `to` | yes | Recipient in E.164 |
| `from` | one of | Sender number in E.164 |
| `messaging_service_sid` | one of | Messaging service SID (`MG` plus 32 hex), instead of `from` |

## Channel target

The channel's target field holds the Twilio auth token. It is sealed like any channel credential and sent per call. The plugin never stores or logs it.

```bash
levelrail-cli plugins install --manifest plugin.yaml --signature plugin.sig --config config.json --approve read
levelrail-cli channels create --name ops-twilio-sms --kind plugin:twilio-sms --notify-url AUTH_TOKEN
```

## Behaviour

- 401 and 403 are permanent ("authentication failed").
- 429 waits out a short Retry-After and retries, otherwise reports retryable.
- 5xx and network errors retry twice, then report retryable.
- Other 4xx are permanent and the response excerpt is shown with the credential redacted.

Build and sign locally with `scripts/plugins-build.sh all twilio-sms`. Tests: `go test ./plugins/twilio-sms`.
