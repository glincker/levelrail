---
title: "Experimental features"
tag: "Experimental"
description: Features hidden at launch, the APP_EXPERIMENTAL switch that turns them on, and what each surface does while a feature is off.
"og:url": "https://docs.levelrail.com/experimental-features"
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
| `git-push` | Deploy an app with `git push` to a remote on your own server, per-app token ([Git push deploy](/git-push-deploy)), beta |
| `saml-scim` | SAML single sign-on and SCIM provisioning on the sign-in library ([SAML](/sso-saml), [SCIM](/scim)), beta, not yet verified against a live identity provider |
| `plugins` | [Plugins](/plugins): signed, operator-approved plugins in locked-down containers, starting with notification channels |
| `canvas` | [Canvas](/canvas) (beta): a visual project editor that turns a draft into a reviewable plan. Needs `iac` to review and apply |
| `resource-advisor` | [Resource advisor](/resource-advisor) (beta): limit, idle and tuning suggestions |
| `cli-console` | [CLI console](/cli-console) (beta): a web console for the platform CLI, never a host shell |
| `mobile-api` | [Mobile API](/mobile-api) (beta): phone pairing, device tokens and the mobile overview endpoint |
| `mcp-http` | [Remote MCP](/mcp) (experimental): a read-only MCP endpoint at `POST /mcp` for bearer token clients |
| `mail` | [Mail DNS](/mail-dns) (experimental): plan, apply and check the SPF, DKIM, DMARC, MX and related records for email |

Every feature has a switch in Settings, Beta features, and the choice is saved at once. `ai-chat`, `resource-advisor`, `cli-console`, `mobile-api`, `git-push`, `saml-scim` and `canvas` apply live. The rest are wired at server start: the card shows "Restart to apply" with a Restart now button (root, supervised installs), and `levelrail-cli settings beta-features set <feature> on --restart` does the same from the CLI. A feature listed in `APP_EXPERIMENTAL` shows as "Enabled by server setting" and stays on regardless of Settings. Turning a feature off never deletes its data.

Prefer config over the UI? Each card has "Use the CLI or server config instead", which shows the CLI command and the systemd and Docker lines to copy.

Features that are off are not hidden: they show as locked entries in the sidebar and command palette, and opening one lands on Settings, Beta features. `levelrail-cli settings beta-features list` prints the same list.

## Graduation criteria (proposed)

Proposed, not in force. A flag leaves this list when its row is met and the evidence is linked from [Feature status](/feature-status). "Real host" means a run on a fresh VPS, not a local Docker daemon. Status comes from `internal/experimental/catalog.go`.

| Key | Status | Proposed proof to graduate |
| --- | --- | --- |
| `git-push` | beta | A live e2e of push, build and deploy through the real API, plus one real host run with a public domain |
| `canvas` | beta | A live e2e of draft, plan and apply, plus one real host run. Graduates only with `iac` |
| `resource-advisor` | beta | A live e2e of suggest, apply and dismiss, plus one real host run with real usage history |
| `cli-console` | beta | A real host run, plus an authz review of the gate order, step-up and command policy |
| `mobile-api` | beta | A real host run with a real phone, plus an authz review of device token scope and revocation |
| `saml-scim` | beta | Interop tests against Okta and Entra ID, on top of the Keycloak and SimpleSAMLphp lab runs |
| `ai-chat` | beta | One live call to a real model provider, covering a tool call and an error path |
| `iac` | experimental | Stays experimental: apply changes many resources in one run. Needs a real host apply and a rollback story. Stacks are part of this flag |
| `load-balancer` | experimental | Stays experimental: proven on one machine only. Needs a multi-host run, which waits for multi-node to leave beta |
| `ai-models` | experimental | Stays experimental: needs NVIDIA hardware and a real GPU run. Every current test uses fakes |
| `plugins` | experimental | Stays experimental: needs a signing and approval review and a sandbox escape review |
| `mail` | experimental | Stays experimental: needs a run against real DNS providers and a real mail receiver |
| `mcp-http` | experimental | Stays experimental: read only and without OAuth. Needs an OAuth design and an authz review |
| `cloudflare-tunnel` | experimental | Stays experimental: reconciler unit tests only and it needs a real account |
| `access-roles` | experimental | No criteria stated. Proposed: an e2e where a custom role is denied a real action, and an authz review |
| `global-environments` | experimental | No criteria stated. Proposed: an e2e of the switcher with isolation between environments, and a migration check on an existing install |
| `ai-control` | experimental | No criteria stated. Proposed: an e2e where admin mode blocks and allows a real agent action, and an authz review |

## What "off" means

- API: routes of the feature answer `404` with `{"error": "...", "code": "experimental_feature_disabled", "feature": "<key>"}`. The message names `APP_EXPERIMENTAL`.
- CLI: the commands are hidden from `--help` and from shell completion, and exit with a usage error naming `APP_EXPERIMENTAL`. The CLI reads its own environment, not the control plane's: set the variable in the shell you run the CLI from, and regenerate any installed completion script after changing it.
- MCP: the tools are not registered. The MCP server is a separate process and reads its own environment: set the variable there, not only on the control plane.
- Dashboard: navigation entries, [command palette](/command-palette) entries, `g` shortcuts and in-page pointers are hidden, pages redirect home on direct visits. The dashboard follows the control plane's setting.
- Reconcilers: the model, [load balancer](/load-balancing) routing and Cloudflare Tunnel controllers do not start.
- Data: nothing is deleted. Turning a feature back on restores it as it was.

Set the variable on the control plane with `sudo systemctl edit levelrail` (`[Service]`, then `Environment=APP_EXPERIMENTAL=...`) and restart.
