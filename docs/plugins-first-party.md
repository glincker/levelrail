---
title: "First-party plugins"
tag: "Experimental"
description: Notification channel plugins for services the built-in channels do not cover, built on the same public contract as any third-party plugin.
---

These plugins add alert channels for services the built-in channels do not cover. Each is a small out-of-process program on the public [plugin contract](/plugins): it runs in a locked-down container, receives only the event and its own config, and gets the credential per call. They have no private channel to the control plane.

Already built in, so no plugin exists for them: Slack, Discord, Telegram, Teams, PagerDuty, Opsgenie, ntfy, Pushover, Gotify, Mattermost, Rocket.Chat, Google Chat, Webex, Lark, email, Resend and generic webhooks. The built-in email channel sends through the control plane's own SMTP settings, so a per-channel SMTP relay is not offered as a plugin.

Plugins need `APP_EXPERIMENTAL=plugins` (see [plugins](/plugins)).

## The plugins

| Plugin | Delivers to | Channel target (the credential) | Outbound hosts |
| --- | --- | --- | --- |
| `zulip` | A Zulip stream topic | `<bot email>:<api key>` | your server (see below) |
| `matrix` | A Matrix room | Bot access token | your homeserver (see below) |
| `twilio-sms` | SMS to one number | Twilio auth token | `api.twilio.com` |
| `dingtalk` | A DingTalk group robot | Robot webhook URL | `oapi.dingtalk.com` |
| `wecom` | A WeCom group robot | Robot webhook URL | `qyapi.weixin.qq.com` |
| `mailgun` | Email through the Mailgun API | Mailgun API key | `api.mailgun.net`, `api.eu.mailgun.net` |

Brand marks: the manifest has no icon field yet, so the dashboard shows the monogram tile for these. A thesvg mark can be wired when the marketplace listing carries one.

## Config fields

Config is plugin-wide and non-secret, given at install with `--config <file>`. Anything secret goes in the channel target instead.

| Plugin | Config |
| --- | --- |
| `zulip` | `base_url`, `stream` (required); `topic` (default `alerts`); `allow_http` |
| `matrix` | `homeserver`, `room_id` (required, the internal `!id:server` form); `allow_http` |
| `twilio-sms` | `account_sid`, `to` (required); `from` or `messaging_service_sid` |
| `dingtalk` | `keyword` (optional, prefixed to each message when the robot requires one) |
| `wecom` | none |
| `mailgun` | `domain`, `from`, `to` (required); `region` (`us` or `eu`) |

`allow_http` is for a lab server on plain http. Without it, any non-https address is refused. An address with embedded credentials is always refused.

## Use one

Build, sign and trust (for a package you built yourself), install, then add a channel:

```bash
scripts/plugins-build.sh all twilio-sms
levelrail-cli plugins trust add --name "Local test" --public-key "$(cat dist/plugins/publisher.pub)"
echo '{"account_sid":"ACXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX","from":"+15550001111","to":"+15552223333"}' > twilio.json
levelrail-cli plugins install --manifest dist/plugins/twilio-sms/plugin.yaml \
  --signature dist/plugins/twilio-sms/plugin.sig --config twilio.json --approve read
levelrail-cli plugins enable twilio-sms
levelrail-cli channels create --name oncall-sms --kind plugin:twilio-sms --notify-url AUTH_TOKEN
```

Alert rules then deliver through `plugin:twilio-sms` like any other channel, with the same delivery history.

## DNS provider plugins

`cloudflare-dns` is the first plugin of kind `dns_provider`: it lists zones and creates, replaces, deletes and verifies records in Cloudflare. It needs no config and no credential of its own. The control plane hands it the Cloudflare DNS token you already stored under Settings on every call, and the plugin never keeps or logs it. Outbound host: `api.cloudflare.com`.

By default the built-in client writes DNS records. To use the plugin instead, build, install, approve and enable it, then:

- Plugins page: on the plugin's row, press Test connection (it lists your zones with the stored token), then Use for DNS records.
- Domains page: pick the plugin under DNS record writer in the domain automation card.
- CLI: `levelrail-cli plugins test cloudflare-dns`, then `levelrail-cli plugins dns-provider cloudflare-dns`. `plugins dns-provider --off` returns to the built-in client, and with no argument it shows the current choice.
- API: `POST /api/v1/plugins/{name}/test` and `GET` or `PUT /api/v1/settings/dns-provider-plugin` (writes need root).

Then the domain Fix button, go-live, add-domain automation and the records editor all write through the plugin. There is no automatic fallback: if the plugin is down or paused, the fix reports it, so you always know which code wrote a record. Every write is verified afterwards, and a record the plugin accepted but cannot see is reported as unconfirmed and stays undoable. The Cloudflare proxy toggle is not applied to records the plugin writes yet. See ADR 038.

## Self-hosted services

Zulip and Matrix run on your own server, so the shipped manifest lists a placeholder host (`zulip.example.com`, `matrix.example.com`). Hosts a plugin declares are shown at approval but are not yet enforced per host (see [plugins](/plugins)), so the placeholder gives the plugin network access and the plugin itself only ever calls the `base_url` or `homeserver` you configure. To declare your real host, copy the manifest, change `outbound_hosts`, sign it with your own key and install that copy. The image digest stays the same.

## Failure handling

Every plugin maps service answers the same way: 401 and 403 are permanent authentication failures, 429 waits out a short Retry-After and retries (a longer wait is reported as retryable), 5xx and network errors retry twice and then report retryable, and other 4xx are permanent with a short response excerpt. The credential is redacted from every message. Services that answer HTTP 200 with an error code in the body (DingTalk, WeCom) are treated as failures too.

## Building and signing

`scripts/plugins-build.sh build|sign|all [name...]` compiles each plugin for Linux, builds an image named `<registry>/levelrail-plugin-<name>:local` and writes `dist/plugins/<name>/plugin.yaml` (digest pinned) and `plugin.sig`. Without `PLUGINS_KEY` it signs with a throwaway key that exists for local tests only. It never pushes an image. Publishing is the maintainer's step: push each image, set `PLUGINS_DIGEST_<NAME>` (or write `dist/plugins/<name>/digest`) to the registry digest, and run `sign` with `PLUGINS_KEY` pointing at the real publisher key.

## Not yet

- Plugin icons and a marketplace listing for these plugins.
- DingTalk signature security and Matrix end-to-end encrypted rooms.
