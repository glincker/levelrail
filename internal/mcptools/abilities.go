package mcptools

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	abilityRead           = "read"
	abilityReadSensitive  = "read:sensitive"
	abilityWrite          = "write"
	abilityWriteSensitive = "write:sensitive"
	abilityDeploy         = "deploy"
	abilityRoot           = "root"

	abilityProbeTimeout = 3 * time.Second
	abilityProbeTTL     = 30 * time.Second
)

// deployTierTools are the tools whose API route needs the deploy ability, not write.
var deployTierTools = map[string]struct{}{
	"deploy_app": {}, "deploy_compose": {}, "deploy_template": {}, "rollback_app": {}, "rollback_app_to_deploy": {},
	"cancel_deploy": {}, "start_canary": {}, "promote_canary": {}, "abort_canary": {},
	"restart_app": {}, "promote_app": {}, "approve_deploy_approval": {}, "reject_deploy_approval": {},
}

// rootTierTools are read tools whose API route needs the root ability, so a read token cannot call them.
var rootTierTools = map[string]struct{}{
	"get_security_reachability": {}, "list_export_destinations": {},
	"list_sso_connections": {}, "get_scim_status": {}, "list_sso_group_mappings": {},
}

// anyAuthTools answer for any authenticated token, whatever its abilities.
var anyAuthTools = map[string]struct{}{"whoami": {}, aiControlStatusTool: {}}

// requiredAbilities returns the abilities that can each satisfy a tool, any one of them.
func requiredAbilities(name string, m Meta) []string {
	if _, ok := anyAuthTools[name]; ok || isToolsetMetaTool(name) {
		return nil
	}
	if _, ok := rootTierTools[name]; ok {
		return []string{abilityRoot}
	}
	if m.Class == ClassRead {
		return []string{abilityRead, abilityReadSensitive}
	}
	if _, ok := deployTierTools[name]; ok {
		return []string{abilityDeploy}
	}
	return []string{abilityWrite, abilityWriteSensitive}
}

// abilityAllows reports whether a token holding abilities can call the tool. Empty abilities means unknown and allows all.
func abilityAllows(abilities []string, name string, m Meta) bool {
	if len(abilities) == 0 || slices.Contains(abilities, abilityRoot) {
		return true
	}
	need := requiredAbilities(name, m)
	if len(need) == 0 {
		return true
	}
	for _, a := range need {
		if slices.Contains(abilities, a) {
			return true
		}
	}
	return false
}

// abilityProbe reads the connected token's abilities, briefly cached.
type abilityProbe struct {
	client *apiclient.Client
	mu     sync.Mutex
	at     time.Time
	cached []string
}

// get returns nil when the abilities cannot be trusted to narrow the list: a read
// failure, or IAM policies that can grant resource-scoped access beyond the base abilities.
func (p *abilityProbe) get(ctx context.Context) []string {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.at.IsZero() && time.Since(p.at) < abilityProbeTTL {
		return p.cached
	}
	ctx, cancel := context.WithTimeout(ctx, abilityProbeTimeout)
	defer cancel()
	res, err := p.client.MyAccess(ctx)
	p.cached = nil
	if err == nil && len(res.Policies) == 0 {
		p.cached = res.Abilities
	}
	p.at = time.Now()
	return p.cached
}

// abilityMiddleware narrows tools/list to what the token can call. Calls are untouched: the server still decides each one.
func abilityMiddleware(p *abilityProbe) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method != "tools/list" {
				return next(ctx, method, req)
			}
			res, err := next(ctx, method, req)
			list, ok := res.(*mcp.ListToolsResult)
			if err != nil || !ok {
				return res, err
			}
			abilities := p.get(ctx)
			if len(abilities) == 0 {
				return res, nil
			}
			filtered := *list
			filtered.Tools = make([]*mcp.Tool, 0, len(list.Tools))
			for _, t := range list.Tools {
				m, known := toolTable[t.Name]
				if !known || abilityAllows(abilities, t.Name, m) {
					filtered.Tools = append(filtered.Tools, t)
				}
			}
			return &filtered, nil
		}
	}
}

var needsAbilityPattern = regexp.MustCompile(`\(needs ([a-z:]+)\)`)

// forbiddenHint turns an ability 403 into a short structured error naming the missing ability and a read-only tool to use instead.
func forbiddenHint(tool string, err error) error {
	var apiErr *apiclient.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusForbidden {
		return err
	}
	match := needsAbilityPattern.FindStringSubmatch(apiErr.Message)
	if match == nil {
		return err
	}
	alt := "whoami"
	if m, ok := toolTable[tool]; ok && m.Class != ClassRead {
		alt = "plan_change"
	}
	return fmt.Errorf(`forbidden: {"missing_ability":%q,"use_instead":%q}`, match[1], alt)
}

// withForbiddenHint rewrites an ability 403 from a tool handler into forbiddenHint's short form.
func withForbiddenHint[In, Out any](tool string, h mcp.ToolHandlerFor[In, Out]) mcp.ToolHandlerFor[In, Out] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
		res, out, err := h(ctx, req, in)
		if err != nil {
			err = forbiddenHint(tool, err)
		}
		return res, out, err
	}
}
