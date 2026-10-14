---
title: "Use OneUptime with Levelrail: env var injection"
sidebarTitle: "OneUptime"
description: "Attach OneUptime to an app and have its environment variables injected at the next deploy."
---

Attach OneUptime to an app and have its environment variables injected at the next deploy.

This page covers how to export OpenTelemetry data from an app to OneUptime.

## What you get

- Env vars `OTEL_EXPORTER_OTLP_HEADERS`, `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_SERVICE_NAME` injected into the app's container.
- The values stored with the same envelope encryption as app secrets.
- Removal on detach, effective at the app's next deploy.

## Set up

1. Prepare the credential. A header carrying your service token, for example `x-oneuptime-token=<token>`.
2. In the dashboard, open **Apps > (your app) > Integrations tab > Attach integration**.
3. Redeploy the app, then confirm the variable is present in the running container's environment.

From the CLI:

```bash
levelrail-cli apps integrations add my-app oneuptime --field OTEL_EXPORTER_OTLP_HEADERS=<VALUE>
levelrail-cli apps integrations list my-app
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| Vendor side | Whatever key the vendor needs; Levelrail only injects the value. The endpoint defaults to https://oneuptime.com/otlp; override it for a self-hosted OneUptime. |

## Endpoint and format

Injected variables: `OTEL_EXPORTER_OTLP_HEADERS`, `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_SERVICE_NAME`.

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

**A required field is rejected.** Required fields must be filled; optional ones fall back to the catalog default. The endpoint defaults to https://oneuptime.com/otlp; override it for a self-hosted OneUptime.

## FAQ

**Is the value shown again after saving?**

No. The values you type are never shown again.

**Does Levelrail install the SDK?**

No. Your app brings the SDK; Levelrail only provides its environment variables.

## Related

- [Datadog](/integrations/app-integrations/datadog): App integrations
- [Axiom](/integrations/app-integrations/axiom): App integrations
- [All app integrations](/integrations/app-integrations)
- [OneUptime documentation](https://oneuptime.com/docs), maintained by the vendor
