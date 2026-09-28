package diagnose

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Cause codes a typed diagnosis can carry.
const (
	CauseWrongPort        = "WRONG_PORT"
	CauseOOMKilled        = "OOM_KILLED"
	CauseMissingEnv       = "MISSING_ENV"
	CausePortInUse        = "PORT_IN_USE"
	CausePermissionDenied = "PERMISSION_DENIED"
	CauseExecFormatError  = "EXEC_FORMAT_ERROR"
	CauseCommandNotFound  = "COMMAND_NOT_FOUND"
	CauseHealthcheckFail  = "HEALTHCHECK_FAILING"
	CauseCrashloopGeneric = "CRASHLOOP_GENERIC"
	CauseImagePullFailed  = "IMAGE_PULL_FAILED"
	CauseBuildOutOfDisk   = "BUILD_OUT_OF_DISK"
)

// Fix kinds.
const (
	FixPatch  = "patch"  // every change has a concrete value, safe to apply as previewed
	FixInput  = "input"  // at least one change needs a value the operator supplies
	FixManual = "manual" // no API setter exists, Hint says what to do
)

// Change is one field edit a fix proposes. Field uses the app resource's
// own JSON paths, with env vars as "env.NAME".
type Change struct {
	Field      string
	From       string
	To         string
	NeedsInput bool
}

// Fix is one remedy for a Cause. N is 1-based and unique across a Result.
type Fix struct {
	N        int
	Label    string
	Kind     string
	Changes  []Change
	Hint     string
	Redeploy bool
}

// Cause is one typed, evidence-backed reason for a failure.
type Cause struct {
	Code        string
	Title       string
	Explanation string
	Confidence  string
	Evidence    []Signal
	Fixes       []Fix
}

// Facts are structured observations about the app and its container that
// the log text alone cannot supply. Zero values mean "unknown".
type Facts struct {
	Port           int
	HealthPath     string
	MemoryBytes    int64
	OOMFactor      float64
	HasExit        bool
	ExitCode       int
	OOMKilled      bool
	NodeArch       string
	ListeningPorts []int
	ExposedPorts   []int
	EnvKeys        []string
	BuildFailed    bool
}

type line struct {
	source string
	text   string
}

type rule func(in Input, lines []line) *Cause

var rules = []rule{
	ruleOOMKilled,
	ruleMissingEnv,
	ruleImagePull,
	ruleBuildOutOfDisk,
	ruleExecFormat,
	ruleCommandNotFound,
	rulePermissionDenied,
	rulePortInUse,
	ruleWrongPort,
	ruleHealthcheck,
}

// Analyze runs every typed rule over in and returns causes ordered by
// confidence, then rule order, with fixes numbered across the whole slice.
func Analyze(in Input) []Cause {
	lines := in.lines()
	var causes []Cause
	for _, r := range rules {
		if c := r(in, lines); c != nil {
			causes = append(causes, *c)
		}
	}
	if len(causes) == 0 {
		if c := crashloopCause(in); c != nil {
			causes = append(causes, *c)
		}
	}
	sort.SliceStable(causes, func(i, j int) bool {
		return confidenceRank(causes[i].Confidence) > confidenceRank(causes[j].Confidence)
	})
	n := 0
	for i := range causes {
		for j := range causes[i].Fixes {
			n++
			causes[i].Fixes[j].N = n
		}
	}
	return causes
}

func confidenceRank(c string) int {
	switch c {
	case ConfidenceHigh:
		return 2
	case ConfidenceMedium:
		return 1
	}
	return 0
}

func (in Input) lines() []line {
	var out []line
	for _, ts := range in.textSources() {
		for _, l := range strings.Split(ts.text, "\n") {
			out = append(out, line{ts.source, l})
		}
	}
	return out
}

// firstMatch returns the first line matching re and its submatches.
func firstMatch(lines []line, re *regexp.Regexp) (line, []string, bool) {
	for _, l := range lines {
		if m := re.FindStringSubmatch(l.text); m != nil {
			return l, m, true
		}
	}
	return line{}, nil, false
}

// firstContains returns the first line containing any lowercase needle.
func firstContains(lines []line, needles ...string) (line, bool) {
	for _, l := range lines {
		low := strings.ToLower(l.text)
		for _, n := range needles {
			if strings.Contains(low, n) {
				return l, true
			}
		}
	}
	return line{}, false
}

func evidence(l line) Signal {
	return Signal{Source: l.source, Excerpt: truncate(strings.TrimSpace(l.text))}
}

func containsInt(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

func itoa64(n int64) string { return strconv.FormatInt(n, 10) }
