---
description: Configure SMTP, AWS SES, or Resend as the backend for alert emails and password resets, and send a test email to confirm it works.
---

# Email notifications

Levelrail sends email for alert notifications (when an alert rule with an email channel fires), password-reset links and team invites. All of them go through one backend, configured once at **Settings > Email**.

Credentials are stored with the control plane's envelope encryption, so a master key must be configured. They are never shown back after saving: a saved field reads "already configured, leave blank to keep it".

## Choose a backend

<Tabs :items="['SMTP', 'AWS SES', 'Resend']">
<Tab value="SMTP">

Works with any mail server or transactional provider that speaks SMTP (Gmail, Mailgun, Postmark, your own server). Needs a host, port, "from" address, and optionally a username and password.

```bash
levelrail-cli settings email set --backend smtp \
  --smtp-host smtp.example.com --smtp-port 587 \
  --smtp-username apikey --smtp-password '...' \
  --smtp-from alerts@example.com
```

You can also set `APP_SMTP_HOST`, `APP_SMTP_PORT`, `APP_SMTP_USERNAME`, `APP_SMTP_PASSWORD` and `APP_SMTP_FROM` on the control plane.

</Tab>
<Tab value="AWS SES">

For installs already on AWS. Needs a region, access key ID, secret access key and a verified "from" address.

```bash
levelrail-cli settings email set --backend ses \
  --ses-region us-east-1 --ses-access-key-id AKIA... \
  --ses-secret-access-key '...' --ses-from alerts@example.com
```

</Tab>
<Tab value="Resend">

A dedicated transactional provider with a single API key. Needs the API key and a "from" address. Configure it in **Settings > Email** or through `PUT` on the email settings API; the CLI's `settings email set` covers SMTP and SES only.

</Tab>
</Tabs>

`levelrail-cli settings email get` shows the current settings without credential values.

## Test it

After saving, use **Send test email** in **Settings > Email**. It sends one real message through the same code path alerts, resets and invites use, so a successful test covers all three.

## Other notification channels

Email is one channel among several. Slack, Discord, Telegram, PagerDuty and the rest are configured per channel at **Settings > Notification channels**. See [Observability](observability.md#notification-channels).
