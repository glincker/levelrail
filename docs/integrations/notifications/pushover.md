---
title: "Use Pushover with Levelrail: alerts"
sidebarTitle: "Pushover"
description: "Add Pushover as a notification channel so alert rules and deploy events reach it."
---

Add Pushover as a notification channel so alert rules and deploy events reach it.

This page covers how to send alerts and deploy notices to Pushover.

## What you get

- Alert rule notifications and deploy events delivered to Pushover.
- A test send and a delivery history for the channel.
- The destination encrypted at rest, with only a masked placeholder in the alert store.

## Set up

1. Prepare the credential. Two values: the Pushover user key and the application API token.
2. In the dashboard, open **Settings > Notification channels** (`/settings/notification-channels`), then add the Pushover channel.
3. Send a test message from the channel row or with `channels test`, then check `channels deliveries` for the result.

From the CLI:

```bash
levelrail-cli channels create --name phone --kind pushover --pushover-user-key <USER_KEY> --pushover-api-token <APP_TOKEN>
levelrail-cli channels test <channel-id>
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| Destination | A Pushover user key and application API token. |

## Endpoint and format

Channel kind `pushover`. Destination: A Pushover user key and application API token.

## Security notes

- The destination often is the credential, so it is envelope encrypted under the master key and the alert store keeps only a masked placeholder.
- Every send passes through an SSRF guard that rejects loopback, private and link-local targets unless you allow private networks.
- Nothing you enter here is shown again after you save it.

## Limits and caveats

- Delivery attempts are recorded and can be listed with `channels deliveries`.

## Troubleshooting

**Pushover rejects the message.** Create an application in Pushover for the API token, and copy your user key from the dashboard.

**Create or test says the channel is not configured.** Channel credentials are encrypted under the master key, so the control plane needs one configured.

**Alerts fire but nothing arrives.** Run `levelrail-cli channels test <id>` for a real test message and `channels deliveries <id>` to see the recorded result.

## FAQ

**Where do I get the destination for Pushover?**

Create an application in Pushover for the API token, and copy your user key from the dashboard.

**Can one channel serve many alert rules?**

Yes. Rules reference channels, so one Pushover channel can receive any number of rules.

## Related

- [ntfy](/integrations/notifications/ntfy): Notification channels
- [Telegram](/integrations/notifications/telegram): Notification channels
- [All notification channels](/integrations/notifications)
- [Pushover documentation](https://pushover.net/api), maintained by the vendor
