---
description: Configure SMTP, AWS SES, or Resend as the backend for alert emails and password resets, and send a test email to confirm it works.
---

# Email notifications

Levelrail sends two kinds of email: alert notifications (when an alert
rule you've configured fires) and password-reset links. Both go through
the same backend, configured once at **Settings > Email**.

## Choosing a backend

Pick whichever you already have:

- **SMTP** works with any mail server or transactional-email provider
  that speaks SMTP (Gmail, Mailgun, Postmark, your own mail server).
- **AWS SES** if you already run infrastructure on AWS.
- **Resend** if you want a dedicated transactional-email provider with a
  simple API key.

Each backend needs a "from" address and provider-specific credentials.
Credentials are encrypted at rest and never shown back to you after
saving; a saved field shows "already configured, leave blank to keep
it" instead of the actual value.

## Testing it

After saving, use **Send test email** to confirm the backend actually
works end-to-end: it sends one real email through the exact same code
path alert notifications and password resets use, so a successful test
means both of those will work too.

## Looking for Slack, Discord, or webhook notifications instead?

Email is one notification channel among several. Chat and webhook
notifications (Slack, Discord, Telegram, PagerDuty, and more) live at
**Settings > Notification channels**, configured per-alert-rule rather
than platform-wide.
