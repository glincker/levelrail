---
title: "Use Plausible with Levelrail: env var injection"
sidebarTitle: "Plausible"
description: "Attach Plausible to an app and have its environment variables injected at the next deploy. Self-hosted Plausible works through the API host field."
---

Attach Plausible to an app and have its environment variables injected at the next deploy. Self-hosted Plausible works through the API host field.

This page covers how to inject Plausible web analytics settings into an app.

## What you get

- Env vars `NEXT_PUBLIC_PLAUSIBLE_DOMAIN`, `NEXT_PUBLIC_PLAUSIBLE_API_HOST` injected into the app's container.
- The values stored with the same envelope encryption as app secrets.
- Removal on detach, effective at the app's next deploy.

## Set up

1. Prepare the credential. The site domain registered in Plausible; the API host defaults to https://plausible.io.
2. In the dashboard, open **Apps > (your app) > Integrations tab > Attach integration**.
3. Redeploy the app, then confirm the variable is present in the running container's environment.

From the CLI:

```bash
levelrail-cli apps integrations add my-app plausible --field NEXT_PUBLIC_PLAUSIBLE_DOMAIN=<VALUE>
levelrail-cli apps integrations list my-app
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| Vendor side | Whatever key the vendor needs; Levelrail only injects the value. Set the API host for a self-hosted Plausible. |

## Endpoint and format

Injected variables: `NEXT_PUBLIC_PLAUSIBLE_DOMAIN`, `NEXT_PUBLIC_PLAUSIBLE_API_HOST`.

## Security notes

- Values are stored with the same envelope encryption as every app secret and are never shown again after saving.
- They are injected as defaults: a value in the app's own app.yaml or env wins.
- Nothing you enter here is shown again after you save it.

## Limits and caveats

- This is env var injection only. There is no webhook or OAuth wiring with the vendor.
- If the app's own app.yaml or dashboard env declares the same name, the app's value wins.

## Troubleshooting

**The variable is missing in the running app.** Integration env vars apply at the next build or deploy. Redeploy after attaching.

**The app uses a different value than I attached.** An env var of the same name in app.yaml or the app's env overrides the integration's default.

**A required field is rejected.** Required fields must be filled; optional ones fall back to the catalog default. Set the API host for a self-hosted Plausible.

## FAQ

**Is the value shown again after saving?**

No. The values you type are never shown again.

**Does Levelrail install the SDK?**

No. Your app brings the SDK; Levelrail only provides its environment variables.

## Related

- [PostHog](/integrations/app-integrations/posthog): App integrations
- [Datadog](/integrations/app-integrations/datadog): App integrations
- [All app integrations](/integrations/app-integrations)
- [Plausible documentation](https://plausible.io/docs), maintained by the vendor
