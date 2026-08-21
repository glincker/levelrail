package compose

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/GLINCKER/levelrail/internal/spec"
)

// ExpandBuildService reads and parses the compose file svc.Build.Path
// points at (resolved relative to sourceDir, the same git checkout root
// every other build.type already resolves its own paths against), and
// returns one spec.Service per compose service it declares: a
// build:-bearing compose service becomes an ordinary build.type:
// dockerfile entry, an image:-only one becomes build.type: image. The
// caller (Pipeline.DeploySpec) is expected to splice these entries into
// its own Services map in svc's place and fan out exactly as it already
// does for any other declared service, no reconciler or build-pipeline
// change needed: every compose-declared service converges through the
// exact same one-container-per-service path every other service does.
//
// svc.Build.Type must be spec.BuildCompose; anything else is a caller
// bug, not a user-facing error.
func ExpandBuildService(svc spec.Service, sourceDir string) (map[string]spec.Service, error) {
	if svc.Build.Type != spec.BuildCompose {
		return nil, fmt.Errorf("compose: expand: build.type is %q, not %q", svc.Build.Type, spec.BuildCompose)
	}

	composePath := filepath.Join(sourceDir, svc.Build.Path)
	data, err := os.ReadFile(composePath) //nolint:gosec // svc.Build.Path is app.yaml-declared, resolved against a real git checkout, the same trust boundary every other build.type's own Path already has
	if err != nil {
		return nil, fmt.Errorf("compose: read %q: %w", svc.Build.Path, err)
	}

	f, err := Parse(data)
	if err != nil {
		return nil, err
	}
	if err := f.ValidateForBuild(); err != nil {
		return nil, err
	}

	// composeDir is the directory a compose service's own build.context
	// is relative to, per Compose's own semantics: the compose file's
	// own directory, not sourceDir (the outer git checkout root), so a
	// compose file that isn't at the repo root (e.g.
	// services/api/docker-compose.yml) still resolves its services'
	// build contexts correctly.
	composeDir := filepath.Dir(svc.Build.Path)

	out := make(map[string]spec.Service, len(f.Services))
	for _, key := range sortedServiceNames(f) {
		csvc := f.Services[key]
		expanded, err := toSpecService(key, csvc, f.Domains[key], composeDir)
		if err != nil {
			return nil, err
		}
		out[key] = expanded
	}
	return out, nil
}

// toSpecService converts one compose.Service, already validated by
// ValidateForBuild, into the spec.Service ExpandBuildService returns
// for it.
func toSpecService(key string, csvc Service, domain string, composeDir string) (spec.Service, error) {
	for envKey, v := range csvc.Environment {
		if vars := FindMagicVars(v); len(vars) > 0 {
			// Magic vars (SERVICE_PASSWORD_*, SERVICE_FQDN_*, ...) need
			// generation and secret persistence (ResolveMagicVars),
			// meaningful for the direct-import path's own deploy flow
			// (internal/api's compose-import handler already wires that
			// up). This build-from-source path has no equivalent wiring
			// yet: passing the token through unresolved would silently
			// start a container with a literal "${SERVICE_...}" string
			// in its env, a worse failure than rejecting it loudly here.
			return spec.Service{}, fmt.Errorf("compose: service %q: env %q: magic var %q is not supported for build.type: compose, use a literal value or a real secret instead", key, envKey, vars[0].Token)
		}
	}

	var build spec.Build
	if csvc.Build != nil {
		build = spec.Build{
			Type:          spec.BuildDockerfile,
			BaseDirectory: filepath.Join(composeDir, buildContext(csvc.Build.Context)),
			Path:          csvc.Build.Dockerfile,
		}
	} else {
		build = spec.Build{Type: spec.BuildImage, Image: csvc.Image}
	}

	s := spec.Service{
		Build:  build,
		Labels: csvc.Labels,
	}
	if domain != "" {
		s.Domains = []string{domain}
	}
	for _, p := range csvc.Ports {
		s.Port = p.ContainerPort
		break
	}
	if len(csvc.Environment) > 0 {
		s.Env = make(map[string]spec.EnvVar, len(csvc.Environment))
		for k, v := range csvc.Environment {
			s.Env[k] = spec.EnvVar{Value: v}
		}
	}
	for _, v := range csvc.Volumes {
		s.Volumes = append(s.Volumes, spec.Volume{Name: v.Name, Path: v.ContainerPath})
	}
	return s, nil
}

// buildContext defaults an unset build.context (Compose's own rule: a
// bare "build:" with only a dockerfile: field, or a scalar "build: ."
// already sets Context explicitly) to ".", the compose file's own
// directory, matching real Docker Compose's default.
func buildContext(context string) string {
	if context == "" {
		return "."
	}
	return context
}
