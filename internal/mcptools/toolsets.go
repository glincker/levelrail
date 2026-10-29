package mcptools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	toolListToolsets   = "list_toolsets"
	toolEnableToolset  = "enable_toolset"
	toolDisableToolset = "disable_toolset"

	// EnvAllTools names the env var that restores the static, everything
	// listed at once tools/list for clients without list_changed support.
	EnvAllTools = "APP_MCP_ALL"

	// maxTrackedSessions bounds the per-session toolset map on a shared HTTP server.
	maxTrackedSessions = 256
)

// dynamicCoreTools is the always-listed core of a progressive session: the
// agent-core allowlist plus the lookups an agent needs before it can pick a toolset.
var dynamicCoreTools = func() map[string]struct{} {
	core := map[string]struct{}{"get_app": {}, "whoami": {}, "search": {}}
	for n := range agentCoreTools {
		core[n] = struct{}{}
	}
	return core
}()

// DynamicCoreTools returns the sorted always-listed core of a progressive session.
func DynamicCoreTools() []string {
	names := make([]string, 0, len(dynamicCoreTools))
	for n := range dynamicCoreTools {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func isToolsetMetaTool(name string) bool {
	return name == toolListToolsets || name == toolEnableToolset || name == toolDisableToolset
}

// toolsetDescriptions is one line per toolset; a test fails when a toolset has none.
var toolsetDescriptions = map[string]string{
	"account":             "Your identity, sessions and personal settings.",
	"alerts":              "Alert rules, firing alerts, silences and noise review.",
	"apps":                "App config, env, images, timeline and pending changes.",
	"audit":               "Audit log queries.",
	"backups":             "Database and volume backups, drills and targets.",
	"builds":              "Build history, logs and build cache.",
	"canary":              "Canary releases: start, inspect, promote and abort.",
	"dashboards":          "Custom dashboards.",
	"databases":           "Managed databases and access sessions.",
	"deploys":             "Deploy history, freezes, approvals, rollback and promotion.",
	"diagnostics":         "Failure investigation and resource advice.",
	"domains":             "Domains, certificates, DNS checks and Cloudflare.",
	"environments":        "Environments and their settings.",
	"flags":               "Feature flags.",
	"functions":           "Functions.",
	"global-environments": "Shared env sets across apps.",
	"iac":                 "Plan and apply declarative resource files.",
	"identity":            "SSO connections, SCIM status and group mappings (root).",
	"imports":             "Import plans for apps moving in from other platforms.",
	"migrate":             "Read-only migration status: import sessions, cutover plans and runs, data copies.",
	"security":            "Exposure report, export destination health, template requirements.",
	"inbox":               "Your notification inbox: list and unread count.",
	"iam":                 "Tokens, policies and access templates.",
	"images":              "Registry images and credentials.",
	"loadbalancer":        "Per-app load balancer settings and history.",
	"logs":                "Log search, archive, drains and signatures.",
	"metrics":             "App and node metrics, traffic and cost.",
	"models":              "AI models, serving and fit checks.",
	"nodes":               "Nodes, drains and moves.",
	"notifications":       "Notification channels and delivery.",
	"orgs":                "Organizations.",
	"pipelines":           "Pipelines and their runs.",
	"plugins":             "Plugin install and status.",
	"previews":            "Preview environments.",
	"registry":            "Registry credentials.",
	"scheduled":           "Scheduled tasks.",
	"search":              "Platform search and action search.",
	"settings":            "Platform settings.",
	"stacks":              "Stacks: plan and apply resource files with revision history.",
	"storage":             "Storage buckets and volumes.",
	"system":              "Control plane health, upgrades, DR and janitor.",
	"teams":               "Teams and members.",
	"templates":           "Service and project templates.",
	"webhooks":            "Webhook endpoints and deliveries.",
}

// toolsetSession tracks which toolsets each connected session enabled.
type toolsetSession struct {
	mu      sync.Mutex
	enabled map[mcp.Session]map[string]bool
}

func (s *toolsetSession) groups(sess mcp.Session) map[string]bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]bool, len(s.enabled[sess]))
	for g := range s.enabled[sess] {
		out[g] = true
	}
	return out
}

func (s *toolsetSession) set(server *mcp.Server, sess mcp.Session, group string, on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.enabled) >= maxTrackedSessions {
		live := map[mcp.Session]bool{}
		for ss := range server.Sessions() {
			live[ss] = true
		}
		for k := range s.enabled {
			if !live[k] {
				delete(s.enabled, k)
			}
		}
	}
	if s.enabled[sess] == nil {
		s.enabled[sess] = map[string]bool{}
	}
	if on {
		s.enabled[sess][group] = true
	} else {
		delete(s.enabled[sess], group)
	}
}

// toolsetCost is one toolset's size in the listing a client would pay for.
type toolsetCost struct {
	Tools  int
	Tokens int
}

// toolsetCosts measures every toolset the mode exposes by listing a throwaway static server once.
func toolsetCosts(client *apiclient.Client, opts Options) map[string]toolsetCost {
	static := opts
	static.Dynamic, static.Toolsets, static.Profile = false, nil, ""
	static.OmitOutputSchemas = true
	server, _ := NewServerWithOptions(client, static)
	st, ct := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = server.Run(ctx, st) }()
	session, err := mcp.NewClient(&mcp.Implementation{Name: "levelrail-mcp-costs", Version: "0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		return nil
	}
	defer func() { _ = session.Close() }()
	costs := map[string]toolsetCost{}
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			break
		}
		m, ok := Lookup(tool.Name)
		if !ok {
			continue
		}
		b, _ := json.Marshal(tool)
		c := costs[m.Group]
		c.Tools++
		c.Tokens += (len(b) + charsPerTokenEstimate - 1) / charsPerTokenEstimate
		costs[m.Group] = c
	}
	return costs
}

// charsPerTokenEstimate is the rough 4 characters per token the budget tests use.
const charsPerTokenEstimate = 4

type toolsetInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Tools       int    `json:"tools"`
	EstTokens   int    `json:"est_tokens"`
	Enabled     bool   `json:"enabled"`
}

type listToolsetsOutput struct {
	Toolsets []toolsetInfo `json:"toolsets"`
}

type toolsetNamesInput struct {
	Toolsets []string `json:"toolsets" jsonschema:"toolset names from list_toolsets"`
}

type toolsetChangeOutput struct {
	Changed     []string `json:"changed"`
	Enabled     []string `json:"enabled"`
	ToolsListed int      `json:"tools_listed"`
}

type toolsetController struct {
	server *mcp.Server
	client *apiclient.Client
	opts   Options
	state  *toolsetSession
	once   sync.Once
	costs  map[string]toolsetCost
}

func (c *toolsetController) cost() map[string]toolsetCost {
	c.once.Do(func() { c.costs = toolsetCosts(c.client, c.opts) })
	return c.costs
}

// exposed reports the toolsets that have at least one tool this mode lists.
func (c *toolsetController) exposed(ctx context.Context) map[string]bool {
	out := map[string]bool{}
	abilities := c.opts.probe.get(ctx)
	for name, m := range toolTable {
		if c.opts.allows(name, m) && abilityAllows(abilities, name, m) {
			out[m.Group] = true
		}
	}
	return out
}

func (c *toolsetController) listed(ctx context.Context, enabled map[string]bool) int {
	n := len(isToolsetMetaToolNames())
	abilities := c.opts.probe.get(ctx)
	for name, m := range toolTable {
		if !c.opts.allows(name, m) || !abilityAllows(abilities, name, m) {
			continue
		}
		if _, core := dynamicCoreTools[name]; core || enabled[m.Group] {
			n++
		}
	}
	return n
}

func isToolsetMetaToolNames() []string {
	return []string{toolListToolsets, toolEnableToolset, toolDisableToolset}
}

func sessionOf(req *mcp.CallToolRequest) mcp.Session {
	if req == nil || req.Session == nil {
		return nil
	}
	return req.Session
}

func (c *toolsetController) register() {
	ro := &mcp.ToolAnnotations{ReadOnlyHint: true}
	f := false
	mut := &mcp.ToolAnnotations{DestructiveHint: &f, IdempotentHint: true, OpenWorldHint: &f}
	mcp.AddTool(c.server, &mcp.Tool{
		Name:        toolListToolsets,
		Description: "List toolsets with tool count and token cost. Enable one with enable_toolset.",
		Annotations: ro,
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, listToolsetsOutput, error) {
		enabled := c.state.groups(sessionOf(req))
		costs := c.cost()
		var out listToolsetsOutput
		exposed := c.exposed(ctx)
		for _, g := range Groups() {
			if !exposed[g] {
				continue
			}
			out.Toolsets = append(out.Toolsets, toolsetInfo{
				Name: g, Description: toolsetDescriptions[g], Tools: costs[g].Tools,
				EstTokens: costs[g].Tokens, Enabled: enabled[g],
			})
		}
		return nil, out, nil
	})
	mcp.AddTool(c.server, &mcp.Tool{
		Name:        toolEnableToolset,
		Description: "Add toolsets' tools to this session's tool list.",
		Annotations: mut,
	}, func(ctx context.Context, req *mcp.CallToolRequest, in toolsetNamesInput) (*mcp.CallToolResult, toolsetChangeOutput, error) {
		return c.change(ctx, req, in, true)
	})
	mcp.AddTool(c.server, &mcp.Tool{
		Name:        toolDisableToolset,
		Description: "Remove toolsets' tools from this session's tool list.",
		Annotations: mut,
	}, func(ctx context.Context, req *mcp.CallToolRequest, in toolsetNamesInput) (*mcp.CallToolResult, toolsetChangeOutput, error) {
		return c.change(ctx, req, in, false)
	})
}

func (c *toolsetController) change(ctx context.Context, req *mcp.CallToolRequest, in toolsetNamesInput, on bool) (*mcp.CallToolResult, toolsetChangeOutput, error) {
	sess := sessionOf(req)
	if sess == nil || len(in.Toolsets) == 0 {
		return nil, toolsetChangeOutput{}, fmt.Errorf("toolsets is required, see list_toolsets")
	}
	exposed := c.exposed(ctx)
	names := make([]string, 0, len(in.Toolsets))
	for _, raw := range in.Toolsets {
		g := strings.ToLower(strings.TrimSpace(raw))
		if !exposed[g] {
			return nil, toolsetChangeOutput{}, fmt.Errorf("unknown toolset %q, see list_toolsets", raw)
		}
		names = append(names, g)
	}
	before := c.state.groups(sess)
	var changed []string
	for _, g := range names {
		if before[g] != on {
			changed = append(changed, g)
		}
		c.state.set(c.server, sess, g, on)
	}
	after := c.state.groups(sess)
	if len(changed) > 0 {
		c.register()
	}
	enabled := make([]string, 0, len(after))
	for g := range after {
		enabled = append(enabled, g)
	}
	sort.Strings(enabled)
	return nil, toolsetChangeOutput{Changed: changed, Enabled: enabled, ToolsListed: c.listed(ctx, after)}, nil
}

// middleware filters tools/list to the core plus the session's enabled
// toolsets and refuses calls to anything outside it. Registration is
// untouched, so the mode and the API token's own authz still decide the
// final outcome of every call.
func (c *toolsetController) middleware(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		switch method {
		case "tools/list":
			res, err := next(ctx, method, req)
			list, ok := res.(*mcp.ListToolsResult)
			if err != nil || !ok {
				return res, err
			}
			enabled := c.state.groups(req.GetSession())
			filtered := *list
			filtered.Tools = make([]*mcp.Tool, 0, len(list.Tools))
			for _, t := range list.Tools {
				if c.visible(t.Name, enabled) {
					filtered.Tools = append(filtered.Tools, t)
				}
			}
			return &filtered, nil
		case "tools/call":
			name := ""
			switch p := req.GetParams().(type) {
			case *mcp.CallToolParamsRaw:
				name = p.Name
			case *mcp.CallToolParams:
				name = p.Name
			}
			if msg := c.refusal(name, c.state.groups(req.GetSession())); msg != "" {
				return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: msg}}}, nil
			}
		}
		return next(ctx, method, req)
	}
}

func (c *toolsetController) visible(name string, enabled map[string]bool) bool {
	if isToolsetMetaTool(name) {
		return true
	}
	if _, core := dynamicCoreTools[name]; core {
		return true
	}
	m, ok := toolTable[name]
	return ok && enabled[m.Group]
}

func (c *toolsetController) refusal(name string, enabled map[string]bool) string {
	if c.visible(name, enabled) {
		return ""
	}
	m, ok := toolTable[name]
	if !ok {
		return ""
	}
	return fmt.Sprintf("tool %q is in the %q toolset, which is not enabled: call enable_toolset with {\"toolsets\":[%q]} first", name, m.Group, m.Group)
}

// installDynamicToolsets turns a fully registered server into a progressive one.
func installDynamicToolsets(server *mcp.Server, client *apiclient.Client, opts Options) {
	c := &toolsetController{server: server, client: client, opts: opts, state: &toolsetSession{enabled: map[mcp.Session]map[string]bool{}}}
	c.register()
	server.AddReceivingMiddleware(c.middleware)
}
