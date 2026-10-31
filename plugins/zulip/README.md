# Zulip plugin

Posts each alert to a Zulip stream topic as a bot.

Kind `notification_channel`, ability `read`, outbound host: taken from the `base_url` config value.

## Config

Plugin-wide, non-secret, passed at install with `--config`:

```json
{"base_url": "https://zulip.example.com", "stream": "ops", "topic": "alerts"}
```

| Field | Required | Meaning |
| --- | --- | --- |
| `base_url` | yes | Your Zulip server, https only |
| `stream` | yes | Stream name |
| `topic` | no | Topic, default `alerts` |
| `allow_http` | no | Permit plain http for a lab server |

## Channel target

The channel's target field holds `<bot email>:<api key>` (from the bot's settings in Zulip). It is sealed like any channel credential and sent per call. The plugin never stores or logs it.

```bash
levelrail-cli plugins install --manifest plugin.yaml --signature plugin.sig --config config.json --approve read
levelrail-cli channels create --name ops-zulip --kind plugin:zulip --notify-url bot@zulip.example.com:API_KEY
```

## Behaviour

- 401 and 403 are permanent ("authentication failed").
- 429 waits out a short Retry-After and retries, otherwise reports retryable.
- 5xx and network errors retry twice, then report retryable.
- Other 4xx are permanent and the response excerpt is shown with the credential redacted.

Build and sign locally with `scripts/plugins-build.sh all zulip`. Tests: `go test ./plugins/zulip`.
