package api

import (
	"context"
	"slices"
	"strings"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Default scope policy for tokens minted without explicit abilities, the way an
// MCP client token usually is. Read-only unless an operator widens it.
const (
	// KeyMCPDefaultTokenScope is the runtime setting key.
	KeyMCPDefaultTokenScope = "mcp.default_token_scope" //nolint:gosec // setting key, not a credential
	// EnvMCPDefaultTokenScope only seeds the default.
	EnvMCPDefaultTokenScope = "APP_MCP_DEFAULT_TOKEN_SCOPE" //nolint:gosec // env var name, not a credential

	tokenScopeReadOnly  = "read_only"
	tokenScopeReadWrite = "read_write"

	// tokenAccessReadOnly and tokenAccessCanChange are the wire labels on a token.
	tokenAccessReadOnly  = "read_only"
	tokenAccessCanChange = "can_change"
)

func init() {
	runtimesettings.Register(runtimesettings.Definition{
		Key: KeyMCPDefaultTokenScope, Category: runtimesettings.CategorySecurity, Kind: runtimesettings.KindString,
		Default: tokenScopeReadOnly, Env: EnvMCPDefaultTokenScope, Apply: runtimesettings.ApplyLive,
		AllowedStrings: []string{tokenScopeReadOnly, tokenScopeReadWrite},
		StrengthOrder:  []string{tokenScopeReadWrite, tokenScopeReadOnly},
		Risk:           runtimesettings.RiskWeakensSecurity,
		FailClosed:     true,
		EnvConvert:     func(raw string) (string, error) { return strings.ToLower(strings.TrimSpace(raw)), nil },
		TitleKey:       "runtimeSettings:items.mcpDefaultTokenScope.title",
		DescKey:        "runtimeSettings:items.mcpDefaultTokenScope.description",
		Title:          "Default token scope",
		Description:    "Scope given to a new API token or MCP client token created without explicit abilities: read_only (view only) or read_write (also change config). A token can still be created with explicit abilities. Existing tokens keep their scope. The environment variable only seeds the default.",
	})
}

// defaultTokenAbilities resolves the abilities for a token request that named
// none. A failed or invalid policy read fails closed to read-only.
func (rt *Router) defaultTokenAbilities(ctx context.Context, allowChanges bool) []string {
	policy := tokenScopeReadOnly
	if rt.runtimeSettings != nil {
		if v, err := rt.runtimeSettings.StringChecked(ctx, KeyMCPDefaultTokenScope); err == nil {
			policy = v
		}
	}
	if allowChanges || policy == tokenScopeReadWrite {
		return []string{AbilityRead, AbilityWrite}
	}
	return []string{AbilityRead}
}

// tokenAccess labels a token read_only only when none of its abilities can
// change anything, root and token-only grants included.
func tokenAccess(abilities []string) string {
	for _, a := range abilities {
		if !slices.Contains([]string{AbilityRead, AbilityReadSensitive}, a) {
			return tokenAccessCanChange
		}
	}
	return tokenAccessReadOnly
}

// withPolicyAccess upgrades a read_only label to can_change when IAM policies are
// attached, since a policy can allow changes the base abilities do not. A failed
// lookup also upgrades, so the label never understates access.
func (rt *Router) withPolicyAccess(ctx context.Context, res tokenResource) tokenResource {
	if res.Access != tokenAccessReadOnly || rt.policies == nil {
		return res
	}
	attached, err := rt.policies.ListPoliciesForPrincipal(ctx, store.PrincipalTypeToken, res.ID)
	if err != nil || len(attached) > 0 {
		res.Access = tokenAccessCanChange
	}
	return res
}
