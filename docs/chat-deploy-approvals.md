# Chat-interactive deploy approvals

When a deploy or promotion targets a protected environment, Levelrail
requires a second, distinct user to approve it (see `docs/deploy-safety.md`
for the two-person approval gate itself). By default an approver has to
open the dashboard's Approvals queue to act. This feature adds a real
Approve/Deny button to the Slack or Discord message Levelrail can send
when a request comes in, so an approver can decide from their phone
without opening the dashboard.

This is an opt-in addition to an existing notification channel
(Settings -> Notification channels), not a separate integration: connect
a Slack or Discord channel first the normal way, then turn on
"Interactive approval buttons" on that channel's edit dialog.

## What you need

### Slack

Levelrail's existing Slack notification channel sends through an
Incoming Webhook. To receive button clicks back, that webhook needs to
belong to a Slack app with **Interactivity** turned on:

1. Create a Slack app at <https://api.slack.com/apps> (or use the one
   your Incoming Webhook already belongs to).
2. Under **Interactivity & Shortcuts**, turn Interactivity on and set the
   Request URL to:

   ```
   https://<your-levelrail-host>/api/v1/webhooks/slack/interactions
   ```

3. Under **Basic Information**, copy the **Signing Secret**. This is
   what Levelrail uses to verify an incoming button click really came
   from your Slack app; paste it into the channel's "Interactive
   approval secret" field.
4. Make sure the same app owns the Incoming Webhook URL the channel is
   already configured with (api.slack.com/apps/<app>/incoming-webhooks).

### Discord

Discord's equivalent of a Request URL is an application's Interactions
Endpoint URL, and verification uses the application's public key rather
than a shared secret:

1. Open your application at <https://discord.com/developers/applications>
   (the same one your Incoming Webhook belongs to; a webhook is always
   owned by an application).
2. Under **General Information**, copy the **Public Key** and paste it
   into the channel's "Interactive approval secret" field.
3. Set **Interactions Endpoint URL** to:

   ```
   https://<your-levelrail-host>/api/v1/webhooks/discord/interactions
   ```

   Discord sends a verification ping to this URL the moment you save it
   and refuses to save the URL if the ping isn't answered correctly, so
   this step must happen after the channel's secret is already saved in
   Levelrail with interactive approvals turned on.

## Turning it on

In Settings -> Notification channels, edit an existing Slack or Discord
channel, paste the secret from the step above, and turn on "Interactive
approval buttons." It's off by default: without a secret configured
there is nothing to verify an incoming click against, so the toggle
requires one.

## What it actually does, and what it doesn't

- A deploy or promotion that needs approval posts a message with
  **Approve** and **Deny** buttons to every channel that has this turned
  on.
- Clicking a button decides the approval through the exact same code
  path the dashboard's own Approve/Deny buttons use (the same
  not-pending check, the same "the requester can't also approve their
  own request" check, the same freeze gate). It cannot apply a change
  the dashboard path wouldn't also allow.
- **Identity scope, stated plainly:** Levelrail has no mechanism linking
  a Slack or Discord user to a Levelrail account. A verified click is
  authorized at the *channel* level, not the individual level: anyone
  who can click the button in that Slack channel or Discord server can
  decide the approval, the same way anyone with access to a configured
  webhook URL can already trigger an outbound notification. The
  clicking user's name is recorded on the decision for visibility, but
  it is not what authorizes the decision. Treat the Slack channel or
  Discord server you point this at as the trust boundary, not individual
  membership in it.
- A forged or replayed request cannot approve anything: every incoming
  request is signature-verified against the channel's own secret before
  anything else happens, and an already-decided approval cannot be
  decided again (a second click, or a retried delivery, gets told "no
  longer pending" rather than re-applying).

## Endpoints

| Platform | Endpoint | Auth |
| --- | --- | --- |
| Slack | `POST /api/v1/webhooks/slack/interactions` | `X-Slack-Signature` + `X-Slack-Request-Timestamp`, HMAC-SHA256 against the channel's signing secret |
| Discord | `POST /api/v1/webhooks/discord/interactions` | `X-Signature-Ed25519` + `X-Signature-Timestamp`, Ed25519 against the channel's public key |

Both routes are unauthenticated by session or API token, the same way
`POST /api/v1/webhooks/github/{name}` is: neither platform can present
one. Their own signature check is what stands in for auth, and every
request that fails it is rejected outright with no state change.
