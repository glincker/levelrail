// Package agentinit renders the files "init" writes into a project: app.yaml,
// AGENTS.md and .mcp.json. All product and binary names come from Names, so
// nothing here hardcodes a brand.
package agentinit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/stackdetect"
)

// TokenEnvVar is the env var the MCP config references for the API token.
const TokenEnvVar = "APP_API_TOKEN" //nolint:gosec // env var name, not a credential

// URLEnvVar is the env var that overrides the API URL.
const URLEnvVar = "APP_API_URL"

// MCP modes init accepts. ModeAgentCore selects the small tool profile
// rather than a class mode.
const (
	ModeReadOnly  = "read-only"
	ModeStandard  = "standard"
	ModeFull      = "full"
	ModeAgentCore = "agent-core"
)

// Names carries every user-visible name, resolved by the caller.
type Names struct {
	// Product is the display name.
	Product string
	// CLI is the command name (os.Args[0]).
	CLI string
	// MCPBinary is the MCP server binary name.
	MCPBinary string
	// ServerKey is the key under mcpServers in .mcp.json.
	ServerKey string
}

// Options is everything the renderers need.
type Options struct {
	Names
	// APIURL is the control plane URL, empty when unknown.
	APIURL string
	// Mode is one of the Mode constants; empty means ModeAgentCore.
	Mode string
	// App is the service name to use in AGENTS.md examples.
	App string
}

// ValidMode reports whether m is an accepted MCP mode.
func ValidMode(m string) bool {
	switch m {
	case ModeReadOnly, ModeStandard, ModeFull, ModeAgentCore:
		return true
	}
	return false
}

// AppYAML renders an app.yaml for st and validates it with the platform's own
// spec parser before returning it.
func AppYAML(st stackdetect.Stack) (string, error) {
	if !st.Detected() {
		return "", fmt.Errorf("no stack detected, cannot write a spec")
	}
	var b strings.Builder
	b.WriteString("version: 1\nservices:\n")
	fmt.Fprintf(&b, "  %s:\n    build:\n      type: %s\n", st.Name, st.Build)
	if st.Path != "" {
		fmt.Fprintf(&b, "      path: %s\n", st.Path)
	}
	if st.Port > 0 {
		fmt.Fprintf(&b, "    port: %d\n", st.Port)
	}
	if st.Build != stackdetect.BuildCompose && st.Build != stackdetect.BuildStatic {
		b.WriteString("    health:\n      readiness: { path: /, interval: 5s, timeout: 2s }\n")
	}
	out := b.String()
	if _, err := spec.Parse([]byte(out)); err != nil {
		return "", fmt.Errorf("generated spec failed validation: %w", err)
	}
	return out, nil
}

type mcpServer struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env"`
}

// MCPArgs returns the MCP server arguments for mode.
func MCPArgs(mode string) []string {
	switch mode {
	case ModeReadOnly, ModeStandard, ModeFull:
		return []string{"--mode", mode}
	default:
		return []string{"--tool-profile", ModeAgentCore}
	}
}

// MCPJSON renders a stdio .mcp.json that references the token through an env
// var and never embeds it.
func MCPJSON(o Options) (string, error) {
	if o.Mode != "" && !ValidMode(o.Mode) {
		return "", fmt.Errorf("unknown mode %q, want read-only, standard, full or agent-core", o.Mode)
	}
	env := map[string]string{TokenEnvVar: "${" + TokenEnvVar + "}"}
	if o.APIURL != "" {
		env[URLEnvVar] = "${" + URLEnvVar + ":-" + o.APIURL + "}"
	}
	doc := map[string]map[string]mcpServer{
		"mcpServers": {o.ServerKey: {Command: o.MCPBinary, Args: MCPArgs(o.Mode), Env: env}},
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal mcp config: %w", err)
	}
	return string(raw) + "\n", nil
}

var agentsTmpl = template.Must(template.New("agents").Parse(agentsTemplate))

// AgentsMD renders the AGENTS.md instructions.
func AgentsMD(o Options) (string, error) {
	data := struct {
		Options
		TokenEnv, URLEnv string
		ModeLabel        string
		URL              string
	}{Options: o, TokenEnv: TokenEnvVar, URLEnv: URLEnvVar, ModeLabel: o.Mode, URL: o.APIURL}
	if data.ModeLabel == "" {
		data.ModeLabel = ModeAgentCore
	}
	if data.App == "" {
		data.App = "my-app"
	}
	var buf bytes.Buffer
	if err := agentsTmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("render AGENTS.md: %w", err)
	}
	return buf.String(), nil
}

const agentsTemplate = `# Deploying this project

This project deploys to your {{.Product}} instance{{if .URL}} at {{.URL}}{{end}}. Use the CLI ({{.CLI}}) or the MCP server ({{.MCPBinary}}, configured in .mcp.json, mode {{.ModeLabel}}). Both read the API token from ` + "`{{.TokenEnv}}`" + `{{if .URL}} and the URL from ` + "`{{.URLEnv}}`" + `{{end}}. Never write a token into a file, a commit or a chat message.

The app spec is app.yaml in this directory. Validate a change before you ship it.

## Deploy

1. Edit app.yaml or the code, commit, and push. A push to the tracked branch deploys through the webhook.
2. To deploy by hand: ` + "`{{.CLI}} apps deploy {{.App}}`" + `.
3. Preview a spec change without applying it: ` + "`{{.CLI}} apps preflight {{.App}}`" + ` (MCP: ` + "`preflight_app`, `plan_change`" + `).

## Wait for the result

Do not assume a deploy worked. Block until it converges:

` + "```" + `
{{.CLI}} apps wait {{.App}}
` + "```" + `

The exit code says whether it succeeded. MCP: ` + "`wait_for_deploy`" + `.

## Check status

` + "```" + `
{{.CLI}} apps status {{.App}}
{{.CLI}} attention
` + "```" + `

## Read a failure

` + "```" + `
{{.CLI}} apps deploys list {{.App}}
{{.CLI}} apps deploys show {{.App}} <deploy-id>
{{.CLI}} apps diagnose {{.App}}
{{.CLI}} apps logs {{.App}} --follow
` + "```" + `

` + "`apps deploys show`" + ` returns the structured failure (stage, reason, suggested fix). Read it before changing anything, and change one thing per attempt. Add ` + "`--json`" + ` for machine-readable output.

## Roll back

` + "```" + `
{{.CLI}} apps rollback {{.App}}
{{.CLI}} apps deploys rollback-to {{.App}} <deploy-id>
` + "```" + `

Roll back first when production is down, then investigate.

## Env vars and secrets

- Plain config: ` + "`{{.CLI}} apps env import {{.App}} --file .env --dry-run`" + `, then without ` + "`--dry-run`" + `.
- Secrets: put them in a git-ignored file and run ` + "`{{.CLI}} apps secrets set {{.App}} --env-file <path>`" + `, so values never appear in shell history or chat. Do not read secret values back or print them.
- Env changes take effect on the next restart: ` + "`{{.CLI}} apps apply {{.App}}`" + `.

## Rules for agents

- Use ` + "`--dry-run`" + ` or ` + "`plan_change`" + ` before anything destructive.
- Stay inside this app. Do not delete apps, databases or backups without being asked.
- If a command is denied, the token lacks that ability: report it instead of working around it.
`
