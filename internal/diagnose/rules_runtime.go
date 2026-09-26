package diagnose

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const (
	mib            = 1 << 20
	oomMinStepMiB  = 256
	oomRoundMiB    = 64
	defaultOOMMult = 2.0
)

func ruleOOMKilled(in Input, lines []line) *Cause {
	f := in.Facts
	var ev []Signal
	conf := ""
	switch {
	case f.OOMKilled:
		conf = ConfidenceHigh
		ev = append(ev, Signal{Source: "container_state", Excerpt: fmt.Sprintf("OOMKilled=true, exit code %d", f.ExitCode)})
	case f.HasExit && f.ExitCode == 137:
		conf = ConfidenceMedium
		ev = append(ev, Signal{Source: "container_state", Excerpt: "exit code 137 (SIGKILL)"})
	}
	if l, ok := firstContains(lines, OOMLogPatterns...); ok {
		if conf == "" {
			conf = ConfidenceMedium
		}
		ev = append(ev, evidence(l))
	}
	if conf == "" {
		return nil
	}
	c := &Cause{
		Code:        CauseOOMKilled,
		Title:       "Container killed for using too much memory",
		Explanation: "The kernel killed the container because it exceeded its memory limit.",
		Confidence:  conf,
		Evidence:    ev,
	}
	if f.MemoryBytes <= 0 {
		c.Fixes = []Fix{{Label: "Set a memory limit and size it above the app's peak usage", Kind: FixManual, Hint: "No memory limit is configured, so the host itself ran out of memory. Free memory on the node or move the app to a larger node."}}
		return c
	}
	next := increasedMemory(f.MemoryBytes, f.OOMFactor)
	c.Fixes = []Fix{{
		Label:    fmt.Sprintf("Raise the memory limit from %d MiB to %d MiB", f.MemoryBytes/mib, next/mib),
		Kind:     FixPatch,
		Changes:  []Change{{Field: "resources.memory_bytes", From: itoa64(f.MemoryBytes), To: itoa64(next)}},
		Redeploy: true,
	}}
	return c
}

// increasedMemory grows cur by factor (at least a fixed step) rounded up
// to a whole number of oomRoundMiB.
func increasedMemory(cur int64, factor float64) int64 {
	if factor <= 1 {
		factor = defaultOOMMult
	}
	next := int64(float64(cur) * factor)
	if floor := cur + oomMinStepMiB*mib; next < floor {
		next = floor
	}
	step := int64(oomRoundMiB * mib)
	return (next + step - 1) / step * step
}

var missingEnvRes = []*regexp.Regexp{
	regexp.MustCompile(`(?i)required environment variable[s]?:?\s*['"` + "`" + `]?([A-Za-z_][A-Za-z0-9_]*)`),
	regexp.MustCompile(`(?i)environment variable\s+['"` + "`" + `]?([A-Z_][A-Z0-9_]*)['"` + "`" + `]?\s+(?:is\s+)?(?:not set|required|missing|undefined|not defined)`),
	regexp.MustCompile(`\b([A-Z][A-Z0-9_]{2,})\b (?:is not set|is required|is missing|is undefined|must be set|is not defined|not found in environment)`),
	regexp.MustCompile(`KeyError: ['"]([A-Z][A-Z0-9_]+)['"]`),
	regexp.MustCompile(`(?i)missing (?:required )?env(?:ironment)?(?: var(?:iable)?)?:?\s+['"` + "`" + `]?([A-Z][A-Z0-9_]+)`),
	regexp.MustCompile(`required key ([A-Z][A-Z0-9_]+) missing value`),
	regexp.MustCompile(`\b([A-Z][A-Z0-9_]{2,}):\s*\[\s*['"](?:Required|Invalid input)['"]`),
	regexp.MustCompile(`(?i)(?:getenv|os\.environ|process\.env)[\[\.(]\s*['"]?([A-Z][A-Z0-9_]+)['"]?\]?\)?\s*(?:is )?(?:undefined|null|nil|not set)`),
}

var (
	pydanticFieldRe = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*)\s*$`)
	zodPathRe       = regexp.MustCompile(`"path":\s*\[\s*"([A-Za-z_][A-Za-z0-9_]*)"\s*\]`)
)

func ruleMissingEnv(in Input, lines []line) *Cause {
	have := make(map[string]bool, len(in.Facts.EnvKeys))
	for _, k := range in.Facts.EnvKeys {
		have[strings.ToUpper(k)] = true
	}
	var names []string
	seen := map[string]bool{}
	var ev []Signal
	add := func(l line, name string) {
		name = strings.ToUpper(name)
		if seen[name] || have[name] || len(name) < 3 {
			return
		}
		seen[name] = true
		names = append(names, name)
		if len(ev) < 3 {
			ev = append(ev, evidence(l))
		}
	}
	for i, l := range lines {
		for _, re := range missingEnvRes {
			if m := re.FindStringSubmatch(l.text); m != nil {
				add(l, m[1])
			}
		}
		if m := zodPathRe.FindStringSubmatch(l.text); m != nil && zodRequiredNear(lines, i) {
			add(l, m[1])
		}
		// pydantic: a bare field name line followed by "Field required".
		if m := pydanticFieldRe.FindStringSubmatch(l.text); m != nil && i+1 < len(lines) && strings.Contains(lines[i+1].text, "Field required") {
			add(l, m[1])
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
		Code:        CauseMissingEnv,
		Title:       "Required environment variable is not set",
		Explanation: "The app exited because it expected environment variables that are not configured: " + strings.Join(names, ", ") + ".",
		Confidence:  ConfidenceHigh,
		Evidence:    ev,
		Fixes: []Fix{{
			Label:    "Set " + strings.Join(names, ", "),
			Kind:     FixInput,
			Changes:  changes,
			Redeploy: true,
		}},
	}
}

func zodRequiredNear(lines []line, i int) bool {
	for j := i; j < len(lines) && j <= i+4; j++ {
		if strings.Contains(lines[j].text, "Required") {
			return true
		}
	}
	return false
}

var (
	pullNotFoundNeedles  = []string{"manifest unknown", "not found: manifest", "manifest for", "no such image", "repository does not exist", "name unknown"}
	pullRateLimitNeedles = []string{"toomanyrequests", "rate limit", "too many requests"}
	pullAuthNeedles      = []string{"unauthorized", "authentication required", "pull access denied", "requested access to the resource is denied", "denied:", "incorrect username or password"}
)

func ruleImagePull(_ Input, lines []line) *Cause {
	kinds := []struct {
		needles []string
		title   string
		expl    string
		hint    string
		conf    string
	}{
		{pullRateLimitNeedles, "Registry rate limit reached", "The registry refused the pull because too many anonymous or free-tier requests were made.", "Add registry credentials for this image so pulls are authenticated, or wait for the limit to reset and redeploy.", ConfidenceHigh},
		{pullNotFoundNeedles, "Image or tag not found", "The registry has no image with this name and tag.", "Check the image name and tag for typos and confirm the tag was pushed.", ConfidenceHigh},
		{pullAuthNeedles, "Registry denied access", "The registry rejected the pull as unauthorized. For most registries this also appears when the image name is wrong.", "Add a registry credential for this image's registry, and double check the image name.", ConfidenceHigh},
	}
	for _, k := range kinds {
		if l, ok := firstContains(lines, k.needles...); ok {
			return &Cause{
				Code:        CauseImagePullFailed,
				Title:       k.title,
				Explanation: k.expl,
				Confidence:  k.conf,
				Evidence:    []Signal{evidence(l)},
				Fixes:       []Fix{{Label: "Fix the image reference or registry credential", Kind: FixManual, Hint: k.hint}},
			}
		}
	}
	return nil
}

func ruleBuildOutOfDisk(in Input, lines []line) *Cause {
	l, ok := firstContains(lines, "no space left on device", "enospc", "disk quota exceeded")
	if !ok {
		return nil
	}
	expl := "The node ran out of disk space."
	if in.Facts.BuildFailed {
		expl = "The build failed because the build node ran out of disk space."
	}
	return &Cause{
		Code:        CauseBuildOutOfDisk,
		Title:       "No space left on device",
		Explanation: expl,
		Confidence:  ConfidenceHigh,
		Evidence:    []Signal{evidence(l)},
		Fixes:       []Fix{{Label: "Free disk space on the node", Kind: FixManual, Hint: "Prune unused images, build cache and stopped containers on the node, or add disk, then redeploy."}},
	}
}

var execFormatNeedles = []string{"exec format error", "no matching manifest for", "does not match the detected host platform"}

func ruleExecFormat(in Input, lines []line) *Cause {
	l, ok := firstContains(lines, execFormatNeedles...)
	if !ok {
		return nil
	}
	hint := "The image was built for a different CPU architecture than this node. Rebuild it for the node's architecture or publish a multi-arch image."
	if in.Facts.NodeArch != "" {
		hint = "The image was built for a different CPU architecture than this node (" + in.Facts.NodeArch + "). Rebuild with --platform linux/" + in.Facts.NodeArch + " or publish a multi-arch image."
	}
	return &Cause{
		Code:        CauseExecFormatError,
		Title:       "Image architecture does not match the node",
		Explanation: "The binary or image cannot run on this node's CPU architecture (for example an amd64 image on an arm64 node).",
		Confidence:  ConfidenceHigh,
		Evidence:    []Signal{evidence(l)},
		Fixes:       []Fix{{Label: "Use an image built for this node's architecture", Kind: FixManual, Hint: hint}},
	}
}

var commandNotFoundRes = []*regexp.Regexp{
	regexp.MustCompile(`exec: "([^"]+)": executable file not found`),
	regexp.MustCompile(`(?:sh|bash|dash): (?:\d+: )?([^\s:]+): (?:command )?not found`),
	regexp.MustCompile(`exec (/[^\s:]+): no such file or directory`),
	regexp.MustCompile(`OCI runtime create failed.*exec: "?([^"\s:]+)"?: (?:stat .*: )?no such file`),
	regexp.MustCompile(`([^\s:]+): command not found`),
}

func ruleCommandNotFound(in Input, lines []line) *Cause {
	l, m, ok := firstMatchAny(lines, commandNotFoundRes)
	exit127 := in.Facts.HasExit && in.Facts.ExitCode == 127
	if !ok && !exit127 {
		return nil
	}
	c := &Cause{
		Code:       CauseCommandNotFound,
		Title:      "Start command not found",
		Confidence: ConfidenceHigh,
	}
	cmd := ""
	if ok {
		cmd = m[1]
		c.Evidence = []Signal{evidence(l)}
	} else {
		c.Confidence = ConfidenceMedium
		c.Evidence = []Signal{{Source: "container_state", Excerpt: "exit code 127 (command not found)"}}
	}
	c.Explanation = "The container could not execute its start command"
	hint := "Check the image's entrypoint or command. If it is a shell script, confirm it has a valid shebang, Unix line endings and the executable bit."
	if cmd != "" {
		c.Explanation += ": " + cmd + " was not found"
		hint = "Check that " + cmd + " exists in the image and is on PATH. If it is a shell script, confirm it has a valid shebang, Unix line endings and the executable bit."
	}
	c.Explanation += "."
	c.Fixes = []Fix{{Label: "Fix the entrypoint or start command", Kind: FixManual, Hint: hint}}
	return c
}

func firstMatchAny(lines []line, res []*regexp.Regexp) (line, []string, bool) {
	for _, l := range lines {
		for _, re := range res {
			if m := re.FindStringSubmatch(l.text); m != nil {
				return l, m, true
			}
		}
	}
	return line{}, nil, false
}

var permissionRes = []*regexp.Regexp{
	regexp.MustCompile(`EACCES: permission denied, \w+ '([^']+)'`),
	regexp.MustCompile(`PermissionError: \[Errno 13\] Permission denied: '([^']+)'`),
	regexp.MustCompile(`cannot (?:create|open|access|write)[^:']*'([^']+)': Permission denied`),
	regexp.MustCompile(`open (/[^\s:]+): permission denied`),
	regexp.MustCompile(`mkdir (/[^\s:]+): permission denied`),
	regexp.MustCompile(`java\.io\.FileNotFoundException: (/[^\s]+) \(Permission denied\)`),
}

func rulePermissionDenied(_ Input, lines []line) *Cause {
	l, m, ok := firstMatchAny(lines, permissionRes)
	if !ok {
		return nil
	}
	path := m[1]
	return &Cause{
		Code:        CausePermissionDenied,
		Title:       "Permission denied on a path",
		Explanation: "The app's user cannot write to " + path + ". This usually means a mounted volume or bind mount is owned by root while the container runs as a non-root user.",
		Confidence:  ConfidenceHigh,
		Evidence:    []Signal{evidence(l)},
		Fixes: []Fix{{
			Label: "Change the owner of " + path + " to the container's user",
			Kind:  FixManual,
			Hint:  "Volumes and bind mounts cannot be edited here. On the node, chown the host directory (or volume) to the UID the container runs as, for example: chown -R <uid>:<gid> <host path>, then redeploy.",
		}},
	}
}

var portInUseRes = []*regexp.Regexp{
	regexp.MustCompile(`Bind for [^\s:]*:(\d+) failed: port is already allocated`),
	regexp.MustCompile(`EADDRINUSE[^\n]*?:(\d+)`),
	regexp.MustCompile(`listen tcp[^\n]*?:(\d+): bind: address already in use`),
	regexp.MustCompile(`(?i)address already in use[^\n]*?:(\d+)`),
}

func rulePortInUse(_ Input, lines []line) *Cause {
	l, m, ok := firstMatchAny(lines, portInUseRes)
	if !ok {
		if l2, ok2 := firstContains(lines, "port is already allocated", "address already in use", "eaddrinuse"); ok2 {
			l, m, ok = l2, []string{"", ""}, true
		}
	}
	if !ok {
		return nil
	}
	port := m[1]
	expl := "Something else is already listening on the port this app needs."
	hint := "Stop the other process or container using the port, or pin a different host port for this app."
	if port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			expl = fmt.Sprintf("Something else is already listening on port %d.", p)
			hint = fmt.Sprintf("Stop whatever is using port %d on the node (ss -ltnp), or pin a different host port for this app.", p)
		}
	}
	return &Cause{
		Code:        CausePortInUse,
		Title:       "Port already in use",
		Explanation: expl,
		Confidence:  ConfidenceHigh,
		Evidence:    []Signal{evidence(l)},
		Fixes:       []Fix{{Label: "Free the port or choose another", Kind: FixManual, Hint: hint}},
	}
}
