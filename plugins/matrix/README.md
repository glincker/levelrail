# Matrix plugin

Posts each alert to a Matrix room with a bot access token. A retried send reuses its transaction id, so a room never gets a duplicate.

Kind `notification_channel`, ability `read`, outbound host: taken from the `homeserver` config value.

## Config

Plugin-wide, non-secret, passed at install with `--config`:

```json
{"homeserver": "https://matrix.example.com", "room_id": "!abc123:example.com"}
```

| Field | Required | Meaning |
| --- | --- | --- |
| `homeserver` | yes | Homeserver base URL, https only |
| `room_id` | yes | Internal room id starting with `!` |
| `allow_http` | no | Permit plain http for a lab server |

## Channel target

The channel's target field holds the bot's access token. It is sealed like any channel credential and sent per call. The plugin never stores or logs it.

```bash
levelrail-cli plugins install --manifest plugin.yaml --signature plugin.sig --config config.json --approve read
levelrail-cli channels create --name ops-matrix --kind plugin:matrix --notify-url syt_ACCESS_TOKEN
```

## Behaviour

- 401 and 403 are permanent ("authentication failed").
- 429 waits out a short Retry-After and retries, otherwise reports retryable.
- 5xx and network errors retry twice, then report retryable.
- Other 4xx are permanent and the response excerpt is shown with the credential redacted.

Build and sign locally with `scripts/plugins-build.sh all matrix`. Tests: `go test ./plugins/matrix`.
