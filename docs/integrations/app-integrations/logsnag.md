---
title: "Use LogSnag with Levelrail: env var injection"
sidebarTitle: "LogSnag"
description: "Attach LogSnag to an app and have its environment variables injected at the next deploy."
---

Attach LogSnag to an app and have its environment variables injected at the next deploy.

This page covers how to inject LogSnag token and project into an app.

## What you get

- Env vars `LOGSNAG_TOKEN`, `LOGSNAG_PROJECT` injected into the app's container.
- The values stored with the same envelope encryption as app secrets.
- Removal on detach, effective at the app's next deploy.

## Set up

1. Prepare the credential. An API token and the project name from LogSnag's settings; both are required.
2. In the dashboard, open **Apps > (your app) > Integrations tab > Attach integration**.
3. Redeploy the app, then confirm the variable is present in the running container's environment.

From the CLI:

```bash
levelrail-cli apps integrations add my-app logsnag --field LOGSNAG_TOKEN=<VALUE>
levelrail-cli apps integrations list my-app
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| Vendor side | Whatever key the vendor needs; Levelrail only injects the value. The project field is the project name as shown in LogSnag. |

## Endpoint and format

Injected variables: `LOGSNAG_TOKEN`, `LOGSNAG_PROJECT`.

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

**A required field is rejected.** Required fields must be filled; optional ones fall back to the catalog default. The project field is the project name as shown in LogSnag.

## FAQ

**Is the value shown again after saving?**

No. The values you type are never shown again.

**Does Levelrail install the SDK?**

No. Your app brings the SDK; Levelrail only provides its environment variables.

## Related

- [Better Stack](/integrations/app-integrations/betterstack): App integrations
- [Sentry](/integrations/app-integrations/sentry): App integrations
- [All app integrations](/integrations/app-integrations)
- [LogSnag documentation](https://docs.logsnag.com/), maintained by the vendor
