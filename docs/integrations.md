---
description: Attach curated third-party tools (PostHog, Sentry, Datadog, and more) to an app, injecting their env vars automatically at deploy time.
---

# Integrations

An integration is a curated third-party tool you can attach to an app from a small catalog. Attaching one stores the field values it needs (an API key, a DSN) through the same envelope encryption every app secret uses, and injects the tool's env vars into the app's container at its next build or deploy.

This is env-var injection only: there is no deep webhook or API wiring, and no auto-detection-gated suggestions. If a tool needs more than an env var to work, it isn't in this catalog.

## Attaching an integration

From the dashboard: open an app, go to its **Integrations** tab, and use **Attach integration** under **Third-party tools**. Search the catalog, pick a tool, fill in its required field(s), and save. The values you type are never shown again after saving.

From the CLI:

```sh
levelrail apps integrations catalog
levelrail apps integrations add my-app sentry --field SENTRY_DSN=https://key@o0.ingest.sentry.io/0
levelrail apps integrations list my-app
levelrail apps integrations remove my-app <attachment-id>
```

## Precedence

Integration env vars are injected as a default, the same tier as a project/organization/environment shared env var: if the app's own `app.yaml` or dashboard env already declares the same variable name, the app's own value always wins. Detaching an integration removes its env vars cleanly at the app's next deploy.

## Catalog

| Tool | What it does | Env var(s) | Setup |
| --- | --- | --- | --- |
| [Sentry](https://docs.sentry.io/platforms/node/configuration/options/) | Error tracking and performance monitoring. | `SENTRY_DSN` | Paste the DSN from your Sentry project's client keys settings. |
| [PostHog](https://posthog.com/docs/libraries/next-js) | Product analytics, feature flags, and session replay. | `NEXT_PUBLIC_POSTHOG_KEY`, `NEXT_PUBLIC_POSTHOG_HOST` (optional, defaults to PostHog Cloud US) | Paste your project API key from PostHog's project settings. |
| [Datadog](https://docs.datadoghq.com/tracing/trace_collection/library_config/nodejs/) | APM tracing, infrastructure, and log monitoring. | `DD_API_KEY`, `DD_SITE` (optional, defaults to `datadoghq.com`) | Paste an API key from Datadog's organization settings. |
| [Axiom](https://axiom.co/docs/send-data/nextjs) | Log and event data platform. | `AXIOM_TOKEN`, `AXIOM_DATASET` | Create an API token and dataset in Axiom, then paste both. |
| [Better Stack](https://betterstack.com/docs/logs/javascript/install/) | Log management and uptime monitoring. | `LOGTAIL_SOURCE_TOKEN` | Paste the source token from your Better Stack log source. |
| [LogSnag](https://docs.logsnag.com/quick-start) | Real-time event notifications and analytics. | `LOGSNAG_TOKEN`, `LOGSNAG_PROJECT` | Paste an API token and your project name from LogSnag's settings. |
| [BugSnag](https://docs.bugsnag.com/platforms/javascript/) | Error monitoring and stability management. | `BUGSNAG_API_KEY` | Paste the API key from your BugSnag project's settings. |
| [New Relic](https://docs.newrelic.com/docs/apm/agents/nodejs-agent/installation-configuration/install-nodejs-agent/) | Full-stack observability and APM. | `NEW_RELIC_LICENSE_KEY` | Paste your account's license key from New Relic's API keys page. |

Every entry links to that vendor's own current setup docs; the env var names above are read directly from those docs, not guessed.

## How this compares

Vercel and Netlify both ship an integrations marketplace of their own; this one is simpler and self-hosted, limited to env-var injection rather than a full OAuth/webhook connection to each vendor's platform.
