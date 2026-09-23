package compose

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	probepkg "github.com/GLINCKER/levelrail/internal/probe"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// healthcheckURLPattern locates an http(s):// URL embedded in a
// CMD/CMD-SHELL healthcheck command string, the shape a curl or wget
// based check declares its target in.
var healthcheckURLPattern = regexp.MustCompile(`https?://[^\s"']+`)

// composeHealth is a compose healthcheck: block resolved into this
// platform's readiness probe shape.
type composeHealth struct {
	store.ServiceProbe
	ReadyTimeout time.Duration
}

// tcpOnlyPattern matches the bash-only "< /dev/tcp/host/port" connect
// check: it proves nothing an HTTP or exec probe could not, and fails
// outright under the busybox/dash shells most images ship.
var tcpOnlyPattern = regexp.MustCompile(`/dev/tcp/`)

// resolveHealthcheck interprets serviceKey's healthcheck: block. A nil hc,
// an empty test, or "NONE" mean no check: (nil, "", nil). A curl/wget
// command becomes an HTTP(S) probe honoring its redirect, TLS and -f
// flags; any other command becomes an exec probe run inside the
// container, except a bare /dev/tcp connect check, which is left unset
// with a warning. err is only for a structurally broken block.
func resolveHealthcheck(serviceKey string, hc *Healthcheck) (*composeHealth, string, error) {
	if hc == nil || len(hc.Test) == 0 {
		return nil, "", nil
	}

	command, argv, disabled, err := healthcheckCommand(serviceKey, hc.Test)
	if err != nil {
		return nil, "", err
	}
	if disabled {
		return nil, "", nil
	}

	probe, ok := httpProbeFromCommand(command)
	if !ok {
		if tcpOnlyPattern.MatchString(command) {
			warning := fmt.Sprintf("service %q: healthcheck command %q is a bare TCP connect check, which has no readiness equivalent here; deploy it with an app.yaml health: block (HTTP path or exec) if one is needed", serviceKey, command)
			return nil, warning, nil
		}
		probe = store.ServiceProbe{Exec: argv}
	}

	interval, err := parseComposeDuration(hc.Interval)
	if err != nil {
		return nil, "", fmt.Errorf("service %q: healthcheck.interval: %w", serviceKey, err)
	}
	timeout, err := parseComposeDuration(hc.Timeout)
	if err != nil {
		return nil, "", fmt.Errorf("service %q: healthcheck.timeout: %w", serviceKey, err)
	}
	startPeriod, err := parseComposeDuration(hc.StartPeriod)
	if err != nil {
		return nil, "", fmt.Errorf("service %q: healthcheck.start_period: %w", serviceKey, err)
	}
	probe.Interval, probe.Timeout, probe.Failures = interval, timeout, hc.Retries

	out := &composeHealth{ServiceProbe: probe}
	if startPeriod > 0 {
		// Docker's own budget: failures inside start_period don't count,
		// then retries consecutive failures at interval mark it unhealthy.
		out.ReadyTimeout = startPeriod + interval*time.Duration(max(hc.Retries, 1))
	}
	return out, "", nil
}

// healthcheckCommand normalizes test into a shell command string (for
// matching) and the argv an exec probe would run. CMD keeps its argv;
// CMD-SHELL, and a list with no recognized prefix (Compose's own
// leniency), run through /bin/sh -c.
func healthcheckCommand(serviceKey string, test []string) (command string, argv []string, disabled bool, err error) {
	switch test[0] {
	case "NONE":
		return "", nil, true, nil
	case "CMD-SHELL":
		if len(test) < 2 {
			return "", nil, false, fmt.Errorf("service %q: healthcheck: CMD-SHELL requires a command string", serviceKey)
		}
		return test[1], probepkg.ShellCommand(test[1]), false, nil
	case "CMD":
		if len(test) < 2 {
			return "", nil, false, fmt.Errorf("service %q: healthcheck: CMD requires a command", serviceKey)
		}
		return strings.Join(test[1:], " "), test[1:], false, nil
	default:
		joined := strings.Join(test, " ")
		return joined, probepkg.ShellCommand(joined), false, nil
	}
}

// httpProbeFromCommand maps a curl/wget command onto an HTTP probe with
// the same semantics: curl follows redirects only with -L and accepts any
// status below 400 with -f; wget follows redirects by default; -k and
// --no-check-certificate skip TLS verification. ok is false for anything
// that is not a curl/wget call to a URL.
func httpProbeFromCommand(command string) (store.ServiceProbe, bool) {
	fields := strings.Fields(command)
	var tool string
	for _, f := range fields {
		base := f[strings.LastIndex(f, "/")+1:]
		if base == "curl" || base == "wget" {
			tool = base
			break
		}
	}
	if tool == "" {
		return store.ServiceProbe{}, false
	}
	match := healthcheckURLPattern.FindString(command)
	if match == "" {
		return store.ServiceProbe{}, false
	}
	u, err := url.Parse(match)
	if err != nil {
		return store.ServiceProbe{}, false
	}

	p := store.ServiceProbe{Path: u.Path}
	if p.Path == "" {
		p.Path = "/"
	}
	if u.Scheme == probepkg.SchemeHTTPS {
		p.Scheme = probepkg.SchemeHTTPS
	}
	if tool == "curl" {
		follow := hasFlag(fields, "-L", "--location", 'L')
		p.FollowRedirects = &follow
		if hasFlag(fields, "-f", "--fail", 'f') || hasFlag(fields, "", "--fail-with-body", 0) {
			p.ExpectedStatus = "200-399"
		}
		p.TLSSkipVerify = p.Scheme == probepkg.SchemeHTTPS && hasFlag(fields, "-k", "--insecure", 'k')
	} else {
		p.TLSSkipVerify = p.Scheme == probepkg.SchemeHTTPS && hasFlag(fields, "", "--no-check-certificate", 0)
	}
	return p, true
}

// hasFlag reports whether fields carries short, long, or letter inside a
// combined short-flag group such as -fsSL.
func hasFlag(fields []string, short, long string, letter byte) bool {
	for _, f := range fields {
		if (short != "" && f == short) || f == long {
			return true
		}
		if letter != 0 && len(f) > 1 && f[0] == '-' && f[1] != '-' && strings.IndexByte(f[1:], letter) >= 0 {
			return true
		}
	}
	return false
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

// toStoreHealth converts h into the shape ToDesiredServices stores.
func (h *composeHealth) toStoreHealth() *store.ServiceHealth {
	if h == nil {
		return nil
	}
	p := h.ServiceProbe
	return &store.ServiceHealth{Readiness: &p, ReadyTimeout: h.ReadyTimeout}
}

// toSpecHealth converts h into app.yaml's string-duration shape for
// ExpandBuildService's git-sourced path.
func (h *composeHealth) toSpecHealth() *spec.Health {
	if h == nil {
		return nil
	}
	return &spec.Health{
		Readiness: &spec.Probe{
			Path:            h.Path,
			Scheme:          h.Scheme,
			TLSSkipVerify:   h.TLSSkipVerify,
			FollowRedirects: h.FollowRedirects,
			ExpectedStatus:  spec.StatusCodes(h.ExpectedStatus),
			Exec:            spec.ExecCommand(h.Exec),
			Interval:        durationToSpecString(h.Interval),
			Timeout:         durationToSpecString(h.Timeout),
			Failures:        h.Failures,
		},
		ReadyTimeout: durationToSpecString(h.ReadyTimeout),
	}
}

func durationToSpecString(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	return d.String()
}
