---
description: Toggle behavior in a running app without a redeploy, with per-app flags, percentage rollouts and a simple evaluate endpoint.
---

# Feature flags

Feature flags let you change behavior in a running app with no redeploy or restart. You create a flag, your app asks the control plane whether it is on, and you flip it from the dashboard or CLI whenever you like.

## Why not an env var

Env vars are fixed when a container is created, so changing one needs a redeploy or restart. A feature flag is read at runtime: your app calls the control plane's HTTP API for the current value. It authenticates with an ordinary API token, so there is no separate auth system to set up.

## How flags behave

- **Scope.** A flag belongs to one app, and the dashboard and CLI manage it per app. Its `key`, the string your app looks up, is unique across the whole control plane, not just within the app. The evaluate endpoint is flat (`/api/v1/flags/evaluate/{key}`, no app in the path) because API tokens are not scoped to an app.
- **Kill switch.** `enabled: false` turns the flag off for every caller.
- **Rollout.** When `enabled` is true, `rollout_percentage` (0 to 100) decides which callers get it. Callers are bucketed by a consistent hash (FNV-1a) of the `identifier` query parameter, so the same identifier always lands on the same side of a partial rollout. Pass a stable per-user or per-device value. Callers that send no identifier all share one outcome.

## Set up a flag

<Steps>
<Step title="Create the flag">

In the dashboard, open an app's **Feature flags** tab. From the CLI:

```bash
levelrail-cli flags create my-app --key new-checkout --name "New checkout" --rollout 25
```

`--rollout` defaults to 100 and the flag starts enabled unless you pass `--disabled`.

</Step>
<Step title="Create a read-only token for your app">

```bash
levelrail-cli tokens create --name app-flags --abilities read
```

Inject the token into your app as a secret env var such as `FLAGS_TOKEN` (`{ secret: true }` in [app.yaml](app-spec-reference.md#envvar-an-entry-under-env)).

</Step>
<Step title="Call the evaluate endpoint from your app">

```bash
curl -s \
  -H "Authorization: Bearer $FLAGS_TOKEN" \
  "https://your-control-plane/api/v1/flags/evaluate/new-checkout?identifier=user-123"
```

```json
{ "key": "new-checkout", "enabled": true }
```

That is the whole contract. No SDK exists, so any language can call it with a plain HTTP client.

</Step>
<Step title="Change it live">

Flip the switch or move the rollout slider in the dashboard, or use `flags set`. The next evaluate call reflects it.

</Step>
</Steps>

## CLI

```bash
levelrail-cli flags create <app> --key KEY --name NAME [--description DESC] [--disabled] [--rollout PERCENT]
levelrail-cli flags list <app>
levelrail-cli flags get <app> <id>
levelrail-cli flags set <app> <id> --name NAME [--description DESC] [--disabled] [--rollout PERCENT]
levelrail-cli flags delete <app> <id>
```

`flags set` replaces the flag's settings. It requires `--name`, and any option you leave out returns to its default (enabled, rollout 100, empty description). To change only the rollout, repeat the others:

```bash
levelrail-cli flags set my-app <id> --name "New checkout" --rollout 50
```

## API reference

| Method | Path | Ability |
| --- | --- | --- |
| `POST` | `/api/v1/apps/{name}/flags` | `write` |
| `GET` | `/api/v1/apps/{name}/flags` | `read` |
| `GET` | `/api/v1/apps/{name}/flags/{id}` | `read` |
| `PUT` | `/api/v1/apps/{name}/flags/{id}` | `write` |
| `DELETE` | `/api/v1/apps/{name}/flags/{id}` | `write` |
| `GET` | `/api/v1/flags/evaluate/{key}?identifier=...` | `read` |

The evaluate response is only `key` and `enabled`, because apps may call it on every request.

## Not supported

- **Targeting rules.** Rollout is a flat percentage by identifier. There is no targeting by user attribute such as plan.
- **Flag history.** Create, update and delete show up in the generic audit log (`GET /api/v1/audit-log`), with no flag-specific history view.

## Next steps

<CardGroup :cols="2">
<Card title="Identity and access" href="/identity-and-access">

Creating read-only tokens.

</Card>
<Card title="Deploying apps" href="/deploying-apps">

Injecting a token as a secret env var.

</Card>
</CardGroup>
