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
	// SAMLSCIM is the interim hand-built SAML sign-in and SCIM provisioning, off until it moves onto the library's native implementation.
	SAMLSCIM Feature = "saml-scim"
)

// EnvVar names the env var holding the comma separated enabled features.
const EnvVar = "APP_EXPERIMENTAL"

// All returns every gated feature, sorted.
func All() []Feature {
	fs := []Feature{AIChat, AIModels, LoadBalancer, IaC, CloudflareTunnel, AccessRoles, GlobalEnvironments, AIControl, ResourceAdvisor, CLIConsole, MobileAPI, SAMLSCIM}
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
}

// Reset forgets the cached environment, for tests.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	enabled, loaded = nil, false
	runtime = map[Feature]bool{}
}

// DisabledMessage is the operator-facing text for a gated surface.
func DisabledMessage(f Feature) string {
	if Toggleable(f) {
		return fmt.Sprintf("the %s feature is in beta and off by default; turn it on in Settings, Beta features, or set %s=%s", f, EnvVar, f)
	}
	return fmt.Sprintf("the %s feature is experimental and off by default; set %s=%s to enable it", f, EnvVar, f)
}
