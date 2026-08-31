package compose

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// healthcheckURLPattern locates an http(s):// URL embedded in a
// CMD/CMD-SHELL healthcheck command string, the shape a curl or wget
// based check declares its target in.
var healthcheckURLPattern = regexp.MustCompile(`https?://[^\s"']+`)

// composeHealth is a compose healthcheck: block already resolved into
// this platform's own HTTP-path readiness shape.
type composeHealth struct {
	Path     string
	Interval time.Duration
	Timeout  time.Duration
	Failures int
}

// resolveHealthcheck interprets serviceKey's healthcheck: block. A nil
// hc, an empty test, or an explicit "NONE" all mean no check at all:
// (nil, "", nil). A real, non-HTTP command (pg_isready, redis-cli ping,
// a bare TCP probe, ...) returns (nil, warning, nil): this platform never
// fabricates a fake HTTP path for a check it can't actually express. err
// is only non-nil for a structurally broken block (CMD-SHELL with no
// command, or a malformed interval/timeout duration).
func resolveHealthcheck(serviceKey string, hc *Healthcheck) (*composeHealth, string, error) {
	if hc == nil || len(hc.Test) == 0 {
		return nil, "", nil
	}

	command, disabled, err := healthcheckCommand(serviceKey, hc.Test)
	if err != nil {
		return nil, "", err
	}
	if disabled {
		return nil, "", nil
	}

	path, ok := httpPathFromCommand(command)
	if !ok {
		warning := fmt.Sprintf("service %q: healthcheck command %q is not an HTTP check this platform can translate into a readiness probe; deploy it with an app.yaml health: block instead if one is needed", serviceKey, command)
		return nil, warning, nil
	}

	interval, err := parseComposeDuration(hc.Interval)
	if err != nil {
		return nil, "", fmt.Errorf("service %q: healthcheck.interval: %w", serviceKey, err)
	}
	timeout, err := parseComposeDuration(hc.Timeout)
	if err != nil {
		return nil, "", fmt.Errorf("service %q: healthcheck.timeout: %w", serviceKey, err)
	}

	return &composeHealth{Path: path, Interval: interval, Timeout: timeout, Failures: hc.Retries}, "", nil
}

// healthcheckCommand normalizes test's CMD/CMD-SHELL/NONE prefix into a
// single shell command string. A list with no recognized prefix is
// treated the same as CMD-SHELL joined by spaces, matching real Compose's
// own leniency for that shape.
func healthcheckCommand(serviceKey string, test []string) (command string, disabled bool, err error) {
	switch test[0] {
	case "NONE":
		return "", true, nil
	case "CMD-SHELL":
		if len(test) < 2 {
			return "", false, fmt.Errorf("service %q: healthcheck: CMD-SHELL requires a command string", serviceKey)
		}
		return test[1], false, nil
	case "CMD":
		return strings.Join(test[1:], " "), false, nil
	default:
		return strings.Join(test, " "), false, nil
	}
}

// httpPathFromCommand extracts a readiness path from a curl/wget command
// string, the common real-world way a Compose healthcheck expresses an
// HTTP check. ok is false for anything else (a database ping tool, a
// bare TCP check, ...) rather than guessing at a path.
func httpPathFromCommand(command string) (string, bool) {
	lower := strings.ToLower(command)
	if !strings.Contains(lower, "curl") && !strings.Contains(lower, "wget") {
		return "", false
	}
	match := healthcheckURLPattern.FindString(command)
	if match == "" {
		return "", false
	}
	u, err := url.Parse(match)
	if err != nil {
		return "", false
	}
	if u.Path == "" {
		return "/", true
	}
	return u.Path, true
}

// parseComposeDuration treats "" as "not specified", matching
// internal/deploy's own parseDurationOrZero for app.yaml's health config.
// Compose's own duration suffixes (h/m/s/ms/us) are the same shape Go's
// time.ParseDuration accepts, so no separate parser is needed.
func parseComposeDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: %w", s, err)
	}
	return d, nil
}

// toStoreHealth converts h into the shape ToDesiredServices' direct
// compose-import path stores directly, gating that service's own
// blue-green cutover the same way an app.yaml-declared readiness probe
// already does.
func (h *composeHealth) toStoreHealth() *store.ServiceHealth {
	if h == nil {
		return nil
	}
	return &store.ServiceHealth{
		Readiness: &store.ServiceProbe{Path: h.Path, Interval: h.Interval, Timeout: h.Timeout, Failures: h.Failures},
	}
}

// toSpecHealth converts h into the shape ExpandBuildService's
// git-sourced path returns, mirrored back into app.yaml's own
// string-duration form since internal/deploy.toServiceProbe is what
// parses it back into a time.Duration.
func (h *composeHealth) toSpecHealth() *spec.Health {
	if h == nil {
		return nil
	}
	return &spec.Health{
		Readiness: &spec.Probe{Path: h.Path, Interval: durationToSpecString(h.Interval), Timeout: durationToSpecString(h.Timeout), Failures: h.Failures},
	}
}

func durationToSpecString(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	return d.String()
}
