---
title: "Install with your AI agent"
icon: "/assets/icons/sparkle.svg"
sidebarTitle: "Install with an AI agent"
description: "Copy-paste prompts that let Claude Code, Cursor or another agent install Levelrail over SSH, connect through MCP with a read-only token and deploy a first app."
"og:url": "https://docs.levelrail.com/install-with-ai-agent"
---

Outcome: your agent installs Levelrail on a fresh server over SSH, you approve the sign-in yourself, and the agent then reads and deploys through MCP under a scoped token.

<Note>
Before you start: a Linux server you can SSH into as root or a `sudo` user ([What you need](/what-you-need)), an agent that can run shell commands (Claude Code, Cursor or similar), and ports 80 and 443 open. You choose the admin password. The agent never needs it.
</Note>

<Steps>
<Step title="Install on a fresh server over SSH">

Paste this into your agent. Replace the address.

```text
SSH to root@203.0.113.10 and install Levelrail:
curl -fsSL https://levelrail.com/install.sh | sudo sh
Print the dashboard setup link it shows and tell me. I choose the admin password myself.
Do not write any token or password to a file or into this chat.
Then run `levelrail-cli doctor` on the server and report anything that fails.
```

To check the script first, use the verified download in [Getting started](/getting-started#1-install) instead. Back up `/var/lib/levelrail-data/master.key` afterwards.

</Step>
<Step title="Sign in from your machine">

On your own machine, with `levelrail-cli` installed ([Installing just the CLI](/installing#installing-just-the-cli)):

```bash
levelrail-cli auth login --device --api-url https://console.example.com
```

It prints a code. Approve it in the dashboard. The agent does not approve sign-ins for itself.

</Step>
<Step title="Create a read-only token for the agent">

```bash
levelrail-cli tokens create --name my-agent --agent "my-agent" --abilities read
```

Copy the token once, it is not shown again. `read` is the default. Give an agent token an agent name so [AI control](/ai-control) can govern it. A fresh instance starts with agents off, so set a mode first:

```bash
levelrail-cli ai-control set --mode observe
```

</Step>
<Step title="Connect over MCP">

```bash
levelrail-mcp --print-config claude
```

Use `cursor`, `vscode` or `zed` for those clients. It prints a JSON block with a token placeholder and read-only mode. Paste it into the client's MCP config and put your token in `APP_API_TOKEN`. Details: [Connect an AI assistant safely](/mcp-safe-setup) and [AI assistant integration](/ai-assistant).

</Step>
<Step title="Deploy a first app">

Writes need a wider token and mode. Create a second token with `--abilities read,deploy`, set `levelrail-cli ai-control set --mode operate`, then paste:

```text
Using the Levelrail MCP tools, list my apps, then deploy the template uptime-kuma as my-status-page
and report its status. Ask me before any change that is not a deploy.
```

</Step>
</Steps>

## Verify it works

Ask the agent to run `whoami` and `list_apps` through MCP. With a read-only token it should see read tools only. From a terminal:

```bash
levelrail-cli apps status my-status-page
levelrail-cli audit-log --agent my-agent
```

Expected: the app is healthy and the audit log shows what the agent did.

<Accordion title="Troubleshooting">

| What you see | What to do |
| --- | --- |
| `agent access is disabled by an administrator` | The AI control mode is `off`. Run `levelrail-cli ai-control set --mode observe`. |
| `agent access is limited by the current AI control mode` | The mode does not allow that ability. Use `operate` for deploys. |
| The agent cannot see the server | It must SSH first. Check the address and key. |
| The MCP client cannot start the server | `levelrail-mcp` must be on the client's `PATH`. See [AI assistant integration](/ai-assistant). |
| No setup token in the output | Run `sudo APP_DATA_DIR=/var/lib/levelrail-data levelrail setup-token` on the server. |

</Accordion>

## Next steps

<CardGroup cols={2}>
<Card title="AI control" href="/ai-control">

Set what agents may do on your instance.

</Card>
<Card title="Working with AI agents" href="/agents">

The Levelrail skill and agent workflows.

</Card>
</CardGroup>
