---
description: Features hidden at launch, the APP_EXPERIMENTAL switch that turns them on, and what each surface does while a feature is off.
---

# Experimental features

Some features have unit tests but no end-to-end or real-infrastructure evidence (see [feature status](feature-status.md)). They are off by default. Turn them on with one env var:

```
APP_EXPERIMENTAL=ai-models,load-balancer
```

The value is a comma separated list of feature keys. Unknown keys are logged at startup and ignored.

| Key | Feature |
| --- | --- |
| `ai-chat` | In-app AI assistant chat and its settings |
| `ai-models` | AI models on GPU nodes |
| `load-balancer` | Load balancer across an app's replicas |
| `iac` | Platform as code: `apply`, `diff`, `export` |
| `cloudflare-tunnel` | Cloudflare Tunnel |

## What "off" means

- **API**: routes of the feature answer `404` with `{"error": "...", "code": "experimental_feature_disabled", "feature": "<key>"}`. The message names `APP_EXPERIMENTAL`.
- **CLI**: the commands are hidden from `--help` and from shell completion, and exit with a usage error naming `APP_EXPERIMENTAL`. The CLI reads its own environment, not the control plane's: set the variable in the shell you run the CLI from, and regenerate any installed completion script after changing it.
- **MCP**: the tools are not registered. The MCP server is a separate process and reads its own environment: set the variable there, not only on the control plane.
- **Dashboard**: navigation entries, command palette entries, `g` shortcuts and in-page pointers are hidden, pages redirect home on direct visits. The dashboard follows the control plane's setting.
- **Reconcilers**: the model, load balancer routing and Cloudflare Tunnel controllers do not start.
- **Data**: nothing is deleted. Turning a feature back on restores it as it was.

Set the variable on the control plane with `sudo systemctl edit levelrail` (`[Service]`, then `Environment=APP_EXPERIMENTAL=...`) and restart.
