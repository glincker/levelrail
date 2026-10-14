---
title: "Use Webex with Levelrail: alerts"
sidebarTitle: "Webex"
description: "Add Webex as a notification channel so alert rules and deploy events reach it."
---

Add Webex as a notification channel so alert rules and deploy events reach it.

This page covers how to send alerts and deploy notices to Webex.

## What you get

- Alert rule notifications and deploy events delivered to Webex.
- A test send and a delivery history for the channel.
- The destination encrypted at rest, with only a masked placeholder in the alert store.

## Set up

1. Prepare the credential. The destination itself is the credential: A Webex webhook URL.
2. In the dashboard, open **Settings > Notification channels** (`/settings/notification-channels`), then add the Webex channel.
3. Send a test message from the channel row or with `channels test`, then check `channels deliveries` for the result.

From the CLI:

```bash
levelrail-cli channels create --name webex-alerts --kind webex --notify-url <URL>
levelrail-cli channels test <channel-id>
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| Destination | A Webex webhook URL. |

## Endpoint and format

Channel kind `webex`. Destination: A Webex webhook URL.

## Security notes

- The destination often is the credential, so it is envelope encrypted under the master key and the alert store keeps only a masked placeholder.
- Every send passes through an SSRF guard that rejects loopback, private and link-local targets unless you allow private networks.
- Nothing you enter here is shown again after you save it.

## Limits and caveats

- Delivery attempts are recorded and can be listed with `channels deliveries`.

## Troubleshooting

**Webex rejects the message.** Create an incoming webhook for the Webex space and paste its URL.

**Saving the channel returns 400 for the URL.** A destination that is not an absolute http or https URL, or that points at loopback, a private range or link-local, is refused. A self-hosted target on a private network needs APP_NOTIFY_ALLOW_PRIVATE_NETWORKS=true set before you create the channel.

**Create or test says the channel is not configured.** Channel credentials are encrypted under the master key, so the control plane needs one configured.

## FAQ

**Where do I get the destination for Webex?**

Create an incoming webhook for the Webex space and paste its URL.

**Can one channel serve many alert rules?**

Yes. Rules reference channels, so one Webex channel can receive any number of rules.

## Related

- [Microsoft Teams](/integrations/notifications/teams): Notification channels
- [Google Chat](/integrations/notifications/googlechat): Notification channels
- [All notification channels](/integrations/notifications)
- [Webex documentation](https://developer.webex.com/docs), maintained by the vendor
