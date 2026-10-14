---
title: "Use Generic webhook with Levelrail: alerts"
sidebarTitle: "Generic webhook"
description: "Add Generic webhook as a notification channel so alert rules and deploy events reach it."
---

Add Generic webhook as a notification channel so alert rules and deploy events reach it.

This page covers how to send alerts and deploy notices to Generic webhook.

## What you get

- Alert rule notifications and deploy events delivered to Generic webhook.
- A test send and a delivery history for the channel.
- The destination encrypted at rest, with only a masked placeholder in the alert store.
- Extra recent_changes and changes_link fields for deploy and change context.

## Set up

1. Prepare the credential. The destination itself is the credential: An http or https URL that accepts a JSON POST.
2. In the dashboard, open **Settings > Notification channels** (`/settings/notification-channels`), then add the Generic webhook channel.
3. Send a test message from the channel row or with `channels test`, then check `channels deliveries` for the result.

From the CLI:

```bash
levelrail-cli channels create --name generic-alerts --kind generic --notify-url <URL>
levelrail-cli channels test <channel-id>
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| Destination | An http or https URL that accepts a JSON POST. |

## Endpoint and format

Channel kind `generic`. Destination: An http or https URL that accepts a JSON POST.

## Security notes

- The destination often is the credential, so it is envelope encrypted under the master key and the alert store keeps only a masked placeholder.
- Every send passes through an SSRF guard that rejects loopback, private and link-local targets unless you allow private networks.
- Nothing you enter here is shown again after you save it.

## Limits and caveats

- Delivery attempts are recorded and can be listed with `channels deliveries`.

## Troubleshooting

**Generic webhook rejects the message.** Point it at a receiver you control. The payload adds recent_changes and changes_link fields to the standard alert body.

**Saving the channel returns 400 for the URL.** A destination that is not an absolute http or https URL, or that points at loopback, a private range or link-local, is refused. A self-hosted target on a private network needs APP_NOTIFY_ALLOW_PRIVATE_NETWORKS=true set before you create the channel.

**Create or test says the channel is not configured.** Channel credentials are encrypted under the master key, so the control plane needs one configured.

## FAQ

**Where do I get the destination for Generic webhook?**

Point it at a receiver you control. The payload adds recent_changes and changes_link fields to the standard alert body.

**Can one channel serve many alert rules?**

Yes. Rules reference channels, so one Generic webhook channel can receive any number of rules.

## Related

- [Slack](/integrations/notifications/slack): Notification channels
- [ntfy](/integrations/notifications/ntfy): Notification channels
- [All notification channels](/integrations/notifications)
- [Generic webhook documentation](https://www.rfc-editor.org/rfc/rfc9110), maintained by the vendor
