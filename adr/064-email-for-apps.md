# ADR 064: Email for apps

Status: Accepted

Date: 2026-10-30

## Context

Operators want their apps to send email (sign-ups, resets, receipts) without
learning SPF, DKIM and DMARC by hand. ADRs 058, 059, 060 and 062 described a
mail server run inside Levelrail. Hosting mailboxes needs inbound port 25, a
clean sending IP, reverse DNS and long-lived mail storage, which is a different
product from running apps.

## Decision

1. Levelrail does not host human mailboxes and does not run its own webmail.
   The direction for email is "email for apps":
   - connect a sending provider once, with a send-only key;
   - Levelrail creates the SPF, DKIM and DMARC records through the operator's
     DNS provider and verifies each one, with a plain message and a stable
     code for every problem it finds;
   - a test send proves the domain works;
   - each app gets its own send-only credentials, injected as bound
     environment values and masked in the API and UI.
2. System email (alerts, invites, password resets, status page subscribers)
   stays on the provider path.
3. The experimental managed mail server built on mox stays available as a beta
   feature, behind its flag, which is off by default. This ADR does not freeze
   it and does not schedule it for removal. It is not promoted, and it
   receives only fixes for data loss and lockout.
4. Whether the mail server is kept, replaced or removed is decided in a later
   ADR, once email for apps has shipped.
5. The DNS record planning, application and checking code, the provider
   clients and the system email templates are reused for email for apps.

## Consequences

- ADRs 058, 059, 060 and 062 stay accurate for the beta server. They are
  partly superseded: new mail work follows this ADR.
- Operators who want mailboxes can use a mailbox provider and point their
  domain at it. Levelrail checks the domain records the same way.
- Forwarding for a domain, if offered later, uses a DNS provider's own routing
  feature and does not add a receiving server to Levelrail.
