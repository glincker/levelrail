package deploy

import (
	"fmt"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// toDesiredService translates a build's result into the runtime desired
// state the application controller reconciles against. validateEnv
// already rejected any { from: ... } reference, and any { secret: true }
// var with no checker configured or (if required) no value set, before
// a build was even attempted.
func toDesiredService(name, image string, svc spec.Service) (store.DesiredService, error) {
	d := store.DesiredService{
		Name:      name,
		Image:     image,
		Port:      svc.Port,
		Domains:   svc.Domains,
		Env:       literalEnv(svc.Env),
		SecretEnv: secretEnvNames(svc.Env),
		// Effective*, not the raw field: svc.Strategy/svc.Replicas can be
		// "" / 0 (unset in app.yaml), and store.DesiredService's own doc
		// comment on these two fields requires them to always be the
		// already-resolved value, matching Resources/Health's own
		// resolve-before-storing shape below.
		Strategy: svc.EffectiveStrategy(),
		Replicas: svc.EffectiveReplicas(),
		Labels:   svc.Labels,
	}

	if svc.Resources != nil {
		resources, err := toServiceResources(*svc.Resources)
		if err != nil {
			return store.DesiredService{}, fmt.Errorf("resources: %w", err)
		}
		d.Resources = &resources
	}

	if svc.Health != nil {
		health, err := toServiceHealth(*svc.Health)
		if err != nil {
			return store.DesiredService{}, fmt.Errorf("health: %w", err)
		}
		d.Health = &health
	}

	return d, nil
}

func literalEnv(env map[string]spec.EnvVar) map[string]string {
	if len(env) == 0 {
		return nil
	}
	out := make(map[string]string, len(env))
	for k, v := range env {
		if v.Secret {
			continue
		}
		out[k] = v.Value
	}
	return out
}

// secretEnvNames lists the env var names declared { secret: true },
// carrying no value: store.DesiredService.SecretEnv is names only, the
// application controller resolves each one's actual value from
// internal/secrets immediately before container creation.
func secretEnvNames(env map[string]spec.EnvVar) []string {
	var names []string
	for k, v := range env {
		if v.Secret {
			names = append(names, k)
		}
	}
	return names
}

func toServiceResources(r spec.Resources) (store.ServiceResources, error) {
	var out store.ServiceResources
	if r.Memory != "" {
		bytes, err := parseMemoryBytes(r.Memory)
		if err != nil {
			return out, err
		}
		out.MemoryBytes = bytes
	}
	if r.CPU != 0 {
		out.NanoCPUs = int64(r.CPU * 1e9)
	}
	return out, nil
}

// parseMemoryBytes parses app.yaml's "512Mi"/"1Gi" shape (internal/spec's
// JSON Schema already enforces this pattern for anything that went
// through spec.Parse, but this package's own Request takes a bare
// spec.Service a caller could construct by hand, so this stays a real
// check, not a redundant one).
func parseMemoryBytes(s string) (int64, error) {
	digits := 0
	for digits < len(s) && s[digits] >= '0' && s[digits] <= '9' {
		digits++
	}
	if digits == 0 || digits == len(s) {
		return 0, fmt.Errorf("invalid memory value %q, want a number followed by Mi or Gi", s)
	}

	num, err := strconv.ParseInt(s[:digits], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid memory value %q: %w", s, err)
	}

	switch unit := s[digits:]; unit {
	case "Mi":
		return num * 1024 * 1024, nil
	case "Gi":
		return num * 1024 * 1024 * 1024, nil
	default:
		return 0, fmt.Errorf("invalid memory unit %q in %q, want Mi or Gi", unit, s)
	}
}

func toServiceHealth(h spec.Health) (store.ServiceHealth, error) {
	var out store.ServiceHealth
	if h.Readiness != nil {
		p, err := toServiceProbe(*h.Readiness)
		if err != nil {
			return out, fmt.Errorf("readiness: %w", err)
		}
		out.Readiness = &p
	}
	if h.Liveness != nil {
		p, err := toServiceProbe(*h.Liveness)
		if err != nil {
			return out, fmt.Errorf("liveness: %w", err)
		}
		out.Liveness = &p
	}
	return out, nil
}

func toServiceProbe(p spec.Probe) (store.ServiceProbe, error) {
	interval, err := parseDurationOrZero(p.Interval)
	if err != nil {
		return store.ServiceProbe{}, fmt.Errorf("interval: %w", err)
	}
	timeout, err := parseDurationOrZero(p.Timeout)
	if err != nil {
		return store.ServiceProbe{}, fmt.Errorf("timeout: %w", err)
	}
	return store.ServiceProbe{
		Path:     p.Path,
		Interval: interval,
		Timeout:  timeout,
		Failures: p.Failures,
	}, nil
}

// parseDurationOrZero treats "" as "not specified" rather than an error;
// internal/probe already defaults a zero Interval/Timeout sensibly.
func parseDurationOrZero(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: %w", s, err)
	}
	return d, nil
}
