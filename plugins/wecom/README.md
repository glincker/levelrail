# WeCom plugin

Delivers each alert to a WeCom (WeChat Work) group through a group robot.

Kind `notification_channel`, ability `read`, outbound hosts: `qyapi.weixin.qq.com`.

## Config

Plugin-wide, non-secret, passed at install with `--config`:

```json
{}
```

This plugin takes no config.

## Channel target

The channel's target field holds the robot webhook URL (`https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=...`). It is sealed like any channel credential and sent per call. The plugin never stores or logs it.

```bash
levelrail-cli plugins install --manifest plugin.yaml --signature plugin.sig --config config.json --approve read
levelrail-cli channels create --name ops-wecom --kind plugin:wecom --notify-url 'https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=KEY'
```

## Behaviour

- 401 and 403 are permanent ("authentication failed").
- 429 waits out a short Retry-After and retries, otherwise reports retryable.
- 5xx and network errors retry twice, then report retryable.
- Other 4xx are permanent and the response excerpt is shown with the credential redacted.

Build and sign locally with `scripts/plugins-build.sh all wecom`. Tests: `go test ./plugins/wecom`.
