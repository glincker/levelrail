---
title: "Mailboxes, aliases and the mail queue"
tag: "Experimental"
description: Create team owned mailboxes on the mox mail server, suspend and reset them, forward addresses with aliases, and hold or release outgoing mail.
"og:url": "https://docs.levelrail.com/mail-mailboxes"
---

<Note>Available only when `APP_EXPERIMENTAL=mail` is set on the control plane and a mail app from the [mail-mox template](/mail-template) is running.</Note>

## Who owns what

Each mail domain belongs to one team. Mailboxes and aliases on that domain belong to the same team. Members of other teams do not see them: the API answers 404, as if they did not exist. A second team cannot claim a domain that is already taken.

Levelrail keeps only metadata: the address, the owning team, an optional linked Levelrail user, whether the mailbox is suspended, who created it and when. Passwords live in the mail server only, and the control plane never reads message content.

## Claim a domain

The domain must already be served by the mail server. A team owner or admin with the root ability claims it:

```bash
levelrail-cli mail domains claim example.com --team <team-id>
levelrail-cli mail domains list
```

## Mailboxes

```bash
levelrail-cli mail mailboxes add jo@example.com --linked-user <user-id>
levelrail-cli mail mailboxes list --domain example.com
levelrail-cli mail mailboxes suspend jo@example.com
levelrail-cli mail mailboxes resume jo@example.com
levelrail-cli mail mailboxes passwd jo@example.com
levelrail-cli mail mailboxes delete jo@example.com --confirm jo@example.com
```

- `add` and `passwd` print a generated password **once**. Levelrail does not store it, so copy it and hand it to the mailbox user. Users cannot choose their own password. The length comes from `APP_MAIL_PASSWORD_LENGTH` (default 24).
- `suspend` blocks IMAP, SMTP submission and webmail logins. Mail keeps arriving.
- When a member leaves a team, or is removed from it, mailboxes linked to them in that team are suspended. Nothing is deleted automatically.
- `delete` asks for your password, needs the address typed again, and refuses a mailbox that still holds mail unless you pass `--delete-mail`. Suspending is usually the better choice.
- Each new mailbox reports a send cap from `APP_MAIL_DEFAULT_SEND_CAP_PER_HOUR` (default 200). It is recorded as data only and is not enforced yet.

## Aliases

```bash
levelrail-cli mail aliases add team@example.com --to jo@example.com,sam@example.com
levelrail-cli mail aliases list --domain example.com
levelrail-cli mail aliases remove team@example.com
```

Alias targets must be mailboxes your team owns.

## Queue

```bash
levelrail-cli mail queue list
levelrail-cli mail queue hold
levelrail-cli mail queue release --ids 12,13
```

The queue view shows the sender account, recipient domain, attempts, next attempt and last error. It never shows subjects or bodies. Listing needs the `read:sensitive` ability. Hold and release apply to the whole queue unless you pass `--ids`. Release asks for your password. The queue is an instance wide view, so only instance members can use it.

## Permissions

| Action | Ability |
| --- | --- |
| List domains, mailboxes, aliases | `read` |
| List the queue | `read:sensitive` |
| Add a mailbox or alias, suspend, resume | `write:sensitive` |
| Reset a password | `write:sensitive` plus step-up |
| Hold the queue | `write:sensitive` |
| Release the queue | `write:sensitive` plus step-up |
| Claim a domain, delete a mailbox | `root` plus step-up |

IAM policies can name `mail:service:<app>`, `mail:domain:<domain>` and `mail:mailbox:<address>`. A Deny on a domain hides every mailbox on it. Every change is written to the audit log with the address, never a password.

## Limits

- mox keeps one password per account, so revoking one device means resetting the password for all of them.
- mox has no per hour send limit and no app passwords.
