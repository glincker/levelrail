package compose

import (
	"fmt"
	"slices"
	"sort"

	"gopkg.in/yaml.v3"
)

// Depends_on conditions from the long form. service_completed_successfully
// has no one-shot job model here, so it is treated as service_started.
const (
	DependsOnConditionStarted = "service_started"
	DependsOnConditionHealthy = "service_healthy"
	dependsOnConditionDone    = "service_completed_successfully"
)

type rawConditionFile struct {
	Services map[string]struct {
		DependsOn yaml.Node `yaml:"depends_on"`
	} `yaml:"services"`
}

// parseDependsOnConditions returns, per service, the dependencies declared
// with condition: service_healthy. The list form carries no conditions.
func parseDependsOnConditions(data []byte) (map[string][]string, error) {
	var raw rawConditionFile
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("compose: parse depends_on: %w", err)
	}
	out := make(map[string][]string)
	for svc, body := range raw.Services {
		node := body.DependsOn
		if node.Kind != yaml.MappingNode {
			continue
		}
		for i := 0; i+1 < len(node.Content); i += 2 {
			dep := node.Content[i].Value
			var cfg struct {
				Condition string `yaml:"condition"`
			}
			if err := node.Content[i+1].Decode(&cfg); err != nil {
				return nil, fmt.Errorf("compose: service %q: depends_on.%s: %w", svc, dep, err)
			}
			switch cfg.Condition {
			case "", DependsOnConditionStarted, dependsOnConditionDone:
			case DependsOnConditionHealthy:
				out[svc] = append(out[svc], dep)
			default:
				return nil, fmt.Errorf("compose: service %q: depends_on.%s: unknown condition %q", svc, dep, cfg.Condition)
			}
		}
		sort.Strings(out[svc])
	}
	return out, nil
}

// ApplyProfiles drops every service whose profiles: list shares nothing
// with active, the same default Compose applies (a service with no
// profiles always runs). It returns the dropped service keys, sorted, and
// fails when a kept service depends on a dropped one.
func (f *File) ApplyProfiles(active []string) ([]string, error) {
	var dropped []string
	for _, name := range sortedServiceNames(f) {
		svc := f.Services[name]
		if len(svc.Profiles) == 0 || slices.ContainsFunc(svc.Profiles, func(p string) bool { return slices.Contains(active, p) }) {
			continue
		}
		dropped = append(dropped, name)
	}
	for _, name := range dropped {
		delete(f.Services, name)
		delete(f.Domains, name)
	}
	for _, name := range sortedServiceNames(f) {
		for _, dep := range f.Services[name].DependsOn {
			if slices.Contains(dropped, dep) {
				return dropped, fmt.Errorf("compose: service %q depends on %q, which only runs under profile(s) not enabled here", name, dep)
			}
		}
	}
	return dropped, nil
}

// Profiles lists every profile name any service declares, sorted.
func (f *File) Profiles() []string {
	seen := map[string]bool{}
	for _, svc := range f.Services {
		for _, p := range svc.Profiles {
			seen[p] = true
		}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
