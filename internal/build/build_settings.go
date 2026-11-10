package build

import (
	"fmt"
	"regexp"
	"sort"
)

// Settings are the persistent per-app build options a build request carries.
// Values reach BuildKit as frontend attributes, never through a shell.
type Settings struct {
	Target   string
	NoCache  bool
	Provider string
}

// targetStagePattern matches the Dockerfile stage names BuildKit accepts after AS.
var targetStagePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// RailpackProviders lists the providers a settings pin may name, sorted.
func RailpackProviders() []string {
	out := make([]string, 0, len(supportedRailpackProviders))
	for p := range supportedRailpackProviders {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// ValidateSettings checks s against the allowlists; maxLen bounds each value.
func ValidateSettings(s Settings, maxLen int) error {
	if s.Target != "" {
		if len(s.Target) > maxLen {
			return fmt.Errorf("target is longer than %d bytes", maxLen)
		}
		if !targetStagePattern.MatchString(s.Target) {
			return fmt.Errorf("target %q must start with a letter or digit and use only letters, digits, '.', '_' or '-'", s.Target)
		}
	}
	if s.Provider != "" && !supportedRailpackProviders[s.Provider] {
		return fmt.Errorf("provider %q is not supported, use one of %v", s.Provider, RailpackProviders())
	}
	return nil
}

// Apply copies the Dockerfile options onto a build request.
func (s Settings) Apply(r *Request) {
	r.Target = s.Target
	r.NoCache = r.NoCache || s.NoCache
}

// checkPinnedProvider fails a build whose detected provider differs from the pin.
func checkPinnedProvider(pinned, detected string) error {
	if pinned == "" || pinned == detected {
		return nil
	}
	return fmt.Errorf("build: railpack: detected provider %q but the app's build settings require %q", detected, pinned)
}
