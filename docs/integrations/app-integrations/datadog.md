---
title: "Use Datadog with Levelrail: env var injection"
sidebarTitle: "Datadog"
description: "Attach Datadog to an app and have its environment variables injected at the next deploy."
---

Attach Datadog to an app and have its environment variables injected at the next deploy.

This page covers how to inject a Datadog API key and site into an app.

## What you get

- Env vars `DD_API_KEY`, `DD_SITE` injected into the app's container.
- The values stored with the same envelope encryption as app secrets.
- Removal on detach, effective at the app's next deploy.

## Set up

1. Prepare the credential. An API key from Datadog's organization settings; the site defaults to datadoghq.com.
2. In the dashboard, open **Apps > (your app) > Integrations tab > Attach integration**.
3. Redeploy the app, then confirm the variable is present in the running container's environment.

From the CLI:

```bash
levelrail-cli apps integrations add my-app datadog --field DD_API_KEY=<VALUE>
levelrail-cli apps integrations list my-app
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| Vendor side | Whatever key the vendor needs; Levelrail only injects the value. Set DD_SITE to match the Datadog region of your account. |

## Endpoint and format

Injected variables: `DD_API_KEY`, `DD_SITE`.

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

**A required field is rejected.** Required fields must be filled; optional ones fall back to the catalog default. Set DD_SITE to match the Datadog region of your account.

## FAQ

**Is the value shown again after saving?**

No. The values you type are never shown again.

**Does Levelrail install the SDK?**

No. Your app brings the SDK; Levelrail only provides its environment variables.

## Related

- [New Relic](/integrations/app-integrations/newrelic): App integrations
- [OneUptime](/integrations/app-integrations/oneuptime): App integrations
- [All app integrations](/integrations/app-integrations)
- [Datadog documentation](https://docs.datadoghq.com/), maintained by the vendor
