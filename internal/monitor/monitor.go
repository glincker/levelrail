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
	// KindHeartbeat expects an inbound ping from a cron job within interval plus grace.
	KindHeartbeat = "heartbeat"
	// KindTLS watches the certificate expiry of a host and port.
	KindTLS = "tls"
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
	// StatusLink opts the monitor in to opening and resolving a status page incident. Zero
	// confirm and recovery values fall back to the APP_MONITOR_LINK_* settings.
	StatusLink          bool `json:"status_link,omitempty"`
	LinkConfirmFailures int  `json:"link_confirm_failures,omitempty"`
	LinkRecoverySeconds int  `json:"link_recovery_seconds,omitempty"`
	// GraceSeconds is the heartbeat grace period; zero uses APP_MONITOR_HEARTBEAT_GRACE.
	GraceSeconds int `json:"grace_seconds,omitempty"`
	// WarnDays and CritDays override the APP_MONITOR_TLS_* thresholds for one TLS monitor.
	WarnDays int `json:"warn_days,omitempty"`
	CritDays int `json:"crit_days,omitempty"`
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
	// PingToken is the plaintext heartbeat token, set only on the Monitor returned by Create
	// and RotateToken. It is never stored: only its hash is.
	PingToken string
}

// Result is one probe outcome. Reason is a short class, never a body or header value.
type Result struct {
	MonitorID string
	At        int64
	OK        bool
	LatencyMS int
	Code      int
	Reason    string
	// Degraded marks a passing check that is close to failing, such as a certificate
	// inside its warning window. It is not stored, the Reason is.
	Degraded bool
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
	switch m.Kind {
	case KindHeartbeat:
		if m.Timeout <= 0 {
			m.Timeout = time.Second
		}
	case KindTLS:
		if m.Timeout <= 0 {
			m.Timeout = DefaultTLSTimeout
		}
		if m.Spec.Port == 0 {
			m.Spec.Port = 443
		}
	}
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
		return bad("kind must be http, tcp, heartbeat or tls")
	}
	if m.Interval < floor {
		return bad("interval %s is below the minimum %s (%s)", m.Interval, floor, envName)
	}
	if m.Spec.LinkConfirmFailures < 0 || m.Spec.LinkRecoverySeconds < 0 {
		return bad("status link confirm failures and recovery seconds cannot be negative")
	}
	if m.Kind == KindHeartbeat {
		return validateHeartbeat(m.Spec, cfg, bad)
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
	switch m.Kind {
	case KindTCP:
		return validateTCP(m.Spec, bad)
	case KindTLS:
		return validateTLS(m.Spec, bad)
	}
	return validateHTTP(m.Spec, cfg, bad)
}

func validateHeartbeat(s Spec, cfg Config, bad func(string, ...any) error) error {
	if s.GraceSeconds < 0 || time.Duration(s.GraceSeconds)*time.Second > cfg.HeartbeatMaxGrace {
		return bad("grace must be between 0 and %s (APP_MONITOR_HEARTBEAT_MAX_GRACE)", cfg.HeartbeatMaxGrace)
	}
	return nil
}

func validateTLS(s Spec, bad func(string, ...any) error) error {
	if err := validateTCP(s, bad); err != nil {
		return err
	}
	if s.WarnDays < 0 || s.CritDays < 0 || (s.WarnDays > 0 && s.CritDays > 0 && s.CritDays >= s.WarnDays) {
		return bad("critical days must be below warning days, and neither negative")
	}
	return nil
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
