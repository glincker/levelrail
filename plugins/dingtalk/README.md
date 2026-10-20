# DingTalk plugin

Delivers each alert to a DingTalk group through a custom robot.

Kind `notification_channel`, ability `read`, outbound hosts: `oapi.dingtalk.com`.

## Config

Plugin-wide, non-secret, passed at install with `--config`:

```json
{"keyword": "ALERT"}
```

| Field | Required | Meaning |
| --- | --- | --- |
| `keyword` | no | Custom keyword the robot requires, prefixed to every message. Signature security is not supported, use keyword or IP allowlist security |

## Channel target

The channel's target field holds the robot webhook URL (`https://oapi.dingtalk.com/robot/send?access_token=...`). It is sealed like any channel credential and sent per call. The plugin never stores or logs it.

```bash
levelrail-cli plugins install --manifest plugin.yaml --signature plugin.sig --config config.json --approve read
levelrail-cli channels create --name ops-dingtalk --kind plugin:dingtalk --notify-url 'https://oapi.dingtalk.com/robot/send?access_token=TOKEN'
```

## Behaviour

- 401 and 403 are permanent ("authentication failed").
- 429 waits out a short Retry-After and retries, otherwise reports retryable.
- 5xx and network errors retry twice, then report retryable.
- Other 4xx are permanent and the response excerpt is shown with the credential redacted.

Build and sign locally with `scripts/plugins-build.sh all dingtalk`. Tests: `go test ./plugins/dingtalk`.
