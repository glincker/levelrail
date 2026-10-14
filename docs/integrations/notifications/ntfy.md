---
title: "Use ntfy with Levelrail: alerts"
sidebarTitle: "ntfy"
description: "Add ntfy as a notification channel so alert rules and deploy events reach it."
---

Add ntfy as a notification channel so alert rules and deploy events reach it.

This page covers how to send alerts and deploy notices to ntfy.

## What you get

- Alert rule notifications and deploy events delivered to ntfy.
- A test send and a delivery history for the channel.
- The destination encrypted at rest, with only a masked placeholder in the alert store.

## Set up

1. Prepare the credential. The destination itself is the credential: An ntfy topic URL.
2. In the dashboard, open **Settings > Notification channels** (`/settings/notification-channels`), then add the ntfy channel.
3. Send a test message from the channel row or with `channels test`, then check `channels deliveries` for the result.

From the CLI:

```bash
levelrail-cli channels create --name ntfy-alerts --kind ntfy --notify-url <URL>
levelrail-cli channels test <channel-id>
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| Destination | An ntfy topic URL. |

## Endpoint and format

Channel kind `ntfy`. Destination: An ntfy topic URL.

## Security notes

- The destination often is the credential, so it is envelope encrypted under the master key and the alert store keeps only a masked placeholder.
- Every send passes through an SSRF guard that rejects loopback, private and link-local targets unless you allow private networks.
- Nothing you enter here is shown again after you save it.

## Limits and caveats

- A self-hosted ntfy on a private network needs APP_NOTIFY_ALLOW_PRIVATE_NETWORKS=true.
- Delivery attempts are recorded and can be listed with `channels deliveries`.

## Troubleshooting

**ntfy rejects the message.** Use the full topic URL, on ntfy.sh or your own server.

**Saving the channel returns 400 for the URL.** A destination that is not an absolute http or https URL, or that points at loopback, a private range or link-local, is refused. A self-hosted target on a private network needs APP_NOTIFY_ALLOW_PRIVATE_NETWORKS=true set before you create the channel.

**Create or test says the channel is not configured.** Channel credentials are encrypted under the master key, so the control plane needs one configured.

## FAQ

**Where do I get the destination for ntfy?**

Use the full topic URL, on ntfy.sh or your own server.

**Can one channel serve many alert rules?**

Yes. Rules reference channels, so one ntfy channel can receive any number of rules.

## Related

- [Gotify](/integrations/notifications/gotify): Notification channels
- [Pushover](/integrations/notifications/pushover): Notification channels
- [All notification channels](/integrations/notifications)
- [ntfy documentation](https://docs.ntfy.sh/), maintained by the vendor
