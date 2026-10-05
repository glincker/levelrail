---
description: Approve or deny a protected-environment deploy directly from the Slack or Discord notification, without opening the dashboard.
---

# Chat-interactive deploy approvals

When a deploy or promotion targets a protected environment, Levelrail requires a second, distinct user to approve it (see [Deploy safety](deploy-safety.md) for the two-person gate). By default an approver opens the dashboard's Approvals queue. This feature adds Approve and Deny buttons to the Slack or Discord message Levelrail sends when a request comes in, so an approver can decide from a phone.

It is an opt-in addition to an existing notification channel (**Settings > Notification channels**), not a separate integration. Connect a Slack or Discord channel the normal way first, then turn on **Interactive approval buttons** on that channel's edit dialog. It is off by default, and the toggle requires the secret from the platform setup below, because without one there is nothing to verify an incoming click against.

## Set up the platform

<Tabs :items="['Slack', 'Discord']">
<Tab value="Slack">

Levelrail's Slack channel sends through an Incoming Webhook. To receive button clicks, that webhook must belong to a Slack app with **Interactivity** turned on.

<Steps>
<Step title="Open your Slack app">

Create one at <https://api.slack.com/apps>, or use the app your Incoming Webhook already belongs to.

</Step>
<Step title="Set the Request URL">

Under **Interactivity & Shortcuts**, turn Interactivity on and set the Request URL to:

```
https://<your-levelrail-host>/api/v1/webhooks/slack/interactions
```

</Step>
<Step title="Copy the signing secret">

Under **Basic Information**, copy the **Signing Secret** and paste it into the channel's interactive approval secret field. Levelrail uses it to verify that a click came from your Slack app.

</Step>
<Step title="Check webhook ownership">

Make sure the same app owns the Incoming Webhook URL the channel is configured with.

</Step>
</Steps>

</Tab>
<Tab value="Discord">

Discord's equivalent of a Request URL is an application's Interactions Endpoint URL, and verification uses the application's public key instead of a shared secret.

<Steps>
<Step title="Copy the public key">

Open your application at <https://discord.com/developers/applications> (the one your Incoming Webhook belongs to). Under **General Information**, copy the **Public Key** and paste it into the channel's interactive approval secret field. Save the channel with interactive approvals on first.

</Step>
<Step title="Set the Interactions Endpoint URL">

Set it to:

```
https://<your-levelrail-host>/api/v1/webhooks/discord/interactions
```

Discord sends a verification ping when you save and refuses the URL if the ping is not answered correctly, so the key must already be saved in Levelrail.

</Step>
</Steps>

</Tab>
</Tabs>

## What it does

- A deploy or promotion that needs approval posts a message with **Approve** and **Deny** buttons to every channel that has this turned on.
- A click decides the approval through the same code path as the dashboard's buttons: the same not-pending check, the same rule that the requester cannot approve their own request, and the same freeze gate. It cannot apply a change the dashboard would not allow.
- A forged or replayed request cannot approve anything. Every incoming request is signature-verified against the channel's own secret first, and an already-decided approval cannot be decided again (a second click or a retried delivery is told it is no longer pending).

::: warning Identity scope
Levelrail has no link between a Slack or Discord user and a Levelrail account. A verified click is authorized at the channel level: anyone who can click the button in that Slack channel or Discord server can decide the approval. The clicking user's name is recorded on the decision for visibility, but it does not authorize it. Treat the channel or server you point this at as the trust boundary.
:::

## Endpoints

| Platform | Endpoint | Auth |
| --- | --- | --- |
| Slack | `POST /api/v1/webhooks/slack/interactions` | `X-Slack-Signature` and `X-Slack-Request-Timestamp`, HMAC-SHA256 against the channel's signing secret |
| Discord | `POST /api/v1/webhooks/discord/interactions` | `X-Signature-Ed25519` and `X-Signature-Timestamp`, Ed25519 against the channel's public key |

Neither route uses a session or API token, like `POST /api/v1/webhooks/github/{name}`, because neither platform can present one. The signature check stands in for auth, and a request that fails it is rejected with no state change.
