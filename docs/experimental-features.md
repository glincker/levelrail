---
title: "Experimental features"
tag: "Experimental"
description: Features hidden at launch, the APP_EXPERIMENTAL switch that turns them on, and what each surface does while a feature is off.
---

Some features have unit tests but no end-to-end or real-infrastructure evidence (see [feature status](/feature-status)). They are off by default. To turn them on, set one env var:

```
APP_EXPERIMENTAL=ai-models,load-balancer
```

The value is a comma separated list of feature keys. Unknown keys are logged at startup and ignored.

| Key | Feature |
| --- | --- |
| `ai-chat` | In-app AI assistant chat and its settings |
| `ai-models` | AI models on GPU nodes |
| `ai-control` | Full control over AI agent access (admin mode) |
| `access-roles` | Custom roles, guest users, environment grants and role storage |
| `global-environments` | Global environments, environment kinds and switcher |
| `load-balancer` | Load balancer across an app's replicas |
| `iac` | Platform as code: `apply`, `diff`, `export` |
| `cloudflare-tunnel` | Cloudflare Tunnel |
| `saml-scim` | SAML single sign-on and SCIM provisioning on the sign-in library ([SAML](/sso-saml), [SCIM](/scim)), beta, not yet verified against a live identity provider |
| `resource-advisor` | [Resource advisor](/resource-advisor) (beta): limit, idle and tuning suggestions |

Beta features can also be switched on without a restart from Settings, Beta features, or with `levelrail-cli settings beta-features enable <feature>`. That covers `ai-chat`, `resource-advisor`, `cli-console`, `mobile-api` and `saml-scim`. The rest are wired at startup and need `APP_EXPERIMENTAL` plus a restart. A feature listed in `APP_EXPERIMENTAL` stays on regardless of Settings.

Features that are off are not hidden: they show as locked entries in the sidebar and command palette, and opening one lands on Settings, Beta features with the switch or the exact env line to set. `levelrail-cli settings beta-features list` prints the same list.

## What "off" means

- API: routes of the feature answer `404` with `{"error": "...", "code": "experimental_feature_disabled", "feature": "<key>"}`. The message names `APP_EXPERIMENTAL`.
- CLI: the commands are hidden from `--help` and from shell completion, and exit with a usage error naming `APP_EXPERIMENTAL`. The CLI reads its own environment, not the control plane's: set the variable in the shell you run the CLI from, and regenerate any installed completion script after changing it.
- MCP: the tools are not registered. The MCP server is a separate process and reads its own environment: set the variable there, not only on the control plane.
- Dashboard: navigation entries, [command palette](/command-palette) entries, `g` shortcuts and in-page pointers are hidden, pages redirect home on direct visits. The dashboard follows the control plane's setting.
- Reconcilers: the model, [load balancer](/load-balancing) routing and Cloudflare Tunnel controllers do not start.
- Data: nothing is deleted. Turning a feature back on restores it as it was.

Set the variable on the control plane with `sudo systemctl edit levelrail` (`[Service]`, then `Environment=APP_EXPERIMENTAL=...`) and restart.
