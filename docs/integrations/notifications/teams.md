---
title: "Use Microsoft Teams with Levelrail: alerts"
sidebarTitle: "Microsoft Teams"
description: "Add Microsoft Teams as a notification channel so alert rules and deploy events reach it."
---

Add Microsoft Teams as a notification channel so alert rules and deploy events reach it.

This page covers how to send alerts and deploy notices to Microsoft Teams.

## What you get

- Alert rule notifications and deploy events delivered to Microsoft Teams.
- A test send and a delivery history for the channel.
- The destination encrypted at rest, with only a masked placeholder in the alert store.

## Set up

1. Prepare the credential. The destination itself is the credential: A Teams incoming webhook URL.
2. In the dashboard, open **Settings > Notification channels** (`/settings/notification-channels`), then add the Microsoft Teams channel.
3. Send a test message from the channel row or with `channels test`, then check `channels deliveries` for the result.

From the CLI:

```bash
levelrail-cli channels create --name teams-alerts --kind teams --notify-url <URL>
levelrail-cli channels test <channel-id>
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| Destination | A Teams incoming webhook URL. |

## Endpoint and format

Channel kind `teams`. Destination: A Teams incoming webhook URL.

## Security notes

- The destination often is the credential, so it is envelope encrypted under the master key and the alert store keeps only a masked placeholder.
- Every send passes through an SSRF guard that rejects loopback, private and link-local targets unless you allow private networks.
- Nothing you enter here is shown again after you save it.

## Limits and caveats

- Delivery attempts are recorded and can be listed with `channels deliveries`.

## Troubleshooting

**Microsoft Teams rejects the message.** Create an incoming webhook in the Teams channel and paste its URL.

**Saving the channel returns 400 for the URL.** A destination that is not an absolute http or https URL, or that points at loopback, a private range or link-local, is refused. A self-hosted target on a private network needs APP_NOTIFY_ALLOW_PRIVATE_NETWORKS=true set before you create the channel.

**Create or test says the channel is not configured.** Channel credentials are encrypted under the master key, so the control plane needs one configured.

## FAQ

**Where do I get the destination for Microsoft Teams?**

Create an incoming webhook in the Teams channel and paste its URL.

**Can one channel serve many alert rules?**

Yes. Rules reference channels, so one Microsoft Teams channel can receive any number of rules.

## Related

- [Slack](/integrations/notifications/slack): Notification channels
- [Webex](/integrations/notifications/webex): Notification channels
- [All notification channels](/integrations/notifications)
- [Microsoft Teams documentation](https://learn.microsoft.com/microsoftteams/platform/webhooks-and-connectors/), maintained by the vendor
