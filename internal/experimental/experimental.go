// Package experimental is the single platform gate for features hidden at launch.
package experimental

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
)

// Feature names a gated capability.
type Feature string

const (
	// AIChat is the in-app AI assistant chat.
	AIChat Feature = "ai-chat"
	// AIModels is GPU model hosting.
	AIModels Feature = "ai-models"
	// LoadBalancer is multi-replica load balancing.
	LoadBalancer Feature = "load-balancer"
	// IaC is platform as code (plan, apply, export).
	IaC Feature = "iac"
	// CloudflareTunnel is the Cloudflare tunnel integration.
	CloudflareTunnel Feature = "cloudflare-tunnel"
	// AccessRoles is stored roles, the guest role and per-environment grants.
	AccessRoles Feature = "access-roles"
	// GlobalEnvironments is instance-wide typed environments and the environment switcher.
	GlobalEnvironments Feature = "global-environments"
	// AIControl is the Settings switch that governs agent and AI access.
	AIControl Feature = "ai-control"
	// ResourceAdvisor is the beta resource advisor: limit, tuning and idle suggestions.
	ResourceAdvisor Feature = "resource-advisor"
	// CLIConsole is the dashboard's web console for the platform CLI (never a host shell).
	CLIConsole Feature = "cli-console"
	// MobileAPI is phone pairing, device tokens and the mobile overview endpoint.
	MobileAPI Feature = "mobile-api"
	// SAMLSCIM gates SAML sign-in and SCIM provisioning on the sign-in library.
	SAMLSCIM Feature = "saml-scim"
	// Plugins is the out-of-process plugin runtime: signed packages, operator-approved abilities (ADR 034).
	Plugins Feature = "plugins"
	// GitPush is the built-in remote that deploys an app from a push.
	GitPush Feature = "git-push"
	// Canvas is the beta visual editor that composes a project as a reviewable plan.
	Canvas Feature = "canvas"
)

// EnvVar names the env var holding the comma separated enabled features.
const EnvVar = "APP_EXPERIMENTAL"

// All returns every gated feature, sorted.
func All() []Feature {
	fs := []Feature{AIChat, AIModels, LoadBalancer, IaC, CloudflareTunnel, AccessRoles, GlobalEnvironments, AIControl, ResourceAdvisor, CLIConsole, MobileAPI, SAMLSCIM, Plugins, GitPush, Canvas}
	sort.Slice(fs, func(i, j int) bool { return fs[i] < fs[j] })
	return fs
}

// Parse turns a comma separated list into a feature set, rejecting unknown keys.
func Parse(s string) (map[Feature]bool, error) {
	valid := map[Feature]bool{}
	for _, f := range All() {
		valid[f] = true
	}
	set := map[Feature]bool{}
	for _, part := range strings.Split(s, ",") {
		k := Feature(strings.ToLower(strings.TrimSpace(part)))
		if k == "" {
			continue
		}
		if !valid[k] {
			return nil, fmt.Errorf("unknown experimental feature %q in %s, want one of %v", k, EnvVar, All())
		}
		set[k] = true
	}
	return set, nil
}

var (
	mu      sync.Mutex
	enabled map[Feature]bool
	loaded  bool
	runtime = map[Feature]bool{}
	desired = map[Feature]bool{}
)

// load must be called with mu held. Unknown keys are ignored so a typo never
// takes the platform down; Validate reports them at startup.
func load() {
	if loaded {
		return
	}
	raw := os.Getenv(EnvVar)
	set, err := Parse(raw)
	if err != nil {
		set = map[Feature]bool{}
		for _, part := range strings.Split(raw, ",") {
			k := Feature(strings.ToLower(strings.TrimSpace(part)))
			if _, perr := Parse(string(k)); perr == nil && k != "" {
				set[k] = true
			}
		}
	}
	enabled, loaded = set, true
}

// Validate reports an unknown feature key in the environment.
func Validate() error {
	_, err := Parse(os.Getenv(EnvVar))
	return err
}

// Enabled reports whether f is switched on. The environment is read once.
func Enabled(f Feature) bool {
	mu.Lock()
	defer mu.Unlock()
	load()
	return enabled[f] || runtime[f]
}

// EnabledList returns the enabled features, sorted.
func EnabledList() []Feature {
	out := []Feature{}
	for _, f := range All() {
		if Enabled(f) {
			out = append(out, f)
		}
	}
	return out
}

// Set replaces the enabled set, for tests.
func Set(fs ...Feature) {
	mu.Lock()
	defer mu.Unlock()
	enabled = map[Feature]bool{}
	for _, f := range fs {
		enabled[f] = true
	}
	loaded = true
	runtime = map[Feature]bool{}
	desired = map[Feature]bool{}
}

// Reset forgets the cached environment, for tests.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	enabled, loaded = nil, false
	runtime = map[Feature]bool{}
	desired = map[Feature]bool{}
}

// DisabledMessage is the operator-facing text for a gated surface.
func DisabledMessage(f Feature) string {
	return fmt.Sprintf("the %s feature is off by default; turn it on in Settings, Beta features (or set %s=%s on the server)", f, EnvVar, f)
}
