---
title: "Email notifications"
description: Choose an email provider for alert emails, password resets and invites, check your sender domain, send a test, and read the delivery log.
"og:url": "https://docs.levelrail.com/email-notifications"
---

Levelrail sends email for alert notifications (when an alert rule with an email channel fires), password-reset links and team invites. All of them go through the providers configured once at **Settings > Email**: a primary and, optionally, a backup.

Credentials are stored with the control plane's envelope encryption, so a master key must be configured. They are never shown back after saving: a saved field reads "already configured, leave blank to keep it".

The page is a set of compact cards:

- **Status bar.** Appears once a provider exists. It shows the state of the primary and the backup, a **Running on backup** badge while the backup is carrying mail, the last successful send, the last error and the 24 hour counts.
- **Providers.** One row each for the primary and the backup, with the provider, its from address, an enabled switch, the last successful send, a status chip and the actions **Edit**, **Test**, **Make primary** (backup only) and **Remove**. With no provider the card shows the templates directly. With no backup the row reads **Add a backup provider**. **Edit** opens a panel that shows only the fields of that provider's backend, including its TLS settings for SMTP.
- **Sender identity and limits.** From name, from address, reply-to, the envelope sender (SMTP providers) and the sends per minute and per day caps. Every provider uses these.
- **Delivery check.** Appears once a provider exists. A test send through the provider you pick, and a sender domain check for each provider.
- **Branding.** See below.

A credential that is already stored shows a **Set** chip instead of its value, with **Replace** to type a new one and **Reveal** to read it back (see below). The same controls work on the backup.

## Add a provider

**Add a provider** (or **Add a backup provider**) opens a list of templates: Resend, Amazon SES (API and SMTP), Postmark, SendGrid, Mailgun, Google Workspace or Gmail, Microsoft 365, Brevo and a generic SMTP server. Each one shows what it is best for. Choosing a template opens the edit panel already filled in: the SMTP host, port and TLS mode for relays, plus a hint for where to find the credential and a link to the provider's docs. You only add the credential and, if this provider should send as a different address, its own from address. Nothing is saved until you choose **Add provider**.

The **Sender identity** card suggests values you can use with one click, marked **Suggested**: the from name from your instance name, a `noreply@` address on your instance's public domain (when one is configured), the signed-in owner's address as reply-to, and the from address as the envelope sender. A suggestion only fills the field; it is saved when you save the card. Addresses are checked as you type, and once a from address is set the card links to the **Delivery check** for SPF, DKIM and DMARC.

## Primary and backup

The primary sends everything. When it keeps failing, mail switches to the backup automatically and returns to the primary once it recovers. Add the backup from the **Providers** card, test each one on its own, and use **Make primary** to swap them. The primary cannot be removed while a backup exists. The failover threshold and cooldown come from `APP_EMAIL_FAILOVER_THRESHOLD` and `APP_EMAIL_FAILOVER_COOLDOWN_SECONDS`.

```bash
levelrail-cli settings email providers list
levelrail-cli settings email providers add --backend smtp --smtp-host smtp.example.com --smtp-port 587
levelrail-cli settings email providers promote <id>
```

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

After saving, use **Test** on a provider row, or pick a provider in **Delivery check** and choose **Send test email**. Leave the address blank to send to yourself. It sends one real message through that provider alone, with no failover, and shows the provider response; the result is announced to screen readers. A failure comes back as a typed reason with a fix: authentication failed, sender not verified, region mismatch, port blocked, TLS error or rate limited.

```bash
levelrail-cli email test --to you@example.com
```

## Check your sender domain

**Delivery check** has a domain check for each provider. It looks up SPF, DKIM and DMARC for that provider's from address domain, resolved by the control plane itself, and marks each record pass or needs attention with the next step and a hint for that provider. The API takes `?provider=<id>` on `GET /api/v1/settings/email/domain-check`.

```bash
levelrail-cli email check-domain
```

## Branding

The **Branding** card on **Settings > Email** sets the logo, accent color and footer every rich message shares (password resets, invites, alerts, announcements). A live preview renders in a sandboxed frame before you save. The **Minimal**, **Standard** and **Compliance** templates fill the footer text (and, for Compliance, an address line to replace with your postal address), add a dashboard link when your instance has an https address, and fill the accent color from your brand color when it is empty. They only use the variables listed below, and nothing is saved until you choose **Save**. Logo and link addresses are checked for `https` as you type.

- **Logo** is an `https` URL. The control plane never fetches it; it is embedded as an image, so a remote logo tells its host when a message is opened. Host it somewhere you control.
- **Accent color** is a six digit hex value. Empty uses the brand default.
- **Footer** is plain text, an address and up to five `https` links. There is no HTML field. Footer text accepts the global variables `{{brand_name}}`, `{{support_email}}`, `{{dashboard_url}}`, `{{instance_name}}` and `{{year}}`. An unknown variable is refused on save, and a substituted value is never expanded a second time.
- **Powered by** line is off by default.

`javascript:`, `data:` and plain `http` URLs are refused, and logo and link URLs are capped in length.

```bash
levelrail-cli settings email branding set --logo-url https://cdn.example.com/logo.png --brand-color "#107292" --footer-link "Privacy|https://example.com/privacy"
levelrail-cli settings email branding preview > preview.html
```

| Variable | Default | Effect |
| --- | --- | --- |
| `APP_EMAIL_BRANDING_MAX_LOGO_URL` | `500` | Longest logo or link URL accepted. |
| `APP_EMAIL_BRANDING_MAX_FOOTER_LINKS` | `5` | Most footer links. |
| `APP_EMAIL_BRANDING_MAX_FOOTER_TEXT`, `APP_EMAIL_BRANDING_MAX_FOOTER_ADDRESS` | `1000`, `300` | Footer text and address length. |
| `APP_EMAIL_BRANDING_MAX_LINK_LABEL` | `60` | Footer link label length. |

## Delivery log

Every outbound send is recorded with its provider, outcome, provider message id and error, and can be searched and filtered at **Settings > Email > Delivery log** or with `levelrail-cli email log --status failed`. Recipients are stored hashed and shown shortened (`a***@example.com`).

| Variable | Default | Effect |
| --- | --- | --- |
| `APP_EMAIL_LOG_RETENTION_DAYS` | `30` | Rows older than this are swept hourly. `0` keeps them. |
| `APP_EMAIL_LOG_FULL_ADDRESS` | off | Set to `true` to store and show full recipient addresses. |
| `APP_EMAIL_SENDS_PER_MINUTE`, `APP_EMAIL_SENDS_PER_DAY` | unlimited | Default caps, used when the page leaves a cap at 0. |
| `APP_EMAIL_RETRY_ATTEMPTS`, `APP_EMAIL_RETRY_BASE_MS` | `3`, `500` | Attempts and first backoff for transient provider failures. |
| `APP_EMAIL_REVEAL_PER_MINUTE` | `3` | Credential reveals allowed per user per minute. |

Transient failures (provider 5xx, rate limiting, SMTP 4xx, dropped connections) are retried with doubling backoff. Authentication, sender and TLS failures are not retried.

## Reveal a stored credential

Stored credentials are write-only everywhere except one path: **Reveal** next to a configured credential in **Settings > Email**, or `levelrail-cli settings email providers reveal <id> <field> --yes-print`. It needs your password and an authenticator or recovery code, so an account without two-factor authentication is refused. The value shows for 30 seconds, is never cached, and every attempt is recorded in the audit log without the value. There is no MCP tool for it.

The alert rule kind `email_delivery_failing` fires while recent sends mostly fail. It is available through the alerts API and has no default rule.

## Other notification channels

Email is one channel among several. Slack, Discord, Telegram, PagerDuty and the rest are configured per channel at **Settings > Notification channels**. See [Observability](/observability#notification-channels).
