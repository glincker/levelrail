---
title: "Use Browser push with Levelrail: alerts"
sidebarTitle: "Browser push"
description: "Deliver alerts to browsers that subscribed to the dashboard, with no third party service involved."
---

Deliver alerts to browsers that subscribed to the dashboard, with no third party service involved.

This page covers how to send alerts as browser push notifications.

## What you get

- Alerts delivered to every browser subscription registered on the control plane.
- No webhook URL or API key to manage.
- Same alert rules and test send as other channels.

## Set up

1. Prepare the credential. None in the channel. The destination is every browser subscription registered under Browser push.
2. In the dashboard, open **Settings > Notification channels** (`/settings/notification-channels`), then open Browser push.
3. Send a test message from the channel row or with `channels test`, then check `channels deliveries` for the result.

From the CLI:

```bash
levelrail-cli channels list
levelrail-cli channels test <channel-id>
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| Browser permission | Each person grants notification permission in their own browser. |

## Endpoint and format

Channel kind `webpush`. The notify URL is ignored.

## Security notes

- The destination often is the credential, so it is envelope encrypted under the master key and the alert store keeps only a masked placeholder.
- Every send passes through an SSRF guard that rejects loopback, private and link-local targets unless you allow private networks.
- Nothing you enter here is shown again after you save it.

## Limits and caveats

- Subscriptions are created in the dashboard, so the CLI create command's kind list does not include it.
- Needs a master key on the control plane, like every channel.

## Troubleshooting

**No subscription is listed.** Open the dashboard in the target browser and subscribe under Browser push; the browser asks for permission.

**Create says not configured.** The control plane has no master key set.

**Notifications stop on one device.** The browser dropped its subscription, for example after clearing site data. Subscribe again.

## FAQ

**Does it need an external service?**

No third party account is needed on the Levelrail side, only the browser's own push service.

**Can I choose which devices get alerts?**

Every registered subscription receives them. Remove a subscription to stop a device.

## Related

- [Email](/integrations/notifications/email): Notification channels
- [Slack](/integrations/notifications/slack): Notification channels
- [All notification channels](/integrations/notifications)
- [Browser push documentation](https://developer.mozilla.org/docs/Web/API/Push_API), maintained by the vendor
