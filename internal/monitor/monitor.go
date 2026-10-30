// Package monitor runs operator-defined uptime checks and records their results.
package monitor

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Monitor kinds.
const (
	KindHTTP = "http"
	KindTCP  = "tcp"
)

// Location labels results from this control plane, the only vantage point so far.
const Location = "cp"

// Sentinel errors the API maps to status codes.
var (
	ErrInvalid  = errors.New("monitor: invalid")
	ErrNotFound = errors.New("monitor: not found")
	ErrLimit    = errors.New("monitor: limit reached")
)

// StatusRange is an inclusive HTTP status interval.
type StatusRange struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// Spec is the kind specific part of a monitor. Header values never live here, only their names.
type Spec struct {
	URL             string        `json:"url,omitempty"`
	Method          string        `json:"method,omitempty"`
	HeaderNames     []string      `json:"header_names,omitempty"`
	Expect          []StatusRange `json:"expect,omitempty"`
	Contains        string        `json:"contains,omitempty"`
	FollowRedirects bool          `json:"follow_redirects,omitempty"`
	Host            string        `json:"host,omitempty"`
	Port            int           `json:"port,omitempty"`
}

// Monitor is one check definition.
type Monitor struct {
	ID        string
	Name      string
	Kind      string
	Interval  time.Duration
	Timeout   time.Duration
	Enabled   bool
	Spec      Spec
	CreatedAt time.Time
}

// Result is one probe outcome. Reason is a short class, never a body or header value.
type Result struct {
	MonitorID string
	At        int64
	OK        bool
	LatencyMS int
	Code      int
	Reason    string
}

// Rollup is one hour bucket of results for a monitor.
type Rollup struct {
	MonitorID string
	Bucket    int64
	Total     int
	Failed    int
	LatSum    int64
	LatMax    int
}

var headerNameRe = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)

// Normalize fills defaults so stored and scheduled monitors are complete.
func Normalize(m Monitor) Monitor {
	m.Name = strings.TrimSpace(m.Name)
	if m.Kind == KindHTTP {
		if m.Spec.Method == "" {
			m.Spec.Method = http.MethodGet
		}
		m.Spec.Method = strings.ToUpper(m.Spec.Method)
		if len(m.Spec.Expect) == 0 {
			m.Spec.Expect = []StatusRange{{Min: 200, Max: 399}}
		}
	}
	return m
}

// Validate checks a normalized monitor against the configured limits. Errors wrap ErrInvalid.
func Validate(m Monitor, cfg Config) error {
	bad := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
	}
	if m.Name == "" || len(m.Name) > 120 {
		return bad("name is required and at most 120 characters")
	}
	floor := cfg.MinInterval(m.Kind)
	envName := "APP_MONITOR_MIN_INTERVAL_" + strings.ToUpper(m.Kind)
	if floor == 0 {
		return bad("kind must be http or tcp")
	}
	if m.Interval < floor {
		return bad("interval %s is below the minimum %s (%s)", m.Interval, floor, envName)
	}
	if m.Timeout <= 0 {
		return bad("timeout must be positive")
	}
	if m.Timeout > cfg.MaxTimeout {
		return bad("timeout %s is above the maximum %s (APP_MONITOR_MAX_TIMEOUT)", m.Timeout, cfg.MaxTimeout)
	}
	if m.Timeout >= m.Interval {
		return bad("timeout must be shorter than the interval")
	}
	if m.Kind == KindTCP {
		return validateTCP(m.Spec, bad)
	}
	return validateHTTP(m.Spec, cfg, bad)
}

func validateTCP(s Spec, bad func(string, ...any) error) error {
	if s.Host == "" || strings.ContainsAny(s.Host, " /@") {
		return bad("host is required")
	}
	if s.Port < 1 || s.Port > 65535 {
		return bad("port must be 1 to 65535")
	}
	return nil
}

func validateHTTP(s Spec, cfg Config, bad func(string, ...any) error) error {
	u, err := url.Parse(s.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return bad("url must be an absolute http or https URL")
	}
	if u.User != nil {
		return bad("url must not embed credentials, use a header")
	}
	switch s.Method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut:
	default:
		return bad("method must be GET, HEAD, POST or PUT")
	}
	if len(s.HeaderNames) > cfg.MaxHeaders {
		return bad("at most %d headers (APP_MONITOR_HEADER_MAX_COUNT)", cfg.MaxHeaders)
	}
	for _, n := range s.HeaderNames {
		if !headerNameRe.MatchString(n) {
			return bad("header name %q is not valid", n)
		}
	}
	for _, r := range s.Expect {
		if r.Min < 100 || r.Max > 599 || r.Min > r.Max {
			return bad("expected status ranges must sit within 100 to 599")
		}
	}
	if len(s.Contains) > cfg.BodyMaxBytes {
		return bad("contains is longer than the body limit (APP_MONITOR_BODY_MAX_BYTES)")
	}
	if s.Contains != "" && s.Method == http.MethodHead {
		return bad("contains needs a method that returns a body")
	}
	return nil
}
