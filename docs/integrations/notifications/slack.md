---
title: "Use Slack with Levelrail: alerts"
sidebarTitle: "Slack"
description: "Add Slack as a notification channel so alert rules and deploy events reach it."
---

Add Slack as a notification channel so alert rules and deploy events reach it.

This page covers how to send alerts and deploy notices to Slack.

## What you get

- Alert rule notifications and deploy events delivered to Slack.
- A test send and a delivery history for the channel.
- The destination encrypted at rest, with only a masked placeholder in the alert store.

## Set up

1. Prepare the credential. The destination itself is the credential: A Slack incoming webhook URL.
2. In the dashboard, open **Settings > Notification channels** (`/settings/notification-channels`), then add the Slack channel.
3. Send a test message from the channel row or with `channels test`, then check `channels deliveries` for the result.

From the CLI:

```bash
levelrail-cli channels create --name slack-alerts --kind slack --notify-url <URL>
levelrail-cli channels test <channel-id>
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| Destination | A Slack incoming webhook URL. |

## Endpoint and format

Channel kind `slack`. Destination: A Slack incoming webhook URL.

## Security notes

- The destination often is the credential, so it is envelope encrypted under the master key and the alert store keeps only a masked placeholder.
- Every send passes through an SSRF guard that rejects loopback, private and link-local targets unless you allow private networks.
- Nothing you enter here is shown again after you save it.

## Limits and caveats

- Delivery attempts are recorded and can be listed with `channels deliveries`.

## Troubleshooting

**Slack rejects the message.** Create an incoming webhook for the channel in your Slack app and paste its URL.

**Saving the channel returns 400 for the URL.** A destination that is not an absolute http or https URL, or that points at loopback, a private range or link-local, is refused. A self-hosted target on a private network needs APP_NOTIFY_ALLOW_PRIVATE_NETWORKS=true set before you create the channel.

**Create or test says the channel is not configured.** Channel credentials are encrypted under the master key, so the control plane needs one configured.

## FAQ

**Where do I get the destination for Slack?**

Create an incoming webhook for the channel in your Slack app and paste its URL.

**Can one channel serve many alert rules?**

Yes. Rules reference channels, so one Slack channel can receive any number of rules.

## Related

- [Discord](/integrations/notifications/discord): Notification channels
- [Slack](/integrations/sign-in/slack): Sign-in providers
- [All notification channels](/integrations/notifications)
- [Slack documentation](https://api.slack.com/messaging/webhooks), maintained by the vendor
