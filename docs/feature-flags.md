# Feature flags

Toggle behavior in a running app without a redeploy. Package:
`internal/api/feature_flags.go`, `internal/store/feature_flag.go`.

## Why this isn't an env var

Env vars are baked into a container at create time and aren't
live-updatable: changing one still means a redeploy or at least a
restart. A feature flag's whole point is the opposite, toggling behavior
*without* either. So instead of an env var, your app's own code calls
back into the control plane's HTTP API at runtime to read a flag's
current value, the same model any external feature-flag service
(LaunchDarkly, Unleash, etc.) uses, except self-hosted and built into
this platform.

Authentication reuses the existing API token system entirely: create a
token scoped to just the `read` ability, inject it into your app as a
`{ secret: true }` env var the normal way, then have your app send it as
a bearer token when it calls the evaluate endpoint. No new auth surface.

## Scope and key uniqueness

A flag is owned by one app (the same `service_name` scoping
`scheduled-tasks` and alerts already use), which is what the dashboard
tab and the CLI's `flags <verb> <app> ...` commands are scoped by. Its
`key`, the string your app looks it up by, is globally unique across the
whole control plane, not just within that app: the evaluate endpoint is
deliberately flat (`GET /api/v1/flags/evaluate/{key}`, no app name in
the URL) because an API token itself carries no app scoping, so there's
nothing to disambiguate a lookup by key alone.

## Rollout percentage

`enabled` is a hard kill switch: `false` means off for every caller,
full stop. When `enabled` is `true`, `rollout_percentage` (0-100) buckets
callers by hashing the flag's ID together with an optional caller-supplied
`identifier` query parameter (FNV-1a, taken mod 100), the same consistent-
hashing approach LaunchDarkly and Unleash both use for percentage
rollouts. This means the same identifier always lands on the same side of
a partial rollout, it's never a fresh coin flip on every call. Pass a
stable per-user or per-device value as `identifier` if you want a rollout
to stick per-user; omit it and every caller without one shares the same
outcome.

## Integration model

1. **Create a flag** (dashboard: an app's "Feature flags" tab, or the
   CLI):

   ```bash
   levelrail-cli flags create my-app --key new-checkout --name "New checkout" --rollout 25
   ```

2. **Create a read-scoped API token** (Settings -> Tokens, or
   `levelrail-cli tokens create --abilities read`), then inject it into
   your app as a secret env var, e.g. `FLAGS_TOKEN`.

3. **Call the evaluate endpoint from your app's own code**:

   ```bash
   curl -s \
     -H "Authorization: Bearer $FLAGS_TOKEN" \
     "https://your-control-plane/api/v1/flags/evaluate/new-checkout?identifier=user-123"
   ```

   Response:

   ```json
   { "key": "new-checkout", "enabled": true }
   ```

   That's the entire contract: no SDK exists yet (see below), so any
   language can call this with a plain HTTP client.

4. **Toggle it live**: flip the enabled switch or adjust the rollout
   slider in the dashboard (or `levelrail-cli flags set`), and the very
   next call to the evaluate endpoint reflects it, with no redeploy or
   restart of your app.

## API reference

| Method | Path | Ability |
| --- | --- | --- |
| `POST` | `/api/v1/apps/{name}/flags` | `write` |
| `GET` | `/api/v1/apps/{name}/flags` | `read` |
| `GET` | `/api/v1/apps/{name}/flags/{id}` | `read` |
| `PUT` | `/api/v1/apps/{name}/flags/{id}` | `write` |
| `DELETE` | `/api/v1/apps/{name}/flags/{id}` | `write` |
| `GET` | `/api/v1/flags/evaluate/{key}?identifier=...` | `read` |

The evaluate response is deliberately minimal (just `key` and `enabled`)
since it's the hot-path route an app may call on every request.

## CLI

```bash
levelrail-cli flags create <app> --key KEY --name NAME [--description DESC] [--disabled] [--rollout PERCENT]
levelrail-cli flags list <app>
levelrail-cli flags get <app> <id>
levelrail-cli flags set <app> <id> --name NAME [--description DESC] [--disabled] [--rollout PERCENT]
levelrail-cli flags delete <app> <id>
```

## Not built yet (deliberate follow-ups)

- **No language-specific SDK.** The evaluate endpoint is plain HTTP;
  wrapping it in a Go/Node/Python client is a real but separate piece of
  work.
- **No targeting rules beyond a flat rollout percentage.** No
  user-attribute-based targeting (e.g. "50% of users on plan X"), just
  the identifier-based consistent-hash bucketing described above.
- **No dedicated flag change history.** A create/update/delete against a
  flag is captured by the platform's existing generic audit log
  (`GET /api/v1/audit-log`) the same as any other authenticated write;
  there's no flag-specific history view beyond that.
