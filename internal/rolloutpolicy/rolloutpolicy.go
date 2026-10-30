// Package rolloutpolicy resolves a rolling deploy's max_surge,
// max_unavailable and min_available into replica counts, and tracks the
// live progress of each rollout for the API.
package rolloutpolicy

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Env names for the platform defaults an app's rollout: block overrides.
const (
	EnvMaxSurge       = "APP_ROLLOUT_MAX_SURGE"
	EnvMaxUnavailable = "APP_ROLLOUT_MAX_UNAVAILABLE"
	EnvMinAvailable   = "APP_ROLLOUT_MIN_AVAILABLE"
)

// Fallbacks used only when the env is unset: they keep the one replica at
// a time rolling pass that existed before these knobs did.
const (
	fallbackMaxSurge       = "1"
	fallbackMaxUnavailable = "0"
)

// Env reads a variable; OSEnv is the process environment.
type Env func(string) string

// OSEnv reads the real process environment.
var OSEnv Env = os.Getenv

// Policy is an app's rollout settings as written: an int ("2") or a
// percent of replicas ("25%"). Empty inherits the env default.
type Policy struct {
	MaxSurge       string `json:"max_surge,omitempty"`
	MaxUnavailable string `json:"max_unavailable,omitempty"`
	MinAvailable   string `json:"min_available,omitempty"`
}

// Resolved is a Policy turned into replica counts for one replica total.
type Resolved struct {
	MaxSurge       int `json:"max_surge"`
	MaxUnavailable int `json:"max_unavailable"`
	MinAvailable   int `json:"min_available"`
}

// Step is how many replicas one wave replaces.
func (r Resolved) Step() int {
	return max(1, r.MaxSurge+r.MaxUnavailable)
}

// Waves is how many waves a full rollout of replicas takes.
func (r Resolved) Waves(replicas int) int {
	if replicas <= 0 {
		return 0
	}
	step := r.Step()
	return (replicas + step - 1) / step
}

// ParseValue validates an int or percent value without resolving it.
func ParseValue(v string) (n int, percent bool, err error) {
	s := strings.TrimSpace(v)
	if strings.HasSuffix(s, "%") {
		percent = true
		s = strings.TrimSuffix(s, "%")
	}
	n, err = strconv.Atoi(s)
	if err != nil || n < 0 || (percent && n > 100) {
		return 0, false, fmt.Errorf("%q is not a non-negative integer or a percent between 0%% and 100%%", v)
	}
	return n, percent, nil
}

// resolveValue turns v into a count of replicas; percents round up for
// surge and min_available and down for unavailable, so both stay safe.
func resolveValue(v string, replicas int, roundUp bool) (int, error) {
	n, pct, err := ParseValue(v)
	if err != nil {
		return 0, err
	}
	if !pct {
		return n, nil
	}
	num := n * replicas
	if roundUp {
		return (num + 99) / 100, nil
	}
	return num / 100, nil
}

// Validate checks a Policy as written, before any replica count is known.
func (p Policy) Validate() error {
	for _, f := range []struct{ name, v string }{
		{"max_surge", p.MaxSurge}, {"max_unavailable", p.MaxUnavailable}, {"min_available", p.MinAvailable},
	} {
		if f.v == "" {
			continue
		}
		if _, _, err := ParseValue(f.v); err != nil {
			return fmt.Errorf("rollout.%s: %w", f.name, err)
		}
	}
	if isZero(p.MaxSurge) && isZero(p.MaxUnavailable) {
		return fmt.Errorf("rollout: max_surge and max_unavailable cannot both be 0, a rollout could never make progress")
	}
	return nil
}

func isZero(v string) bool {
	n, _, err := ParseValue(v)
	return v != "" && err == nil && n == 0
}

// Resolve fills empty fields from env and converts them for replicas.
// An unusable env value falls back rather than blocking every deploy.
func Resolve(p Policy, env Env, replicas int) Resolved {
	if env == nil {
		env = OSEnv
	}
	pick := func(own, envName, fallback string) string {
		if own != "" {
			return own
		}
		if v := strings.TrimSpace(env(envName)); v != "" {
			if _, _, err := ParseValue(v); err == nil {
				return v
			}
		}
		return fallback
	}
	surge, _ := resolveValue(pick(p.MaxSurge, EnvMaxSurge, fallbackMaxSurge), replicas, true)
	unavail, _ := resolveValue(pick(p.MaxUnavailable, EnvMaxUnavailable, fallbackMaxUnavailable), replicas, false)
	minAvail, _ := resolveValue(pick(p.MinAvailable, EnvMinAvailable, "0"), replicas, true)
	if surge == 0 && unavail == 0 {
		// A percent can round both to zero on small replica counts.
		surge = 1
	}
	return Resolved{MaxSurge: surge, MaxUnavailable: unavail, MinAvailable: minAvail}
}

// DisruptionError is returned when a voluntary operation would breach an
// app's min_available budget.
type DisruptionError struct {
	App          string
	Action       string
	MinAvailable int
	Available    int
	Remaining    int
}

func (e *DisruptionError) Error() string {
	return fmt.Sprintf("%s on app %q would leave %d of %d replicas available, below its min_available of %d: "+
		"scale the app up first, wait for its replicas to be ready, or lower rollout.min_available in app.yaml",
		e.Action, e.App, e.Remaining, e.Available, e.MinAvailable)
}

// CheckDisruption refuses taking removing replicas out of available ones
// when that would drop below minAvailable. Zero minAvailable never blocks.
func CheckDisruption(app, action string, minAvailable, available, removing int) error {
	if minAvailable <= 0 || removing <= 0 {
		return nil
	}
	if remaining := available - removing; remaining < minAvailable {
		return &DisruptionError{App: app, Action: action, MinAvailable: minAvailable, Available: available, Remaining: max(0, remaining)}
	}
	return nil
}

// Progress is one app's rolling deploy as the reconciler last saw it.
type Progress struct {
	Active         bool      `json:"active"`
	Phase          string    `json:"phase"`
	Wave           int       `json:"wave"`
	Waves          int       `json:"waves"`
	Desired        int       `json:"desired"`
	Updated        int       `json:"updated"`
	Ready          int       `json:"ready"`
	Unavailable    int       `json:"unavailable"`
	Surge          int       `json:"surge"`
	MaxSurge       int       `json:"max_surge"`
	MaxUnavailable int       `json:"max_unavailable"`
	Message        string    `json:"message,omitempty"`
	StartedAt      time.Time `json:"started_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Rollout phases.
const (
	PhaseRolling  = "rolling"
	PhaseComplete = "complete"
	PhaseFailed   = "failed"
)

var progress sync.Map // service name -> Progress

// Record stores p for service, keeping the first StartedAt of a rollout.
func Record(service string, p Progress) {
	now := time.Now().UTC()
	p.UpdatedAt = now
	if prev, ok := Lookup(service); ok && prev.Active && !prev.StartedAt.IsZero() {
		p.StartedAt = prev.StartedAt
	}
	if p.StartedAt.IsZero() {
		p.StartedAt = now
	}
	progress.Store(service, p)
}

// Lookup returns service's last recorded rollout, if any.
func Lookup(service string) (Progress, bool) {
	v, ok := progress.Load(service)
	if !ok {
		return Progress{}, false
	}
	p, _ := v.(Progress)
	return p, true
}
