package api

import (
	"errors"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/bindaddr"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/placement"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/standalone"
	"github.com/GLINCKER/levelrail/internal/store"
)

// validKnownStrategies mirrors internal/spec's own
// StrategyRolling/StrategyRecreate/StrategyBlueGreen exactly, all three
// reconciler-backed (internal/reconcile/application's controller runs
// all three).
var validKnownStrategies = map[string]bool{
	spec.StrategyRolling:   true,
	spec.StrategyRecreate:  true,
	spec.StrategyBlueGreen: true,
}

// validateAppPort allows no port for a service that only publishes host
// ports (a DNS or VPN style UDP server), or one stored without a port (a
// compose worker); a routed domain still needs one.
func validateAppPort(a appResource, storedPortless bool) error {
	if a.Port == 0 && (len(a.PublishedPorts) > 0 || storedPortless) {
		if len(a.Domains) > 0 {
			return errors.New("port is required when domains are set, the proxy routes HTTP to it")
		}
		return nil
	}
	if a.Port <= 0 || a.Port > 65535 {
		return errors.New("port must be between 1 and 65535, or set published_ports for a service with no HTTP port")
	}
	return nil
}

func validateAppResource(a appResource) error {
	return validateAppResourceWith(a, false)
}

func validateAppResourceWith(a appResource, storedPortless bool) error {
	if a.Name == "" {
		return errors.New("name is required")
	}
	if a.Name == standalone.Scope {
		return errors.New("name " + standalone.Scope + " is reserved for standalone domains")
	}
	if reservedSecretNamespace(a.Name) {
		return errors.New("name " + a.Name + " is reserved for control plane settings")
	}
	if a.Image == "" {
		return errors.New("image is required")
	}
	if err := validateAppPort(a, storedPortless); err != nil {
		return err
	}
	if err := validateServiceResources(a.Resources); err != nil {
		return err
	}
	if err := placement.ValidateRules(a.Tolerations, a.Affinity); err != nil {
		return err
	}
	if a.HostPort != nil && (*a.HostPort < 1 || *a.HostPort > 65535) {
		return errors.New("host_port must be between 1 and 65535")
	}
	// Empty is valid (falls through to store.DefaultBindAddress), only a
	// value that fails internal/bindaddr.Validate is an error.
	if a.BindAddress != "" {
		if err := bindaddr.Validate(a.BindAddress); err != nil {
			return fmt.Errorf("bind_address: %w", err)
		}
	}
	// Empty defers to the app.yaml next.cache value.
	if a.NextCache != "" && a.NextCache != spec.NextCacheNone && a.NextCache != spec.NextCacheRedis {
		return fmt.Errorf("next_cache must be %q or %q", spec.NextCacheNone, spec.NextCacheRedis)
	}
	// Empty is valid (falls through to store.DefaultDeployStrategy), but
	// a non-empty value that isn't one of the three known spec constants
	// is almost certainly a typo, and worth rejecting loudly here rather
	// than saving it and letting the reconciler discover it later.
	if a.Strategy != "" && !validKnownStrategies[a.Strategy] {
		return fmt.Errorf("strategy must be one of %q, %q, %q", spec.StrategyRolling, spec.StrategyRecreate, spec.StrategyBlueGreen)
	}
	// Zero is valid (falls through to store.DefaultReplicas), only a
	// negative count is nonsensical.
	if a.Replicas < 0 {
		return errors.New("replicas must not be negative")
	}
	if err := spec.ValidateLabels(a.Labels); err != nil {
		return err
	}
	if err := validateHealth(a.Health); err != nil {
		return err
	}
	for key, value := range a.Secrets {
		if key == "" {
			return errors.New("secrets: key must not be empty")
		}
		if value == "" {
			return fmt.Errorf("secrets[%q]: value is required", key)
		}
	}
	if len(a.VaultEnv) > 0 {
		secretBacked := unionSecretEnvNames(a.SecretEnv, a.Secrets)
		secretBackedSet := make(map[string]bool, len(secretBacked))
		for _, k := range secretBacked {
			secretBackedSet[k] = true
		}
		for key, ref := range a.VaultEnv {
			if key == "" {
				return errors.New("vault_env: key must not be empty")
			}
			if ref.Path == "" || ref.Key == "" {
				return fmt.Errorf("vault_env[%q]: path and key are both required", key)
			}
			if secretBackedSet[key] {
				return fmt.Errorf("env var %q cannot be both vault-backed and secret-backed", key)
			}
		}
	}
	// Mirrors the app.yaml schema's own minProperties: 1 on hooks: (this
	// endpoint's store.ServiceHooks bypasses that schema entirely, so the
	// same "no pointless empty block" rule needs its own check here): a
	// non-nil Hooks with both commands empty can only be a caller mistake,
	// never a meaningful desired state.
	if a.Hooks != nil && a.Hooks.PreDeploy == "" && a.Hooks.PostDeploy == "" {
		return errors.New("hooks must set at least one of pre_deploy or post_deploy")
	}
	for _, domain := range a.Domains {
		if err := ingress.ValidateWildcardDomain(domain); err != nil {
			return err
		}
	}
	return nil
}

// validateAppUpdate lets a service deployed without a port (compose
// worker, UDP-only) stay so; req already carries the merged stored fields.
func validateAppUpdate(req appResource, existing store.DesiredService) error {
	return validateAppResourceWith(req, req.Port == 0 && existing.Port == 0)
}
