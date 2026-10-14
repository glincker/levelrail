---
title: "Use Email with Levelrail: alerts"
sidebarTitle: "Email"
description: "Add Email as a notification channel so alert rules and deploy events reach it."
---

Add Email as a notification channel so alert rules and deploy events reach it.

This page covers how to send alerts and deploy notices to Email.

## What you get

- Alert rule notifications and deploy events delivered to Email.
- A test send and a delivery history for the channel.
- The destination encrypted at rest, with only a masked placeholder in the alert store.

## Set up

1. Prepare the credential. No secret in the channel. The sending backend holds the SMTP or API credentials.
2. In the dashboard, open **Settings > Notification channels** (`/settings/notification-channels`), then add the Email channel.
3. Send a test message from the channel row or with `channels test`, then check `channels deliveries` for the result.

From the CLI:

```bash
levelrail-cli channels create --name oncall-email --kind email --notify-url oncall@example.com
levelrail-cli channels test <channel-id>
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| Destination | An email address, sent through the control plane's mail backend. |

## Endpoint and format

Channel kind `email`. Destination: An email address, sent through the control plane's mail backend.

## Security notes

- The destination often is the credential, so it is envelope encrypted under the master key and the alert store keeps only a masked placeholder.
- Every send passes through an SSRF guard that rejects loopback, private and link-local targets unless you allow private networks.
- Nothing you enter here is shown again after you save it.

## Limits and caveats

- Needs the outbound email backend configured; the channel itself only holds the address.
- Delivery attempts are recorded and can be listed with `channels deliveries`.

## Troubleshooting

**The test email never arrives.** Send a test from Settings > Email first. The channel uses that backend, so a broken SMTP login breaks every email channel.

**Create says not configured.** The control plane has no master key set, which this feature needs.

**Mail lands in spam.** Set a from address on a domain whose SPF and DKIM match your sending service.

## FAQ

**Where do I get the destination for Email?**

Configure Settings > Email (SMTP, AWS SES or Resend), or APP_SMTP_HOST, APP_SMTP_PORT, APP_SMTP_USERNAME, APP_SMTP_PASSWORD and APP_SMTP_FROM.

**Can one channel serve many alert rules?**

Yes. Rules reference channels, so one Email channel can receive any number of rules.

## Related

- [Resend](/integrations/notifications/resend): Notification channels
- [Slack](/integrations/notifications/slack): Notification channels
- [All notification channels](/integrations/notifications)
- [Email documentation](https://www.rfc-editor.org/rfc/rfc5321), maintained by the vendor
