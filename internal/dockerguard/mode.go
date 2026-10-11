// Package dockerguard is an in-process Unix socket proxy in front of the
// Docker Engine API. It forwards only an allowlisted set of endpoints and
// rejects container configurations that would hand a container the host.
// See docs/docker-access.md for the surface and the rules.
package dockerguard

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Mode selects what the guard does with a request a rule would deny.
type Mode string

const (
	// ModeOff starts no proxy: Docker clients dial the daemon directly.
	ModeOff Mode = "off"
	// ModeAudit forwards everything and records what enforce would deny.
	ModeAudit Mode = "audit"
	// ModeEnforce rejects denied requests with 403 before they reach Docker.
	ModeEnforce Mode = "enforce"
)

// DefaultMode is audit so a fresh install observes before it blocks.
const DefaultMode = ModeAudit

// Environment variables the guard reads.
const (
	EnvMode             = "APP_DOCKER_GUARD"
	EnvSocket           = "APP_DOCKER_GUARD_SOCKET"
	EnvMaxBodyBytes     = "APP_DOCKER_GUARD_MAX_BODY_BYTES"
	EnvAuditDedup       = "APP_DOCKER_GUARD_AUDIT_DEDUP"
	EnvSummaryWindow    = "APP_DOCKER_GUARD_SUMMARY_WINDOW"
	EnvAllowHostNetwork = "APP_DOCKER_GUARD_ALLOW_HOST_NETWORK"
	EnvHeaderTimeout    = "APP_DOCKER_GUARD_HEADER_TIMEOUT"
	EnvRecordQueue      = "APP_DOCKER_GUARD_RECORD_QUEUE"
)

// Defaults for the tunables above.
const (
	DefaultMaxBodyBytes  int64 = 4 << 20
	DefaultAuditDedup          = time.Hour
	DefaultSummaryWindow       = 7 * 24 * time.Hour
	DefaultHeaderTimeout       = 30 * time.Second
	DefaultRecordQueue         = 256
	// SocketName is the guard socket's file name under the data dir.
	SocketName = "docker-guard.sock"
)

// ParseMode validates s. Empty means DefaultMode. An unknown value returns
// ModeEnforce with an error: a typo must never silently disable the guard.
func ParseMode(s string) (Mode, error) {
	switch m := Mode(strings.ToLower(strings.TrimSpace(s))); m {
	case "":
		return DefaultMode, nil
	case ModeOff, ModeAudit, ModeEnforce:
		return m, nil
	default:
		return ModeEnforce, fmt.Errorf("dockerguard: %s=%q is not one of off, audit, enforce", EnvMode, s)
	}
}

// Tunables are the guard's operator-adjustable limits.
type Tunables struct {
	MaxBodyBytes     int64
	AuditDedup       time.Duration
	SummaryWindow    time.Duration
	HeaderTimeout    time.Duration
	RecordQueue      int
	AllowHostNetwork bool
}

// TunablesFromEnv reads Tunables, keeping the default for any unset or
// invalid value and returning the first parse error for the caller to log.
func TunablesFromEnv(lookup func(string) (string, bool)) (Tunables, error) {
	if lookup == nil {
		lookup = os.LookupEnv
	}
	t := Tunables{
		MaxBodyBytes:  DefaultMaxBodyBytes,
		AuditDedup:    DefaultAuditDedup,
		SummaryWindow: DefaultSummaryWindow,
		HeaderTimeout: DefaultHeaderTimeout,
		RecordQueue:   DefaultRecordQueue,
	}
	var firstErr error
	keep := func(err error) {
		if firstErr == nil {
			firstErr = err
		}
	}
	if v, ok := envValue(lookup, EnvMaxBodyBytes); ok {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			t.MaxBodyBytes = n
		} else {
			keep(fmt.Errorf("dockerguard: %s=%q is not a positive integer", EnvMaxBodyBytes, v))
		}
	}
	for _, d := range []struct {
		env string
		dst *time.Duration
	}{{EnvAuditDedup, &t.AuditDedup}, {EnvSummaryWindow, &t.SummaryWindow}, {EnvHeaderTimeout, &t.HeaderTimeout}} {
		if v, ok := envValue(lookup, d.env); ok {
			if parsed, err := time.ParseDuration(v); err == nil && parsed > 0 {
				*d.dst = parsed
			} else {
				keep(fmt.Errorf("dockerguard: %s=%q is not a positive duration", d.env, v))
			}
		}
	}
	if v, ok := envValue(lookup, EnvRecordQueue); ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			t.RecordQueue = n
		} else {
			keep(fmt.Errorf("dockerguard: %s=%q is not a positive integer", EnvRecordQueue, v))
		}
	}
	if v, ok := envValue(lookup, EnvAllowHostNetwork); ok {
		b, err := strconv.ParseBool(v)
		if err != nil {
			keep(fmt.Errorf("dockerguard: %s=%q is not a boolean", EnvAllowHostNetwork, v))
		}
		t.AllowHostNetwork = err == nil && b
	}
	return t, firstErr
}

func envValue(lookup func(string) (string, bool), key string) (string, bool) {
	v, ok := lookup(key)
	v = strings.TrimSpace(v)
	return v, ok && v != ""
}
