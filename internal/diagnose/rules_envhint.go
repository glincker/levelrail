package diagnose

import (
	"regexp"
	"strings"
)

var (
	envHintLeadRe   = regexp.MustCompile(`(?i)docker run|adding the following|set the following environment`)
	envHintFlagRe   = regexp.MustCompile(`(?:^|\s)-e\s+([A-Z][A-Z0-9_]{2,})=(.*)$`)
	envHintAcceptRe = regexp.MustCompile(`(?i)please accept\b.*\b(?:eula|licen[cs]e|terms)\b`)
	envHintURLRe    = regexp.MustCompile(`https?://[^\s'"<>]+`)
	envHintValueRe  = regexp.MustCompile(`^[A-Za-z0-9._:/-]{1,64}$`)
)

const envHintWindow = 3

// ruleImageEnvHint handles images that print the exact "-e NAME=VALUE" they
// need, such as a licence acceptance. The fix is a patch the user still has
// to click, never auto applied.
func ruleImageEnvHint(in Input, lines []line) *Cause {
	have := make(map[string]bool, len(in.Facts.EnvKeys))
	for _, k := range in.Facts.EnvKeys {
		have[strings.ToUpper(k)] = true
	}
	seen := map[string]bool{}
	var changes []Change
	var ev []Signal
	var labels []string
	needsInput := false
	for i, l := range lines {
		m := envHintFlagRe.FindStringSubmatch(l.text)
		if m == nil || !envHintLeadNear(lines, i) {
			continue
		}
		name := m[1]
		if seen[name] || have[name] {
			continue
		}
		seen[name] = true
		val := strings.Trim(strings.TrimSpace(m[2]), `"'`)
		ch := Change{Field: "env." + name}
		if envHintValueRe.MatchString(val) {
			ch.To = val
			labels = append(labels, name+"="+val)
		} else {
			ch.NeedsInput = true
			needsInput = true
			labels = append(labels, name)
		}
		changes = append(changes, ch)
		if len(ev) < 3 {
			ev = append(ev, evidence(l))
		}
	}
	if len(changes) == 0 {
		return nil
	}
	accept, url := envHintTerms(lines)
	label := "Set " + strings.Join(labels, ", ")
	hint := ""
	if accept {
		label = "Accept the terms and set " + strings.Join(labels, ", ")
		if url != "" {
			hint = "Read the terms at " + url + ". Applying this means you accept them."
		}
	}
	kind := FixPatch
	if needsInput {
		kind = FixInput
	}
	return &Cause{
		Code:        CauseMissingEnv,
		Title:       "This image needs an environment variable before it will start",
		Explanation: "The image printed the environment variable it needs and exited: " + strings.Join(labels, ", ") + ".",
		Confidence:  ConfidenceHigh,
		Evidence:    ev,
		Fixes:       []Fix{{Label: label, Kind: kind, Changes: changes, Hint: hint, Redeploy: true}},
	}
}

func envHintLeadNear(lines []line, i int) bool {
	for j := i - 1; j >= 0 && j >= i-envHintWindow; j-- {
		if envHintLeadRe.MatchString(lines[j].text) {
			return true
		}
	}
	return false
}

// envHintTerms reports whether a licence acceptance line exists and the
// first URL within a few lines of it.
func envHintTerms(lines []line) (bool, string) {
	found := false
	for i, l := range lines {
		if !envHintAcceptRe.MatchString(l.text) {
			continue
		}
		found = true
		for j := i; j < len(lines) && j <= i+envHintWindow; j++ {
			if u := envHintURLRe.FindString(lines[j].text); u != "" {
				return true, u
			}
		}
	}
	return found, ""
}
