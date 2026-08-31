// Package diagnose is a deterministic, pure-Go pattern matcher over
// diagnostic signals the platform already collects: deploy attempt
// status/error, reconcile condition reason strings, crashloop alert
// state, and recent log lines. It is the "read-and-suggest layer on top
// of the platform API" section 4.11 of the project's CLAUDE.md
// describes: it never calls an external model, never writes to any
// resource, and never influences the reconciler. Every explanation this
// package returns is backed by a literal signal it read; when nothing
// matches, Diagnose returns the fallback response rather than a guess.
package diagnose

import "strings"

// Confidence levels Result.Confidence can hold.
const (
	ConfidenceHigh   = "high"
	ConfidenceMedium = "medium"
	ConfidenceNone   = "none"
)

// Signal is one piece of evidence a signature matched against, always
// tied to the literal text it was read from.
type Signal struct {
	Source  string
	Excerpt string
}

// Result is what Diagnose returns.
type Result struct {
	Explanation    string
	Suggestion     string
	Confidence     string
	MatchedSignals []Signal
}

// AttemptInput is the deploy attempt signal, nil if the caller found no
// attempt to diagnose (e.g. an app that has never been deployed).
type AttemptInput struct {
	Status string
	Error  string
}

// ConditionInput is one reconcile condition, the same shape
// GET /api/v1/apps/{name}/deploys already returns.
type ConditionInput struct {
	Type, Status, Reason, Message string
}

// CrashloopInput is a crashloop alert rule's current evaluation state,
// nil if no crashloop rule is configured for this app.
type CrashloopInput struct {
	Firing                bool
	RestartCount          int
	RestartCountThreshold int
	RestartWindow         string
}

// Input bundles every signal Diagnose considers. Every field is
// optional except ServiceName: a caller with fewer signals available
// (no telemetry configured, no crashloop rule set up) still gets a
// best-effort result, it just has less evidence to draw on.
type Input struct {
	ServiceName    string
	Attempt        *AttemptInput
	Conditions     []ConditionInput
	Crashloop      *CrashloopInput
	RecentLogLines []string
}

type textSource struct {
	source string
	text   string
}

func (in Input) textSources() []textSource {
	var out []textSource
	if in.Attempt != nil && in.Attempt.Error != "" {
		out = append(out, textSource{"deploy_attempt.error", in.Attempt.Error})
	}
	for _, c := range in.Conditions {
		if c.Message != "" {
			out = append(out, textSource{"condition:" + c.Type + "." + c.Reason, c.Message})
		}
	}
	if len(in.RecentLogLines) > 0 {
		out = append(out, textSource{"logs", strings.Join(in.RecentLogLines, "\n")})
	}
	return out
}

type signature struct {
	reason      string
	patterns    []string // lowercase substrings, any one is enough to match
	confidence  string
	explanation string
	suggestion  string
}

type match struct {
	sig    signature
	signal Signal
}

// signatures is checked in order; the first match becomes Result's
// primary Explanation/Suggestion/Confidence, ordered roughly from most
// fundamental (nothing else can be evaluated) to most specific to least.
var signatures = []signature{
	{
		reason:      "docker_daemon_unreachable",
		patterns:    []string{"cannot connect to the docker daemon", "dial unix", "connect: connection refused", "the docker daemon is not running"},
		confidence:  ConfidenceHigh,
		explanation: "The control plane could not reach the Docker daemon while attempting this deploy. Container operations fail immediately when the daemon itself is unreachable, before anything about the app's own image or config is evaluated.",
		suggestion:  "Confirm the Docker daemon is running and reachable on the target node, then retry the deploy.",
	},
	{
		reason:      "image_pull_failure",
		patterns:    []string{"no such image", "pull access denied", "manifest unknown", "manifest for", "repository does not exist", "unauthorized: authentication required", "toomanyrequests", "requested access to the resource is denied"},
		confidence:  ConfidenceHigh,
		explanation: "Docker could not pull or find the container image this deploy is trying to run.",
		suggestion:  "Check that the image name and tag are correct and were actually pushed; if it's in a private registry, confirm a registry credential is configured for this service.",
	},
	{
		reason:      "build_missing_dockerfile",
		patterns:    []string{"dockerfile: no such file or directory", "failed to read dockerfile", "no such file or directory: dockerfile", "cannot locate dockerfile", "unable to prepare context"},
		confidence:  ConfidenceHigh,
		explanation: "The build could not find a Dockerfile at the path this service's build config points to.",
		suggestion:  "Check app.yaml's build.path (and build.baseDirectory, if set) match where the Dockerfile actually lives in the repo.",
	},
	{
		reason:      "build_dependency_failure",
		patterns:    []string{"npm err!", "eresolve", "could not resolve host", "unable to locate package", "no matching distribution found", "err_pnpm", "process \"/bin/sh"},
		confidence:  ConfidenceMedium,
		explanation: "The build ran but a dependency installation step inside it failed.",
		suggestion:  "Check the build log for the failing install command (npm/pip/yarn/etc.) and confirm the dependency and lockfile are correct and reachable from the build environment.",
	},
	{
		reason:      "port_conflict",
		patterns:    []string{"port is already allocated", "address already in use", "bind: address already in use"},
		confidence:  ConfidenceHigh,
		explanation: "The container could not bind to its configured port because something else on the host is already using it.",
		suggestion:  "Check for another container or process already bound to this service's host port, or change app.yaml's port to a free one.",
	},
	{
		reason:      "health_check_timeout",
		patterns:    []string{"readiness probe", "timed out waiting for"},
		confidence:  ConfidenceHigh,
		explanation: "The new container started but never passed its readiness health check within the configured timeout.",
		suggestion:  "Confirm the service actually listens on the configured port and that health.readiness.path returns a successful response quickly; raise the timeout if the app just needs more startup time.",
	},
	{
		reason:      "possible_oom_kill",
		patterns:    []string{"oomkilled", "out of memory", "exit code 137", "code: 137", "exit status 137"},
		confidence:  ConfidenceMedium,
		explanation: "A signal consistent with the container being killed for using too much memory (OOM) was found.",
		suggestion:  "Check the service's memory usage against app.yaml's resources.memory limit, and consider raising it or reducing the app's memory footprint.",
	},
}

// crashloopSignature is checked separately from signatures above: its
// match isn't a text pattern, it's CrashloopInput.Firing. Deliberately
// last in priority: if a more specific signature (e.g. possible_oom_kill)
// already explains why the app is crashlooping, that's a more useful
// primary explanation than the crashloop symptom itself.
var crashloopSignature = signature{
	reason:      "crashloop_restart_loop",
	confidence:  ConfidenceHigh,
	explanation: "This app's containers are repeatedly restarting: the crashloop alert has fired because the restart count exceeded its configured threshold within the configured window.",
	suggestion:  "Check the app's recent logs for what happens immediately after each start; a process crashing right after startup (bad config, a missing required env var, wrong entrypoint/command) is the most common cause.",
}

const maxExcerptLen = 400

// Diagnose synthesizes in's signals into a human-readable explanation
// and suggested next action. Deterministic: the same Input always
// produces the same Result.
func Diagnose(in Input) Result {
	sources := in.textSources()

	var matched []match
	for _, sig := range signatures {
		ts, excerpt, ok := matchPattern(sources, sig.patterns)
		if !ok {
			continue
		}
		matched = append(matched, match{sig, Signal{Source: ts.source, Excerpt: excerpt}})
	}

	if in.Crashloop != nil && in.Crashloop.Firing {
		matched = append(matched, match{crashloopSignature, Signal{
			Source:  "crashloop_rule",
			Excerpt: crashloopExcerpt(*in.Crashloop),
		}})
	}

	if len(matched) == 0 {
		return fallback(in)
	}

	primary := matched[0]
	signals := make([]Signal, 0, len(matched))
	for _, m := range matched {
		signals = append(signals, m.signal)
	}
	return Result{
		Explanation:    primary.sig.explanation,
		Suggestion:     primary.sig.suggestion,
		Confidence:     primary.sig.confidence,
		MatchedSignals: signals,
	}
}

func matchPattern(sources []textSource, patterns []string) (ts textSource, excerpt string, ok bool) {
	for _, s := range sources {
		lower := strings.ToLower(s.text)
		for _, p := range patterns {
			if strings.Contains(lower, p) {
				return s, excerptFor(s, p), true
			}
		}
	}
	return textSource{}, "", false
}

// excerptFor returns the specific evidence that triggered a match. For
// a multi-line log block, only the matching line is quoted, not the
// whole window; every other source is short enough already to quote in
// full (truncated defensively).
func excerptFor(ts textSource, pattern string) string {
	if ts.source == "logs" {
		for _, line := range strings.Split(ts.text, "\n") {
			if strings.Contains(strings.ToLower(line), pattern) {
				return truncate(strings.TrimSpace(line))
			}
		}
	}
	return truncate(strings.TrimSpace(ts.text))
}

func truncate(s string) string {
	if len(s) <= maxExcerptLen {
		return s
	}
	return s[:maxExcerptLen] + "..."
}

func crashloopExcerpt(c CrashloopInput) string {
	window := c.RestartWindow
	if window == "" {
		window = "the configured window"
	}
	return "restarted " + itoa(c.RestartCount) + " times within " + window + " (threshold " + itoa(c.RestartCountThreshold) + ")"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// fallback is Diagnose's response when no known signature matched: it
// still surfaces whatever raw signal is available (an attempt's error,
// or a failing condition) rather than asserting a cause nothing backs.
func fallback(in Input) Result {
	var signals []Signal
	if in.Attempt != nil && in.Attempt.Status == "failed" && in.Attempt.Error != "" {
		signals = append(signals, Signal{Source: "deploy_attempt.error", Excerpt: truncate(in.Attempt.Error)})
	}
	for _, c := range in.Conditions {
		if c.Status == "False" {
			signals = append(signals, Signal{Source: "condition:" + c.Type, Excerpt: truncate(c.Reason + ": " + c.Message)})
		}
	}

	if len(signals) == 0 {
		return Result{
			Explanation: "No failure signal was found for this app: no failed deploy attempt, no failing reconcile condition, and no firing crashloop alert.",
			Suggestion:  "Nothing to diagnose right now.",
			Confidence:  ConfidenceNone,
		}
	}
	return Result{
		Explanation:    "None of Levelrail's known failure patterns matched the available signals for this app.",
		Suggestion:     "Review the raw signal below; this may be a failure mode not yet covered by automatic diagnosis.",
		Confidence:     ConfidenceNone,
		MatchedSignals: signals,
	}
}
