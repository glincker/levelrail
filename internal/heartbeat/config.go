// Package heartbeat sends an outbound dead-man ping so an external monitor
// alerts when the control plane stops, which no in-box alert can do.
package heartbeat

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/GLINCKER/levelrail/kit/netguard"
)

// Environment variables that configure and lock the heartbeat settings.
const (
	EnvURL          = "APP_HEARTBEAT_URL"
	EnvFailURL      = "APP_HEARTBEAT_FAIL_URL"
	EnvInterval     = "APP_HEARTBEAT_INTERVAL"
	EnvTimeout      = "APP_HEARTBEAT_TIMEOUT"
	EnvBody         = "APP_HEARTBEAT_BODY"
	EnvMethod       = "APP_HEARTBEAT_METHOD"
	EnvAllowPrivate = "APP_HEARTBEAT_ALLOW_PRIVATE"
	EnvReconcileMul = "APP_HEARTBEAT_RECONCILE_WINDOWS"

	defaultInterval     = 60 * time.Second
	defaultTimeout      = 10 * time.Second
	defaultReconcileMul = 3
	minInterval         = 5 * time.Second
)

// Config is the env-derived heartbeat configuration.
type Config struct {
	URL          string
	FailURL      string
	Interval     time.Duration
	Timeout      time.Duration
	Method       string
	BodyOff      bool
	AllowPrivate bool
	// ReconcileWindows is how many reconcile intervals may pass without a pass before the loop counts as stalled.
	ReconcileWindows int
}

// ConfigFromEnv reads and validates the heartbeat env; an invalid URL is a boot error.
func ConfigFromEnv(lookup func(string) (string, bool)) (Config, error) {
	get := func(k string) string {
		v, _ := lookup(k)
		return strings.TrimSpace(v)
	}
	c := Config{
		URL:              get(EnvURL),
		FailURL:          get(EnvFailURL),
		Interval:         defaultInterval,
		Timeout:          defaultTimeout,
		Method:           http.MethodPost,
		BodyOff:          strings.EqualFold(get(EnvBody), "off"),
		AllowPrivate:     isTruthy(get(EnvAllowPrivate)),
		ReconcileWindows: defaultReconcileMul,
	}
	var err error
	if v := get(EnvInterval); v != "" {
		if c.Interval, err = time.ParseDuration(v); err != nil || c.Interval < minInterval {
			return Config{}, fmt.Errorf("%s must be a duration of at least %s", EnvInterval, minInterval)
		}
	}
	if v := get(EnvTimeout); v != "" {
		if c.Timeout, err = time.ParseDuration(v); err != nil || c.Timeout <= 0 {
			return Config{}, fmt.Errorf("%s must be a positive duration", EnvTimeout)
		}
	}
	if v := get(EnvReconcileMul); v != "" {
		if c.ReconcileWindows, err = strconv.Atoi(v); err != nil || c.ReconcileWindows < 1 {
			return Config{}, fmt.Errorf("%s must be a positive integer", EnvReconcileMul)
		}
	}
	if v := strings.ToUpper(get(EnvMethod)); v != "" {
		if v != http.MethodGet && v != http.MethodPost {
			return Config{}, fmt.Errorf("%s must be GET or POST", EnvMethod)
		}
		c.Method = v
	}
	if c.URL != "" {
		if _, err := ValidateURL(c.URL, c.AllowPrivate); err != nil {
			return Config{}, fmt.Errorf("%s: %w", EnvURL, err)
		}
	}
	if c.FailURL != "" {
		if _, err := ValidateURL(c.FailURL, c.AllowPrivate); err != nil {
			return Config{}, fmt.Errorf("%s: %w", EnvFailURL, err)
		}
	}
	return c, nil
}

func isTruthy(v string) bool {
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// ValidateURL accepts an absolute http or https URL. Errors never echo the URL because it holds a secret key.
func ValidateURL(raw string, allowPrivate bool) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return nil, errors.New("must be an absolute http:// or https:// URL")
	}
	if u.User != nil {
		return nil, errors.New("must not contain credentials before the host")
	}
	if ip, perr := netip.ParseAddr(u.Hostname()); perr == nil && !allowPrivate && blockedAddr(ip) {
		return nil, fmt.Errorf("host is a private address; set %s=1 to allow it", EnvAllowPrivate)
	}
	return u, nil
}

// HostOf returns only the host of raw, the one part of a heartbeat URL safe to show or log.
func HostOf(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return u.Host
}

func blockedAddr(ip netip.Addr) bool {
	return netguard.IsBlocked(ip)
}

// guardedDialer refuses private destinations at connect time so a DNS name cannot smuggle one in.
func guardedDialer(allowPrivate bool, timeout time.Duration) *net.Dialer {
	d := &net.Dialer{Timeout: timeout}
	if allowPrivate {
		return d
	}
	d.Control = func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		ip, err := netip.ParseAddr(host)
		if err != nil {
			return err
		}
		if blockedAddr(ip) {
			return errors.New("destination is a private address")
		}
		return nil
	}
	return d
}
