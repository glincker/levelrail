---
title: "Email templates and messages"
description: Edit every email the platform sends, build audiences, and send announcements with opt-outs, scheduling and delivery history.
"og:url": "https://docs.levelrail.com/email-templates"
---

Every email the platform sends uses one branded layout and one template per event. You can edit the wording, preview it, send yourself a test and reset it. You can also write to groups of people. Delivery itself (SMTP, SES or Resend) is set up under **Settings > Email > Delivery**, see [Email notifications](/email-notifications).

## Templates

Open **Settings > Email > Templates**. Built in templates cover password resets, invites, sign-in codes, new sign-in alerts, email changes, alert and deploy notifications, deploy approvals, the test email and two announcement starters.

Each template has a subject, a preheader (the preview line inboxes show) and a body. The editor shows a live preview at desktop and mobile width, in light and dark, with sample data, and keeps the last 10 saved versions so you can restore one.

**Shared layout.** A header image (https only), a footer line such as your postal address, and the brand name and color come from the brand settings. Every email gets an HTML part and a generated plain text part.

### Body markup

Bodies use a deliberately small markup. There is no raw HTML and no remote content other than the header image.

| Write | Result |
| --- | --- |
| a blank line | new paragraph |
| `# Title` | heading |
| `- item` | list |
| `> note` | callout |
| `**bold**` | bold |
| `[label](address)` | link (http, https or mailto) |
| `!button Label\|address` | button |

### Tokens

Tokens look like `{{first_name}}` and are replaced when the email is sent. Values are escaped, there is no code execution, and an unknown token is refused when you save. The editor lists the tokens each email supports; click one to insert it. From the CLI:

```bash
levelrail-cli email tokens list
levelrail-cli email templates list
levelrail-cli email templates get password_reset
levelrail-cli email templates set password_reset --subject "Reset your {{brand_name}} password"
levelrail-cli email templates preview password_reset --scheme dark
levelrail-cli email templates reset password_reset
```

## Audiences

**Settings > Email > Audiences** holds saved groups. A group selects users by role, team, project (through its team), sign-in email domain, or how long since they last signed in, and can add extra addresses and always exclude others. The dialog shows a live recipient count and a masked preview of the first 20.

```bash
levelrail-cli email groups create admins --role owner --role admin
levelrail-cli email groups show admins
```

### Opt-outs

People who unsubscribe, or whom you add by hand, go on the opt-out list. Announcements skip them. Security and account email (password resets, sign-in codes and alerts, invites) always arrives. Every announcement carries a signed one-click unsubscribe link and the standard List-Unsubscribe headers.

## Messages

**Settings > Email > Messages** composes a message to an audience or to chosen addresses. Start from an announcement template, edit the subject and body, preview it, send a test to yourself, and send now or later. Before sending, a confirmation shows how many people will receive it and how many are suppressed.

Sending is queued and rate limited, shows per-recipient status, can be cancelled while it runs, and stays in the history with delivered and failed counts. Only administrators can send. A message is refused when no email provider is configured, and the page offers a button to the email settings.

```bash
levelrail-cli email send --group admins --template maintenance_notice --dry-run
levelrail-cli email send --group admins --template maintenance_notice --yes
```

### Limits

| Setting | Default | Meaning |
| --- | --- | --- |
| `APP_MAIL_BROADCAST_DAILY_CAP` | 500 | Recipients per rolling day across all messages |
| `APP_MAIL_SEND_DELAY` | 200ms | Pause between two messages |
| `APP_MAIL_SEND_BATCH` | 50 | Recipients loaded between status checks |
