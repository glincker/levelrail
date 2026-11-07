package spec

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/kit/cronexpr"
)

// jobNameLike mirrors the job-name rule the HTTP API applies.
var jobNameLike = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)

// Job modes and catch-up values accepted in app.yaml.
const (
	JobModeFresh      = "fresh"
	JobModeExec       = "exec"
	JobCatchUpSkip    = "skip"
	JobCatchUpRunOnce = "run-once"
)

// validateJobs checks the jobs: list: unique names, a parseable cron
// expression and timezone, Go-syntax durations and the enums. Like hooks:,
// jobs make no sense for build types with no single container to run in.
func (svc *Service) validateJobs(name string) error {
	if len(svc.Jobs) == 0 {
		return nil
	}
	if svc.Build.Type == BuildStatic || svc.Build.Type == BuildCompose {
		return fmt.Errorf("spec: service %q: jobs is not meaningful for build.type %q, there is no single container to run a command in", name, svc.Build.Type)
	}
	seen := make(map[string]struct{}, len(svc.Jobs))
	for i := range svc.Jobs {
		j := &svc.Jobs[i]
		where := fmt.Sprintf("spec: service %q: jobs[%d]", name, i)
		if !jobNameLike.MatchString(j.Name) {
			return fmt.Errorf("%s: name %q must be lowercase letters, digits, hyphens or underscores", where, j.Name)
		}
		if _, dup := seen[j.Name]; dup {
			return fmt.Errorf("%s: duplicate job name %q", where, j.Name)
		}
		seen[j.Name] = struct{}{}
		if j.HTTP != nil {
			if len(j.Command) > 0 {
				return fmt.Errorf("%s (%s): set either command or http, not both", where, j.Name)
			}
			if err := validateJobHTTP(j.HTTP); err != nil {
				return fmt.Errorf("%s (%s): http: %w", where, j.Name, err)
			}
		} else if len(j.Command) == 0 {
			return fmt.Errorf("%s (%s): command or http is required", where, j.Name)
		}
		for _, part := range j.Command {
			if strings.TrimSpace(part) == "" {
				return fmt.Errorf("%s (%s): command must not contain empty arguments", where, j.Name)
			}
		}
		loc := time.UTC
		if j.Timezone != "" {
			l, err := time.LoadLocation(j.Timezone)
			if err != nil {
				return fmt.Errorf("%s (%s): timezone %q is not a known IANA time zone", where, j.Name, j.Timezone)
			}
			loc = l
		}
		if err := cronexpr.Validate(j.Schedule, loc, time.Now()); err != nil {
			return fmt.Errorf("%s (%s): schedule: %w", where, j.Name, err)
		}
		for field, v := range map[string]string{"timeout": j.Timeout, "retryBackoff": j.RetryBackoff, "catchUpLookback": j.CatchUpLookback, "warnAfter": j.WarnAfter} {
			if v == "" {
				continue
			}
			if d, err := time.ParseDuration(v); err != nil || d <= 0 {
				return fmt.Errorf("%s (%s): %s %q must be a positive duration such as 30s, 10m or 1h", where, j.Name, field, v)
			}
		}
		if j.Retries < 0 {
			return fmt.Errorf("%s (%s): retries must not be negative", where, j.Name)
		}
		if j.Mode == JobModeExec && (j.CPU != 0 || j.Memory != "") {
			return fmt.Errorf("%s (%s): cpu and memory limits apply to fresh-container jobs, not mode: exec", where, j.Name)
		}
	}
	return nil
}

// validateJobHTTP checks an http job's request. Secret values never live in
// the spec, so a header carries either a literal or a secretRef.
func validateJobHTTP(h *JobHTTP) error {
	if !strings.HasPrefix(h.Path, "/") || strings.HasPrefix(h.Path, "//") {
		return fmt.Errorf("path %q must start with a single /", h.Path)
	}
	h.Method = strings.ToUpper(strings.TrimSpace(h.Method))
	if h.Method == "" {
		h.Method = "GET"
	}
	if h.Method != "GET" && h.Method != "POST" {
		return fmt.Errorf("method %q must be GET or POST", h.Method)
	}
	for i, hd := range h.Headers {
		if !headerNameLike.MatchString(hd.Name) {
			return fmt.Errorf("headers[%d]: name %q is not a valid header name", i, hd.Name)
		}
		if (hd.Value == "") == (hd.SecretRef == "") {
			return fmt.Errorf("headers[%d] (%s): set exactly one of value or secretRef", i, hd.Name)
		}
		if hd.Prefix != "" && hd.SecretRef == "" {
			return fmt.Errorf("headers[%d] (%s): prefix only applies with secretRef", i, hd.Name)
		}
	}
	return nil
}

var headerNameLike = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)
