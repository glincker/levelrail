# Chat bot

Relays a webhook to a chat channel. Post `{"text": "...", "user": "optional"}` with a shared token and the message lands in Slack, Discord or Telegram, whichever you configured. A repeated `X-Delivery-Id` is relayed once (remembered in the platform key-value store).

## Environment

| Variable | Required | Meaning |
| --- | --- | --- |
| `KV_URL`, `KV_TOKEN` | yes | Injected when you enable the key-value store for the function. |
| `WEBHOOK_TOKEN` | yes | Value callers send in `X-Webhook-Token`. Every call is refused while it is unset. |
| `SLACK_WEBHOOK_URL` | one channel | Slack incoming webhook. |
| `DISCORD_WEBHOOK_URL` | one channel | Discord channel webhook. |
| `TELEGRAM_BOT_TOKEN`, `TELEGRAM_CHAT_ID` | one channel | Telegram bot and target chat; both are needed. |
| `DEDUPE_TTL_SECONDS` | no | How long a delivery id is remembered. Default 3600. |

Set tokens as secrets (`--env` values are stored as plain env; use the Environment tab or `apps secrets set` for the tokens).

```
curl -X POST https://<function>/ -H "X-Webhook-Token: $WEBHOOK_TOKEN" \
  -H "Content-Type: application/json" -d '{"text":"build green","user":"ci"}'
```

## Test

```
node --test
```
