// Package failure classifies a failed, held or unhealthy deploy into one
// structured object shared by the API, CLI and MCP surfaces.
package failure

import (
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/diagnose"
	"github.com/GLINCKER/levelrail/internal/untrusted"
)

// Env vars that bound the log excerpt.
const (
	EnvExcerptLines = "APP_FAILURE_EXCERPT_LINES"
	EnvExcerptBytes = "APP_FAILURE_EXCERPT_BYTES"

	defaultExcerptLines = 20
	defaultExcerptBytes = 4000
	excerptTrailing     = 2
)

// Failure is the structured description of why a deploy did not succeed.
type Failure struct {
	Code         string    `json:"code"`
	Cause        string    `json:"cause"`
	FailingStep  string    `json:"failing_step,omitempty"`
	LogExcerpt   string    `json:"log_excerpt,omitempty"`
	SuggestedFix string    `json:"suggested_fix"`
	DocsURL      string    `json:"docs_url"`
	Retryable    bool      `json:"retryable"`
	DeployID     string    `json:"deploy_id"`
	App          string    `json:"app"`
	At           time.Time `json:"at"`
}

// Condition is one failing reconcile condition.
type Condition struct {
	Reason  string
	Message string
}

// Input is everything Classify reads. Only Status is required.
type Input struct {
	App         string
	DeployID    string
	Status      string
	Reason      string
	Error       string
	FailingStep string
	Condition   *Condition
	LogLines    []string
	Crashloop   bool
	At          time.Time
}

// Options bound the log excerpt.
type Options struct {
	MaxLines int
	MaxBytes int
}

// OptionsFromEnv reads Options from lookup, falling back to defaults.
func OptionsFromEnv(lookup func(string) (string, bool)) Options {
	return Options{
		MaxLines: envInt(lookup, EnvExcerptLines, defaultExcerptLines),
		MaxBytes: envInt(lookup, EnvExcerptBytes, defaultExcerptBytes),
	}
}

func envInt(lookup func(string) (string, bool), key string, def int) int {
	if lookup != nil {
		if v, ok := lookup(key); ok {
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
				return n
			}
		}
	}
	return def
}

// RolloutFailureReasons are the reconcile condition reasons that mean a
// finished build did not converge into a healthy container.
var RolloutFailureReasons = map[string]bool{
	"CreateFailed":             true,
	"StartFailed":              true,
	"ReadinessFailed":          true,
	"InspectFailed":            true,
	"EnsureNetworkFailed":      true,
	"VanishedAfterStart":       true,
	"PreDeployHookFailed":      true,
	"OOMKilledDuringReadiness": true,
	"ExitedDuringReadiness":    true,
}

const (
	statusFailed   = "failed"
	statusHeld     = "held"
	buildPhaseMark = ": build: "
)

// Classify returns the Failure for in, or false when the deploy is not in a
// failing or blocked state.
func Classify(in Input, opts Options) (Failure, bool) {
	if !isFailing(in) {
		return Failure{}, false
	}
	if opts.MaxLines <= 0 {
		opts.MaxLines = defaultExcerptLines
	}
	if opts.MaxBytes <= 0 {
		opts.MaxBytes = defaultExcerptBytes
	}

	lines := signalLines(in)
	cl, pattern := match(in, lines)
	if cl == nil {
		cl = fromDiagnose(in)
	}

	f := Failure{
		FailingStep: in.FailingStep,
		DeployID:    in.DeployID,
		App:         in.App,
		At:          in.At,
	}
	if cl == nil {
		f.Code = CodeUnknown
		f.Cause = unknownCause(in)
		f.SuggestedFix = "Read the log excerpt and the full deploy log for the failing step, then retry the deploy."
		f.DocsURL = docsURL(CodeUnknown)
		f.LogExcerpt = excerpt(lines, "", opts)
		return f, true
	}
	f.Code = cl.code
	f.Cause = cl.cause
	f.SuggestedFix = cl.fix
	f.DocsURL = docsURL(cl.code)
	f.Retryable = cl.retryable
	f.LogExcerpt = excerpt(lines, pattern, opts)
	return f, true
}

func isFailing(in Input) bool {
	switch in.Status {
	case statusFailed, statusHeld:
		return true
	}
	return in.Condition != nil && RolloutFailureReasons[in.Condition.Reason]
}

// signalLines returns the error and rollout message first, then log lines.
func signalLines(in Input) []string {
	var lines []string
	if in.Error != "" {
		lines = append(lines, splitLines(in.Error)...)
	}
	if in.Condition != nil {
		if in.Condition.Reason != "" {
			lines = append(lines, in.Condition.Reason)
		}
		lines = append(lines, splitLines(in.Condition.Message)...)
	}
	if in.Reason != "" {
		lines = append(lines, in.Reason)
	}
	return append(lines, in.LogLines...)
}

func splitLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

func inBuildPhase(in Input) bool {
	step := strings.ToLower(in.FailingStep)
	return strings.Contains(in.Error, buildPhaseMark) || strings.Contains(step, "build") || strings.Contains(step, "detect")
}

func match(in Input, lines []string) (*class, string) {
	lower := make([]string, len(lines))
	for i, l := range lines {
		lower[i] = strings.ToLower(l)
	}
	build := inBuildPhase(in)
	held := in.Status == statusHeld
	for i := range classes {
		c := &classes[i]
		if c.held != held || (c.buildOnly && !build) || (c.runtimeOnly && build) {
			continue
		}
		for _, p := range c.patterns {
			for _, l := range lower {
				if strings.Contains(l, p) {
					return c, p
				}
			}
		}
	}
	if in.Crashloop && !held {
		return classByCode(CodeContainerCrashed), ""
	}
	return nil, ""
}

// fromDiagnose reuses the runtime cause analysis as a second stage.
func fromDiagnose(in Input) *class {
	din := diagnose.Input{RecentLogLines: in.LogLines}
	din.Attempt = &diagnose.AttemptInput{Status: in.Status, Error: in.Error}
	if in.Condition != nil {
		din.Conditions = []diagnose.ConditionInput{{Type: "Ready", Status: "False", Reason: in.Condition.Reason, Message: in.Condition.Message}}
	}
	if in.Crashloop {
		din.Crashloop = &diagnose.CrashloopInput{Firing: true}
	}
	for _, c := range diagnose.Analyze(din) {
		code, ok := diagnoseCodes[c.Code]
		if !ok {
			continue
		}
		cl := classByCode(code)
		if cl == nil {
			continue
		}
		if len(c.Fixes) > 0 && c.Fixes[0].Label != "" {
			copied := *cl
			copied.fix = c.Fixes[0].Label
			return &copied
		}
		return cl
	}
	return nil
}

var diagnoseCodes = map[string]string{
	diagnose.CauseWrongPort:        CodePortNotListening,
	diagnose.CauseHealthcheckFail:  CodeHealthCheckFailed,
	diagnose.CausePortInUse:        CodePortNotListening,
	diagnose.CauseOOMKilled:        CodeOOMKilled,
	diagnose.CauseMissingEnv:       CodeMissingEnv,
	diagnose.CauseImagePullFailed:  CodeImagePullFailed,
	diagnose.CauseBuildOutOfDisk:   CodeDiskFull,
	diagnose.CausePermissionDenied: CodeContainerCrashed,
	diagnose.CauseExecFormatError:  CodeContainerCrashed,
	diagnose.CauseCommandNotFound:  CodeContainerCrashed,
	diagnose.CauseCrashloopGeneric: CodeContainerCrashed,
}

func unknownCause(in Input) string {
	if in.Error != "" {
		return "The deploy failed with an error that matches no known failure class: " + untrusted.Sanitize(firstLine(in.Error), 300)
	}
	return "The deploy is not healthy and no known failure class matched the available signals."
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// excerpt returns the window of lines ending just after the last line that
// matches pattern (or the tail when pattern is empty), cleaned and redacted.
func excerpt(lines []string, pattern string, opts Options) string {
	if len(lines) == 0 {
		return ""
	}
	end := len(lines)
	if pattern != "" {
		for i := len(lines) - 1; i >= 0; i-- {
			if strings.Contains(strings.ToLower(lines[i]), pattern) {
				end = min(len(lines), i+1+excerptTrailing)
				break
			}
		}
	}
	start := max(0, end-opts.MaxLines)
	out := make([]string, 0, end-start)
	for _, l := range lines[start:end] {
		out = append(out, untrusted.Redact(untrusted.Clean(l)))
	}
	return untrusted.Truncate(strings.Join(out, "\n"), opts.MaxBytes)
}
