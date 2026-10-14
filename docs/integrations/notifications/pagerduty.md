---
title: "Use PagerDuty with Levelrail: alerts"
sidebarTitle: "PagerDuty"
description: "Add PagerDuty as a notification channel so alert rules and deploy events reach it."
---

Add PagerDuty as a notification channel so alert rules and deploy events reach it.

This page covers how to send alerts and deploy notices to PagerDuty.

## What you get

- Alert rule notifications and deploy events delivered to PagerDuty.
- A test send and a delivery history for the channel.
- The destination encrypted at rest, with only a masked placeholder in the alert store.

## Set up

1. Prepare the credential. The Events API v2 routing key. It holds an address or key, not a URL, so the private network URL check does not apply.
2. In the dashboard, open **Settings > Notification channels** (`/settings/notification-channels`), then add the PagerDuty channel.
3. Send a test message from the channel row or with `channels test`, then check `channels deliveries` for the result.

From the CLI:

```bash
levelrail-cli channels create --name oncall --kind pagerduty --pagerduty-routing-key <ROUTING_KEY>
levelrail-cli channels test <channel-id>
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| Destination | A PagerDuty Events API v2 integration (routing) key. |

## Endpoint and format

Channel kind `pagerduty`. Destination: A PagerDuty Events API v2 integration (routing) key.

## Security notes

- The destination often is the credential, so it is envelope encrypted under the master key and the alert store keeps only a masked placeholder.
- Every send passes through an SSRF guard that rejects loopback, private and link-local targets unless you allow private networks.
- Nothing you enter here is shown again after you save it.

## Limits and caveats

- Messages are capped to PagerDuty's receiver limits, with the recent changes section kept.
- Delivery attempts are recorded and can be listed with `channels deliveries`.

## Troubleshooting

**PagerDuty rejects the message.** Add an Events API v2 integration to a PagerDuty service and use its integration key.

**Create or test says the channel is not configured.** Channel credentials are encrypted under the master key, so the control plane needs one configured.

**Alerts fire but nothing arrives.** Run `levelrail-cli channels test <id>` for a real test message and `channels deliveries <id>` to see the recorded result.

## FAQ

**Where do I get the destination for PagerDuty?**

Add an Events API v2 integration to a PagerDuty service and use its integration key.

**Can one channel serve many alert rules?**

Yes. Rules reference channels, so one PagerDuty channel can receive any number of rules.

## Related

- [Opsgenie](/integrations/notifications/opsgenie): Notification channels
- [Slack](/integrations/notifications/slack): Notification channels
- [All notification channels](/integrations/notifications)
- [PagerDuty documentation](https://developer.pagerduty.com/docs/events-api-v2-overview), maintained by the vendor
