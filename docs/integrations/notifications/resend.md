---
title: "Use Resend with Levelrail: alerts"
sidebarTitle: "Resend"
description: "Add Resend as a notification channel so alert rules and deploy events reach it."
---

Add Resend as a notification channel so alert rules and deploy events reach it.

This page covers how to send alerts and deploy notices to Resend.

## What you get

- Alert rule notifications and deploy events delivered to Resend.
- A test send and a delivery history for the channel.
- The destination encrypted at rest, with only a masked placeholder in the alert store.

## Set up

1. Prepare the credential. A Resend API key. The destination and from addresses are stored with the channel.
2. In the dashboard, open **Settings > Notification channels** (`/settings/notification-channels`), then add the Resend channel.
3. Send a test message from the channel row or with `channels test`, then check `channels deliveries` for the result.

From the CLI:

```bash
levelrail-cli channels create --name mail --kind resend --resend-api-key <API_KEY> --resend-to oncall@example.com --resend-from alerts@example.com
levelrail-cli channels test <channel-id>
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| Destination | A Resend API key, a destination address and an optional from address. |

## Endpoint and format

Channel kind `resend`. Destination: A Resend API key, a destination address and an optional from address.

## Security notes

- The destination often is the credential, so it is envelope encrypted under the master key and the alert store keeps only a masked placeholder.
- Every send passes through an SSRF guard that rejects loopback, private and link-local targets unless you allow private networks.
- Nothing you enter here is shown again after you save it.

## Limits and caveats

- --resend-from is optional and defaults to onboarding@resend.dev.
- Delivery attempts are recorded and can be listed with `channels deliveries`.

## Troubleshooting

**Resend rejects the message.** Create an API key in Resend and use a from address on a domain you verified there.

**Create or test says the channel is not configured.** Channel credentials are encrypted under the master key, so the control plane needs one configured.

**Alerts fire but nothing arrives.** Run `levelrail-cli channels test <id>` for a real test message and `channels deliveries <id>` to see the recorded result.

## FAQ

**Where do I get the destination for Resend?**

Create an API key in Resend and use a from address on a domain you verified there.

**Can one channel serve many alert rules?**

Yes. Rules reference channels, so one Resend channel can receive any number of rules.

## Related

- [Email](/integrations/notifications/email): Notification channels
- [Slack](/integrations/notifications/slack): Notification channels
- [All notification channels](/integrations/notifications)
- [Resend documentation](https://resend.com/docs), maintained by the vendor
