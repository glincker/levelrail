package spec

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/kit/probe"
)

// Platform probe kinds.
const (
	PlatformProbeHTTP = "http"
	PlatformProbeTCP  = "tcp"
)

// PlatformProbe is a health check the platform runs from outside the
// container against its published port, for images with no shell, curl or
// wget to exec into (scratch and distroless builds). Zero durations and
// retries fall back to the APP_PROBE_* defaults.
type PlatformProbe struct {
	Kind         string      `yaml:"kind" json:"kind"`
	Path         string      `yaml:"path,omitempty" json:"path,omitempty"`
	Port         int         `yaml:"port,omitempty" json:"port,omitempty"`
	Interval     string      `yaml:"interval,omitempty" json:"interval,omitempty"`
	Timeout      string      `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	Retries      int         `yaml:"retries,omitempty" json:"retries,omitempty"`
	StartPeriod  string      `yaml:"start_period,omitempty" json:"start_period,omitempty"`
	ExpectStatus StatusCodes `yaml:"expect_status,omitempty" json:"expect_status,omitempty"`
}

// PlatformProbeDurations are a PlatformProbe's parsed duration fields.
type PlatformProbeDurations struct {
	Interval, Timeout, StartPeriod time.Duration
}

// Validate checks p and returns its parsed durations.
func (p PlatformProbe) Validate() (PlatformProbeDurations, error) {
	var d PlatformProbeDurations
	switch p.Kind {
	case PlatformProbeHTTP:
		if p.Path != "" && !strings.HasPrefix(p.Path, "/") {
			return d, fmt.Errorf("path %q must start with /", p.Path)
		}
		if _, err := probe.ParseStatusSet(string(p.ExpectStatus)); err != nil {
			return d, err
		}
	case PlatformProbeTCP:
		if p.Path != "" || p.ExpectStatus != "" {
			return d, errors.New("a tcp probe cannot set path or expect_status")
		}
	default:
		return d, fmt.Errorf("kind %q must be %q or %q", p.Kind, PlatformProbeHTTP, PlatformProbeTCP)
	}
	if p.Port < 0 || p.Port > 65535 {
		return d, fmt.Errorf("port %d is out of range", p.Port)
	}
	if p.Retries < 0 {
		return d, errors.New("retries must not be negative")
	}
	for _, f := range []struct {
		name string
		raw  string
		dst  *time.Duration
	}{{"interval", p.Interval, &d.Interval}, {"timeout", p.Timeout, &d.Timeout}, {"start_period", p.StartPeriod, &d.StartPeriod}} {
		if f.raw == "" {
			continue
		}
		v, err := time.ParseDuration(f.raw)
		if err != nil {
			return d, fmt.Errorf("%s: invalid duration %q: %w", f.name, f.raw, err)
		}
		if v < 0 {
			return d, fmt.Errorf("%s must not be negative", f.name)
		}
		*f.dst = v
	}
	return d, nil
}

// ProbePath is the HTTP path to request, "/" when unset.
func (p PlatformProbe) ProbePath() string {
	if p.Kind == PlatformProbeHTTP && p.Path == "" {
		return "/"
	}
	return p.Path
}
