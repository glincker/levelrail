---
title: "Use BugSnag with Levelrail: env var injection"
sidebarTitle: "BugSnag"
description: "Attach BugSnag to an app and have its environment variables injected at the next deploy."
---

Attach BugSnag to an app and have its environment variables injected at the next deploy.

This page covers how to inject a BugSnag API key into an app.

## What you get

- Env vars `BUGSNAG_API_KEY` injected into the app's container.
- The values stored with the same envelope encryption as app secrets.
- Removal on detach, effective at the app's next deploy.

## Set up

1. Prepare the credential. The API key from your BugSnag project's settings.
2. In the dashboard, open **Apps > (your app) > Integrations tab > Attach integration**.
3. Redeploy the app, then confirm the variable is present in the running container's environment.

From the CLI:

```bash
levelrail-cli apps integrations add my-app bugsnag --field BUGSNAG_API_KEY=<VALUE>
levelrail-cli apps integrations list my-app
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| Vendor side | Whatever key the vendor needs; Levelrail only injects the value. Use the project's notifier API key. |

## Endpoint and format

Injected variables: `BUGSNAG_API_KEY`.

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

**A required field is rejected.** Required fields must be filled; optional ones fall back to the catalog default. Use the project's notifier API key.

## FAQ

**Is the value shown again after saving?**

No. The values you type are never shown again.

**Does Levelrail install the SDK?**

No. Your app brings the SDK; Levelrail only provides its environment variables.

## Related

- [Sentry](/integrations/app-integrations/sentry): App integrations
- [GlitchTip](/integrations/app-integrations/glitchtip): App integrations
- [All app integrations](/integrations/app-integrations)
- [BugSnag documentation](https://docs.bugsnag.com/), maintained by the vendor
