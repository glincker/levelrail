package githubapp

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ManifestConfig is the operator-tunable subset of BuildManifest's
// output: which permissions and webhook events a newly-registered App
// requests. Everything else in Manifest (name, urls) is derived from
// baseURL/brand and has no business being config-driven, since it must
// always match this control plane's own actual routes; permissions and
// events are the one part an operator might genuinely need to extend
// (e.g. a future feature needing pull_requests:write) without waiting
// on a code change and recompile.
type ManifestConfig struct {
	DefaultPermissions map[string]string `yaml:"default_permissions"`
	DefaultEvents      []string          `yaml:"default_events"`
}

// DefaultManifestConfig is what BuildManifest requests: contents:read +
// metadata:read (BuildManifest's own doc comment explains why) plus
// repository_hooks:write, so a freshly registered App can call
// Client.CreateRepoWebhook to auto-register a repo's push webhook. No
// webhook events. Returned by LoadManifestConfig whenever no config
// file is present.
//
// GitHub does not retroactively grant a new permission to an existing
// installation, so an App installed before this permission was added
// here keeps failing CreateRepoWebhook with ErrPermissionDenied until
// reinstalled; see internal/api's handleUseGitHubRepoAsSource for how
// that's handled rather than treated as fatal.
func DefaultManifestConfig() ManifestConfig {
	return ManifestConfig{
		DefaultPermissions: map[string]string{
			"contents":         "read",
			"metadata":         "read",
			"repository_hooks": "write",
		},
		DefaultEvents: []string{},
	}
}

// LoadManifestConfig reads path as YAML and returns the resulting
// ManifestConfig. Unlike internal/brand.Load's brand.yaml, a missing
// file here is not an error: it returns DefaultManifestConfig()
// unchanged, since customizing App permissions/events is an optional,
// advanced knob most operators never need to touch.
func LoadManifestConfig(path string) (ManifestConfig, error) {
	// path is operator-supplied at binary startup (a fixed config
	// location, not attacker-controlled request input), the same
	// exemption internal/brand.Load's own file read carries.
	data, err := os.ReadFile(path) //nolint:gosec // caller-controlled startup config path, not user input
	if errors.Is(err, os.ErrNotExist) {
		return DefaultManifestConfig(), nil
	}
	if err != nil {
		return ManifestConfig{}, fmt.Errorf("githubapp: read manifest config %s: %w", path, err)
	}

	cfg := DefaultManifestConfig()
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return ManifestConfig{}, fmt.Errorf("githubapp: parse manifest config %s: %w", path, err)
	}
	if cfg.DefaultPermissions == nil {
		cfg.DefaultPermissions = map[string]string{}
	}
	if cfg.DefaultEvents == nil {
		cfg.DefaultEvents = []string{}
	}
	return cfg, nil
}
