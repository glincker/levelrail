package cloudconn

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/GLINCKER/levelrail/internal/secretsource"
)

// Providers and auth methods accepted on a connection.
const (
	ProviderOracle       = "oracle"
	ProviderAWS          = "aws"
	ProviderAzure        = "azure"
	ProviderGCP          = "gcp"
	ProviderHetzner      = "hetzner"
	ProviderDigitalOcean = "digitalocean"

	AuthOIDCFederation = "oidc_federation"
	AuthRoleAssumption = "role_assumption"
	AuthSecretRef      = "secret_ref"
	AuthStatic         = "static"
)

// Health classes. A check reports one of these and nothing from the provider's own output.
const (
	ClassOK                   = "ok"
	ClassUnsupported          = "unsupported"
	ClassConfigInvalid        = "config_invalid"
	ClassCredentialUnresolved = "credential_unresolved" //nolint:gosec // class name, not a credential
	ClassAuthFailed           = "auth_failed"
	ClassUnreachable          = "unreachable"
	ClassUnknown              = "unknown"
)

// ErrUnsupported marks a provider operation this build does not implement.
var ErrUnsupported = errors.New("cloudconn: not implemented for this provider")

// Target is what a provider needs to probe one connection. Credential is zero
// for oidc_federation and masks itself when formatted.
type Target struct {
	Connection Connection
	Credential secretsource.Value
}

// Provider is the read-only side of a cloud. Implementations must not create,
// change or delete anything in the account.
type Provider interface {
	Name() string
	// ValidateConfig checks the non-secret settings for the given auth method.
	ValidateConfig(cfg Config, authMethod string) error
	// Check makes one read-only call and returns a Class constant; detail stays inside the provider.
	Check(ctx context.Context, t Target) (string, error)
	// ListRegions returns region identifiers the credential can see, read-only.
	ListRegions(ctx context.Context, t Target) ([]string, error)
}

// Registry maps a provider name to its implementation.
type Registry map[string]Provider

// Names returns the registered provider names, sorted.
func (r Registry) Names() []string {
	out := make([]string, 0, len(r))
	for n := range r {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// DefaultRegistry registers every provider as not implemented. The real OCI, AWS and Azure
// clients need SDK dependencies this repo does not carry yet; adding them is an owner decision.
func DefaultRegistry() Registry {
	r := Registry{}
	for _, n := range []string{ProviderOracle, ProviderAWS, ProviderAzure, ProviderGCP, ProviderHetzner, ProviderDigitalOcean} {
		r[n] = notImplemented{name: n}
	}
	return r
}

// notImplemented validates config for real but refuses every live call.
type notImplemented struct{ name string }

func (p notImplemented) Name() string { return p.name }

func (p notImplemented) ValidateConfig(cfg Config, authMethod string) error {
	return validateProviderConfig(p.name, cfg, authMethod)
}

func (p notImplemented) Check(context.Context, Target) (string, error) {
	return ClassUnsupported, fmt.Errorf("%w: %s health check", ErrUnsupported, p.name)
}

func (p notImplemented) ListRegions(context.Context, Target) ([]string, error) {
	return nil, fmt.Errorf("%w: %s regions", ErrUnsupported, p.name)
}
