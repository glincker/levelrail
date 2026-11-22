package diagnose

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Cause codes for first-deploy failures that need app config, not a rebuild of the image.
const (
	CauseMissingStartCommand = "MISSING_START_COMMAND"
	CauseNodeVersion         = "NODE_VERSION_MISMATCH"
	CauseBuildPublicEnv      = "BUILD_PUBLIC_ENV_MISSING"
	CauseBuildOOM            = "BUILD_OUT_OF_MEMORY"
)

// CommandField is the Change field a start command fix edits.
const CommandField = "command"

// NodeVersionEnv is the variable Railpack reads to pin the Node major, named in the hint only.
const NodeVersionEnv = "RAILPACK_NODE_VERSION"

var startCommandRes = []*regexp.Regexp{
	regexp.MustCompile(`(?i)missing script:?\s*"?start"?`),
	regexp.MustCompile(`(?i)no start command (?:was )?(?:found|detected|could be (?:found|determined))`),
	regexp.MustCompile(`(?i)could(?:n't| not) (?:find|determine) (?:a |the )?(?:start command|how to start)`),
	regexp.MustCompile(`(?i)error: no entrypoint (?:found|detected)`),
}

func ruleMissingStartCommand(_ Input, lines []line) *Cause {
	l, _, ok := firstMatchAny(lines, startCommandRes)
	if !ok {
		return nil
	}
	return &Cause{
		Code:        CauseMissingStartCommand,
		Title:       "No start command was found",
		Explanation: "The project has no start script or entrypoint the platform can run, so the container has nothing to launch.",
		Confidence:  ConfidenceHigh,
		Evidence:    []Signal{evidence(l)},
		Fixes: []Fix{{
			Label:    "Set the start command",
			Kind:     FixInput,
			Changes:  []Change{{Field: CommandField, NeedsInput: true}},
			Hint:     "Use the command that starts your server, for example: npm run start:prod. Adding a \"start\" script to package.json works too.",
			Redeploy: true,
		}},
	}
}

var nodeEngineRes = []*regexp.Regexp{
	regexp.MustCompile(`(?i)the engine "node" is incompatible with this module\. expected version "([^"]+)"`),
	regexp.MustCompile(`(?i)notsup required: \{"node":"([^"]+)"`),
	regexp.MustCompile(`(?i)node\.js version ([<>=^~0-9. ]+?) is required`),
	regexp.MustCompile(`(?i)unsupported engine.*"node":\s*"([^"]+)"`),
	regexp.MustCompile(`(?i)requires node(?:\.js)? ([<>=^~0-9.x ]+)`),
}

var (
	numRe      = regexp.MustCompile(`\d+`)
	upperRe    = regexp.MustCompile(`<\s*(\d+)`)
	maxNodeMaj = 40
)

// nodeMajor turns an engines range into the Node major to pin: the lower
// bound, or one below an exclusive upper bound when one is given.
func nodeMajor(rng string) (int, bool) {
	if m := upperRe.FindStringSubmatch(rng); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil && n > 1 && n <= maxNodeMaj {
			return n - 1, true
		}
	}
	if m := numRe.FindString(rng); m != "" {
		if n, err := strconv.Atoi(m); err == nil && n > 0 && n <= maxNodeMaj {
			return n, true
		}
	}
	return 0, false
}

func ruleNodeVersion(_ Input, lines []line) *Cause {
	l, m, ok := firstMatchAny(lines, nodeEngineRes)
	if !ok {
		return nil
	}
	rng := strings.TrimSpace(m[1])
	major, ok := nodeMajor(rng)
	if !ok {
		return nil
	}
	c := &Cause{
		Code:        CauseNodeVersion,
		Title:       "The Node version does not match what the project requires",
		Explanation: fmt.Sprintf("The project requires Node %s but the build used a different version.", rng),
		Confidence:  ConfidenceHigh,
		Evidence:    []Signal{evidence(l)},
	}
	// Only public-prefix variables reach the build, so no app setting can pin Node.
	c.Fixes = []Fix{{
		Label: fmt.Sprintf("Pin Node %d in the project", major),
		Kind:  FixManual,
		Hint:  fmt.Sprintf("Set \"engines\": {\"node\": \"%d\"} in package.json (or add a .node-version file containing %d), commit, and redeploy. The Railpack variable is %s.", major, major, NodeVersionEnv),
	}}
	return c
}

var publicEnvNameRe = regexp.MustCompile(`\b((?:NEXT_PUBLIC|NUXT_PUBLIC|VITE|REACT_APP|EXPO_PUBLIC|PUBLIC)_[A-Z0-9_]+)\b`)

var publicEnvMissingRe = regexp.MustCompile(`(?i)not set|is required|missing|undefined|not defined|must be set|invalid environment|required|expected string`)

// isPublicEnvName reports whether name is a framework variable inlined into
// client code at build time.
func isPublicEnvName(name string) bool { return publicEnvNameRe.MatchString(name) }

func rulePublicBuildEnv(in Input, lines []line) *Cause {
	if !in.Facts.BuildFailed {
		return nil
	}
	have := make(map[string]bool, len(in.Facts.EnvKeys))
	for _, k := range in.Facts.EnvKeys {
		have[strings.ToUpper(k)] = true
	}
	var names []string
	seen := map[string]bool{}
	var ev []Signal
	for _, l := range lines {
		if !publicEnvMissingRe.MatchString(l.text) {
			continue
		}
		for _, m := range publicEnvNameRe.FindAllStringSubmatch(l.text, -1) {
			n := m[1]
			if seen[n] || have[n] {
				continue
			}
			seen[n] = true
			names = append(names, n)
			if len(ev) < 3 {
				ev = append(ev, evidence(l))
			}
		}
	}
	if len(names) == 0 {
		return nil
	}
	changes := make([]Change, 0, len(names))
	for _, n := range names {
		changes = append(changes, Change{Field: "env." + n, NeedsInput: true})
	}
	return &Cause{
		Code:        CauseBuildPublicEnv,
		Title:       "A build-time public variable is missing",
		Explanation: "The build needs " + strings.Join(names, ", ") + ", which the framework bakes into the client code while building. It cannot be supplied after the build.",
		Confidence:  ConfidenceHigh,
		Evidence:    ev,
		Fixes: []Fix{{
			Label:    "Set " + strings.Join(names, ", ") + " and rebuild",
			Kind:     FixInput,
			Changes:  changes,
			Hint:     "These values are public once built. Do not put secrets in them.",
			Redeploy: true,
		}},
	}
}

var buildOOMNeedles = []string{"reached heap limit", "heap out of memory", "signal: killed", "exit code: 137", "non-zero code: 137", "cannot allocate memory", "oomkilled", "out of memory"}

func ruleBuildOOM(in Input, lines []line) *Cause {
	if !in.Facts.BuildFailed {
		return nil
	}
	l, ok := firstContains(lines, buildOOMNeedles...)
	if !ok {
		return nil
	}
	return &Cause{
		Code:        CauseBuildOOM,
		Title:       "The build ran out of memory",
		Explanation: "The build process was killed because it ran out of memory, so no image was produced.",
		Confidence:  ConfidenceHigh,
		Evidence:    []Signal{evidence(l)},
		Fixes: []Fix{{
			Label:    "Give the build more memory or cap the Node heap",
			Kind:     FixManual,
			Hint:     "Build on a node with more free memory, or lower the build's peak use, for example with NODE_OPTIONS=--max-old-space-size=2048 as a build variable. Then retry the deploy.",
			Redeploy: true,
		}},
	}
}
