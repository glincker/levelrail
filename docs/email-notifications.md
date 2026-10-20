---
title: "Email notifications"
description: Choose an email provider for alert emails, password resets and invites, check your sender domain, send a test, and read the delivery log.
"og:url": "https://docs.levelrail.com/email-notifications"
---

Levelrail sends email for alert notifications (when an alert rule with an email channel fires), password-reset links and team invites. All of them go through one provider, configured once at **Settings > Email**.

Credentials are stored with the control plane's envelope encryption, so a master key must be configured. They are never shown back after saving: a saved field reads "already configured, leave blank to keep it".

The page has four groups: **Sender** (from name, from address, reply-to, optional envelope sender), **Provider**, **Delivery limits** (sends per minute and per day) and **Security** (TLS mode and certificate verification). A status header at the top shows whether sending is configured, the last successful send and the last failure.

## Choose a provider

<Tabs>
<Tab title="SMTP">

Works with any mail server or relay that speaks SMTP (Gmail, Brevo, Mailtrap, your own server). Needs a host, port and from address, and optionally a username and password. The TLS mode defaults to automatic: STARTTLS when the server offers it, and TLS from the first byte on port 465.

```bash
levelrail-cli email settings set --backend smtp \
  --smtp-host smtp.example.com --smtp-port 587 \
  --smtp-username apikey --smtp-password '...' \
  --from alerts@example.com
```

You can also set `APP_SMTP_HOST`, `APP_SMTP_PORT`, `APP_SMTP_USERNAME`, `APP_SMTP_PASSWORD` and `APP_SMTP_FROM` on the control plane.

</Tab>
<Tab title="AWS SES (API)">

Uses the SES API with an IAM access key. Needs a region picked from the list of SES regions (such as `us-east-1`, never a hostname), the access key ID and secret, and a verified from address.

```bash
levelrail-cli email settings set --backend ses \
  --ses-region us-east-1 --ses-access-key-id AKIA... \
  --ses-secret-access-key '...' --from alerts@example.com
```

</Tab>
<Tab title="AWS SES (SMTP)">

Uses the SES SMTP endpoint. The host is derived from the region (`email-smtp.<region>.amazonaws.com`), on port 587 (STARTTLS) or 465 (TLS). SES SMTP credentials are generated in the SES console and are **not** the same as IAM access keys.

```bash
levelrail-cli email settings set --backend ses_smtp \
  --ses-region eu-west-1 --ses-smtp-username AKIA... \
  --ses-smtp-password '...' --from alerts@example.com
```

</Tab>
<Tab title="Resend, Postmark, SendGrid, Mailgun">

Key-based HTTP providers. Each needs an API key (Postmark: server token) and a verified from address. Mailgun also needs the sending domain and a region (US or EU).

```bash
levelrail-cli email settings set --backend mailgun \
  --mailgun-domain mg.example.com --mailgun-region eu \
  --mailgun-api-key '...' --from alerts@example.com
```

</Tab>
</Tabs>

`levelrail-cli email settings get` shows the current settings without credential values. The older `settings email get|set` commands still work.

## Test it

After saving, use **Send test email** in **Settings > Email**. Leave the address blank to send to yourself. It sends one real message through the same code path alerts, resets and invites use, and shows the provider response. A failure comes back as a typed reason with a fix: authentication failed, sender not verified, region mismatch, port blocked, TLS error or rate limited.

```bash
levelrail-cli email test --to you@example.com
```

## Check your sender domain

The **Sender domain check** card looks up SPF, DKIM and DMARC for the from address domain, resolved by the control plane itself, and marks each record pass or needs attention with the next step and a hint for your provider.

```bash
levelrail-cli email check-domain
```

## Delivery log

Every outbound send is recorded with its provider, outcome, provider message id and error, and can be searched and filtered at **Settings > Email > Delivery log** or with `levelrail-cli email log --status failed`. Recipients are stored hashed and shown shortened (`a***@example.com`).

| Variable | Default | Effect |
| --- | --- | --- |
| `APP_EMAIL_LOG_RETENTION_DAYS` | `30` | Rows older than this are swept hourly. `0` keeps them. |
| `APP_EMAIL_LOG_FULL_ADDRESS` | off | Set to `true` to store and show full recipient addresses. |
| `APP_EMAIL_SENDS_PER_MINUTE`, `APP_EMAIL_SENDS_PER_DAY` | unlimited | Default caps, used when the page leaves a cap at 0. |
| `APP_EMAIL_RETRY_ATTEMPTS`, `APP_EMAIL_RETRY_BASE_MS` | `3`, `500` | Attempts and first backoff for transient provider failures. |

Transient failures (provider 5xx, rate limiting, SMTP 4xx, dropped connections) are retried with doubling backoff. Authentication, sender and TLS failures are not retried.

The alert rule kind `email_delivery_failing` fires while recent sends mostly fail. It is available through the alerts API and has no default rule.

## Other notification channels

Email is one channel among several. Slack, Discord, Telegram, PagerDuty and the rest are configured per channel at **Settings > Notification channels**. See [Observability](/observability#notification-channels).
