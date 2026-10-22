# Form to email

Accepts a contact form (`name`, `email`, `message`; JSON or url-encoded) and stores it in the platform key-value store.

The platform has no email send API a function can call and Node ships no SMTP client, so this starter does not send mail itself. It stores every submission, can forward a one-line notice to a webhook you control (a chat channel, or a mail bridge that accepts `{"text": "..."}`), and `GET /submissions` lists what arrived. Add an SMTP library of your choice in `handler.js` if you want real email.

## Environment

| Variable | Required | Meaning |
| --- | --- | --- |
| `KV_URL`, `KV_TOKEN` | yes | Injected when you enable the key-value store for the function. |
| `ADMIN_TOKEN` | no | Bearer token for `GET /submissions`. The route is closed while it is unset. |
| `NOTIFY_WEBHOOK_URL` | no | Receives `{"text": "New message from ..."}` for each submission. |
| `ALLOWED_ORIGIN` | no | CORS origin of your site. Default `*`. |
| `SUBMISSION_TTL_SECONDS` | no | How long submissions are kept. Default 2592000 (30 days). |

A hidden `website` field works as a honeypot: bots that fill it get a success reply and nothing is stored.

## Test

```
node --test
```
