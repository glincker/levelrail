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
		if len(j.Command) == 0 {
			return fmt.Errorf("%s (%s): command is required", where, j.Name)
		}
		for _, part := range j.Command {
			if strings.TrimSpace(part) == "" {
				return fmt.Errorf("%s (%s): command must not contain empty arguments", where, j.Name)
			}
		}
		if _, err := cronexpr.Parse(j.Schedule); err != nil {
			return fmt.Errorf("%s (%s): schedule: %w", where, j.Name, err)
		}
		if j.Timezone != "" {
			if _, err := time.LoadLocation(j.Timezone); err != nil {
				return fmt.Errorf("%s (%s): timezone %q is not a known IANA time zone", where, j.Name, j.Timezone)
			}
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
