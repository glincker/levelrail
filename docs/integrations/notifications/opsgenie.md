---
title: "Use Opsgenie with Levelrail: alerts"
sidebarTitle: "Opsgenie"
description: "Add Opsgenie as a notification channel so alert rules and deploy events reach it."
---

Add Opsgenie as a notification channel so alert rules and deploy events reach it.

This page covers how to send alerts and deploy notices to Opsgenie.

## What you get

- Alert rule notifications and deploy events delivered to Opsgenie.
- A test send and a delivery history for the channel.
- The destination encrypted at rest, with only a masked placeholder in the alert store.

## Set up

1. Prepare the credential. An Opsgenie API key. It holds a key rather than a URL, so the private network URL check does not apply.
2. In the dashboard, open **Settings > Notification channels** (`/settings/notification-channels`), then add the Opsgenie channel.
3. Send a test message from the channel row or with `channels test`, then check `channels deliveries` for the result.

From the CLI:

```bash
levelrail-cli channels create --name oncall --kind opsgenie --opsgenie-api-key <API_KEY>
levelrail-cli channels test <channel-id>
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| Destination | An Opsgenie API key. |

## Endpoint and format

Channel kind `opsgenie`. Destination: An Opsgenie API key.

## Security notes

- The destination often is the credential, so it is envelope encrypted under the master key and the alert store keeps only a masked placeholder.
- Every send passes through an SSRF guard that rejects loopback, private and link-local targets unless you allow private networks.
- Nothing you enter here is shown again after you save it.

## Limits and caveats

- Delivery attempts are recorded and can be listed with `channels deliveries`.

## Troubleshooting

**Opsgenie rejects the message.** Create an API integration in Opsgenie and use its API key.

**Create or test says the channel is not configured.** Channel credentials are encrypted under the master key, so the control plane needs one configured.

**Alerts fire but nothing arrives.** Run `levelrail-cli channels test <id>` for a real test message and `channels deliveries <id>` to see the recorded result.

## FAQ

**Where do I get the destination for Opsgenie?**

Create an API integration in Opsgenie and use its API key.

**Can one channel serve many alert rules?**

Yes. Rules reference channels, so one Opsgenie channel can receive any number of rules.

## Related

- [PagerDuty](/integrations/notifications/pagerduty): Notification channels
- [Slack](/integrations/notifications/slack): Notification channels
- [All notification channels](/integrations/notifications)
- [Opsgenie documentation](https://support.atlassian.com/opsgenie/), maintained by the vendor
