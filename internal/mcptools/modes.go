package mcptools

import (
	"fmt"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/GLINCKER/levelrail/internal/experimental"
)

// Mode bounds which tool classes a server registers. The API token's own
// abilities still apply on top: a mode only shrinks what the model sees.
type Mode string

const (
	// ModeReadOnly registers read tools only.
	ModeReadOnly Mode = "read-only"
	// ModeStandard registers read and mutating tools, not destructive ones.
	ModeStandard Mode = "standard"
	// ModeFull registers every tool.
	ModeFull Mode = "full"

	// EnvMode names the env var that selects the Mode.
	EnvMode = "APP_MCP_MODE"
	// EnvToolsets names the env var that selects the toolsets.
	EnvToolsets = "APP_MCP_TOOLSETS"
	// EnvToolProfile names the env var that selects the tool profile.
	EnvToolProfile = "APP_MCP_TOOL_PROFILE"

	// ProfileAgentCore is a small allowlist of the tools an agent needs to
	// ship and debug an app.
	ProfileAgentCore = "agent-core"
)

// agentCoreTools is the agent-core allowlist. Names not yet registered are
// ignored, so the profile tolerates tools that have not landed.
var agentCoreTools = map[string]struct{}{
	"list_apps": {}, "get_app_status": {}, "get_attention": {}, "deploy_app": {},
	"list_deploys": {}, "cancel_deploy": {}, "diagnose_app_failure": {},
	"get_app_logs": {}, "preflight_app": {}, "rollback_app": {},
	"set_app_env": {}, "unset_app_env": {}, "list_domains": {}, "set_app_domains": {},
}

// AgentCoreTools returns the sorted agent-core allowlist.
func AgentCoreTools() []string {
	names := make([]string, 0, len(agentCoreTools))
	for n := range agentCoreTools {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// ParseToolProfile validates a tool profile name; empty means no profile.
func ParseToolProfile(s string) (string, error) {
	switch p := strings.ToLower(strings.TrimSpace(s)); p {
	case "", ProfileAgentCore:
		return p, nil
	default:
		return "", fmt.Errorf("unknown mcp tool profile %q, want %q", s, ProfileAgentCore)
	}
}

// ParseMode parses a mode name, defaulting an empty value to standard.
func ParseMode(s string) (Mode, error) {
	switch m := Mode(strings.ToLower(strings.TrimSpace(s))); m {
	case "":
		return ModeStandard, nil
	case ModeReadOnly, ModeStandard, ModeFull:
		return m, nil
	default:
		return "", fmt.Errorf("unknown mcp mode %q, want %q, %q or %q", s, ModeReadOnly, ModeStandard, ModeFull)
	}
}

// ParseToolsets parses a comma separated toolset list; empty means all.
func ParseToolsets(s string) ([]string, error) {
	valid := map[string]struct{}{}
	for _, g := range Groups() {
		valid[g] = struct{}{}
	}
	var out []string
	for _, part := range strings.Split(s, ",") {
		g := strings.ToLower(strings.TrimSpace(part))
		if g == "" {
			continue
		}
		if _, ok := valid[g]; !ok {
			return nil, fmt.Errorf("unknown mcp toolset %q, want one of %s", g, strings.Join(Groups(), ", "))
		}
		out = append(out, g)
	}
	return out, nil
}

// Options selects the tools a server exposes.
type Options struct {
	Mode     Mode
	Toolsets []string
	// Profile, when set, replaces the mode's class filter with an allowlist.
	// Read-only mode still hides every non-read tool.
	Profile string
}

// Summary reports what a server exposes.
type Summary struct {
	Mode     Mode
	Toolsets []string
	Profile  string
	Total    int
	Read     int
	Mutate   int
	Destruct int
}

// experimentalFeature returns the gated feature a tool belongs to, if any.
func experimentalFeature(name string, m Meta) (experimental.Feature, bool) {
	switch {
	case m.Group == "models":
		return experimental.AIModels, true
	case m.Group == "loadbalancer":
		return experimental.LoadBalancer, true
	case m.Group == "iac":
		return experimental.IaC, true
	case name == "get_cloudflare_tunnel_status":
		return experimental.CloudflareTunnel, true
	}
	return "", false
}

func (o Options) allows(name string, m Meta) bool {
	if f, gated := experimentalFeature(name, m); gated && !experimental.Enabled(f) {
		return false
	}
	if o.Profile == ProfileAgentCore {
		if _, ok := agentCoreTools[name]; !ok {
			return false
		}
		if o.Mode == ModeReadOnly && m.Class != ClassRead {
			return false
		}
		return o.allowsToolset(m)
	}
	switch o.Mode {
	case ModeReadOnly:
		if m.Class != ClassRead {
			return false
		}
	case ModeFull:
	default:
		if m.Class == ClassDestructive {
			return false
		}
	}
	return o.allowsToolset(m)
}

func (o Options) allowsToolset(m Meta) bool {
	if len(o.Toolsets) == 0 {
		return true
	}
	for _, g := range o.Toolsets {
		if g == m.Group {
			return true
		}
	}
	return false
}

func applyOptions(server *mcp.Server, opts Options) Summary {
	sum := Summary{Mode: opts.Mode, Toolsets: opts.Toolsets, Profile: opts.Profile}
	var hidden []string
	for name, m := range toolTable {
		if !opts.allows(name, m) {
			hidden = append(hidden, name)
			continue
		}
		sum.Total++
		switch m.Class {
		case ClassRead:
			sum.Read++
		case ClassMutate:
			sum.Mutate++
		case ClassDestructive:
			sum.Destruct++
		}
	}
	sort.Strings(hidden)
	server.RemoveTools(hidden...)
	return sum
}
