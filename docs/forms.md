---
title: "Forms"
tag: "Beta"
description: Collect submissions from the HTML forms on a static site with a public endpoint, a spam trap, per-address rate limits, an inbox with CSV export and notifications through your existing alert channels.
"og:url": "https://docs.levelrail.com/forms"
---

A static site has no server to receive a contact form. Forms gives each app one: you create a form with a name, paste a snippet into your HTML, and every post lands in an inbox on the app page.

## Create a form

Open the app, choose **Forms**, then **New form**. Or from a terminal:

```bash
levelrail-cli forms create mysite contact --redirect-url https://mysite.example/thanks
levelrail-cli forms snippet mysite contact
```

The form gets a public endpoint, `POST /api/v1/forms/<app>/<name>`, and a copyable HTML snippet:

```html
<form action="https://levelrail.example.com/api/v1/forms/mysite/contact" method="POST">
  <p hidden><label>Leave empty <input name="bot-field" tabindex="-1" autocomplete="off"></label></p>
  <label>Email <input type="email" name="email" required></label>
  <button type="submit">Send</button>
</form>
```

Form names use lowercase letters, digits, dashes and underscores. The name is part of the address, so it cannot be changed; create a new form and delete the old one instead.

## Design the fields

The Forms tab appears only for static-site apps. In **Design**, add, remove and reorder fields. Each field has a type (text, email, long text, dropdown, checkbox, number, web address or hidden), a label, a name, a required flag, a placeholder and optional minimum and maximum length. A live preview sits beside the editor and the snippet under **Embed** follows your edits in three variants: HTML, HTML with a `fetch` script, and minimal markup without labels.

A form that never sets fields keeps the default name, email and message fields. Once fields are set, the server validates each post (required, type, length, dropdown choice). Invalid posts answer 422 with `{"error":..., "fields":{"email":"invalid_email"}}`. Submitted names that are not declared are dropped, not stored; the spam trap field is always checked first.

Optional extras: a **success message** (shown on the thank-you page and returned as `message` to JSON clients) and **allowed sites**. When allowed sites are set, browser posts from other origins get 403 and CORS only echoes the allowed origin; requests with no `Origin` header (curl, servers) still pass.

```bash
levelrail-cli forms fields get mysite contact > schema.json
levelrail-cli forms fields set mysite contact --file schema.json
```

```json
{
  "fields": [
    { "name": "email", "type": "email", "label": "Email", "required": true },
    { "name": "plan", "type": "select", "options": ["free", "pro"] }
  ],
  "success_message": "Thanks, we will reply soon.",
  "allowed_origins": ["https://mysite.example"]
}
```

## The endpoint address

The snippet uses the app's first https domain. With no domain it uses the configured dashboard (public) URL. With neither, the page shows a warning and a **Set the public URL** button, because that endpoint only works from the machine running Levelrail.

## Test it

**Send a test submission** stores a sample post through the real handler, marked **Test** in the inbox, without notifying channels. CLI: `levelrail-cli forms test <app> <name>`.

## What the endpoint accepts

| Content type | Notes |
| --- | --- |
| `application/x-www-form-urlencoded` | A plain HTML form. Repeated names are joined with commas. |
| `multipart/form-data` | Text fields are kept. Files are not stored, only the file name. |
| `application/json` | A flat object. Lists are joined with commas, nested objects are stored as JSON text. |

A browser post is redirected (303) to the form's redirect URL, or shown a plain thank-you page when none is set. A request that sends JSON or `Accept: application/json` gets `{"ok":true}` instead, so a `fetch` call from your own script does not navigate away.

Unless the form lists allowed sites, the endpoint answers cross-origin requests (`Access-Control-Allow-Origin: *`) because it is write-only: it takes no credentials and never returns stored data. Unknown form names answer 404, and one form's address can never read another form's submissions.

## Spam protection

- **Spam trap.** The hidden `bot-field` input is left empty by people and often filled by bots. A post that fills it is stored as spam, shown to the sender exactly like a success, and never sends a notification. Change the field name in the form settings.
- **Link flood.** A post with more links than `FORMS_SPAM_MAX_LINKS` (default 5) across its fields is stored as spam.
- **Rate limits.** Each client address may post `FORMS_RATE_PER_IP_PER_MINUTE` times a minute (default 10), and each form accepts `FORMS_RATE_PER_FORM_PER_MINUTE` (default 120). Over the limit gets 429 with `Retry-After`. The client address is read the same way as the rest of the API, so `X-Forwarded-For` is trusted only from a configured proxy.
- **Review.** Spam is kept, not dropped. Mark a submission as spam or not spam in the inbox, or with `forms spam` and `forms unspam`.

Heavy abuse of a site's own pages is better handled at the ingress with the [firewall](/firewall) and bot shield; the form endpoint lives on the control plane address, so limits there protect only the form.

## Inbox, notifications and export

The inbox lists the newest submissions first, loads more as you scroll, and searches field text on the server. Filter by spam or not spam, flag or delete single submissions, and export the current filter as CSV. Cells that a spreadsheet would run as a formula are prefixed with a quote.

Pick any [notification channel](/alert-inbox) in the form settings to be told about each new, non-spam submission. The message lists up to eight fields, flattened and shortened, and links to the inbox when a dashboard URL is set. A failing channel never fails the post: the submission is already saved, the failure is logged, and the visitor still sees success.

## Personal data, retention and deletion

Submissions usually contain personal data, so the defaults are conservative.

- The sender's address is never stored. Each submission keeps a salted hash (different salt per form, so hashes cannot be matched across forms), enough to spot repeat senders in the inbox but not to recover the address.
- Submissions older than `FORMS_RETENTION_DAYS` (default 90) are deleted automatically. Set it to `0` to keep them until you delete them.
- A form keeps at most `FORMS_MAX_STORED_PER_FORM` submissions (default 5000). When the cap is reached, spam is dropped first, then the oldest.
- Deleting a submission, a form, or the app removes the stored data. Nothing is kept elsewhere.
- Reading submissions needs the `read:sensitive` ability; creating, editing and deleting forms needs `deploy`.
- Tell your visitors what you collect. The control plane stores what the form sends and nothing about the browser.

## Limits and settings

| Variable | Default | Meaning |
| --- | --- | --- |
| `FORMS_MAX_BODY_BYTES` | 1048576 | Largest accepted post body. |
| `FORMS_MAX_FIELDS` | 100 | Most fields in one post. |
| `FORMS_MAX_FIELD_BYTES` | 16384 | Longest single value. |
| `FORMS_RATE_PER_IP_PER_MINUTE` | 10 | Posts per client address per minute, `0` for no limit. |
| `FORMS_RATE_PER_FORM_PER_MINUTE` | 120 | Posts per form per minute, `0` for no limit. |
| `FORMS_RETENTION_DAYS` | 90 | Days to keep submissions, `0` to keep them. |
| `FORMS_MAX_STORED_PER_FORM` | 5000 | Stored submissions per form, `0` for no cap. |
| `FORMS_MAX_PER_APP` | 20 | Forms per app, `0` for no cap. |
| `FORMS_SPAM_MAX_LINKS` | 5 | Links across all fields before a post counts as spam, `0` to turn the check off. |

Limits are read when the first post arrives after a start, so restart the control plane after changing them.

## CLI

```bash
levelrail-cli forms list <app>
levelrail-cli forms create <app> <name> [--redirect-url URL] [--honeypot FIELD] [--notify CHANNEL_ID,...]
levelrail-cli forms update <app> <name> [--redirect-url URL] [--honeypot FIELD] [--notify CHANNEL_ID,...]
levelrail-cli forms snippet <app> <name>
levelrail-cli forms fields get|set <app> <name> [--file schema.json]
levelrail-cli forms test <app> <name>
levelrail-cli forms submissions <app> <name> [--search TEXT] [--spam true|false] [--limit N]
levelrail-cli forms spam|unspam|remove <app> <name> <submission-id>
levelrail-cli forms export <app> <name> [--search TEXT] [--spam true|false] > submissions.csv
levelrail-cli forms delete <app> <name>
```

## What it does not do yet

- It does not find forms in your HTML automatically. You create each form and use its endpoint.
- It does not verify CAPTCHA tokens or call third-party spam services.
- It does not store uploaded files.
- It does not send a confirmation back to the visitor.
