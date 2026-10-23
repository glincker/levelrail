// Package splittest holds the pure rules of app split testing: variant
// validation, sticky assignment and the env defaults. Ingress and the API
// both import it, so it stays free of store and HTTP types.
package splittest

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"regexp"
	"strings"
	"time"
)

// Env vars that tune the defaults a test can override per cookie.
const (
	EnvCookieName    = "APP_SPLIT_COOKIE_NAME"
	EnvCookieTTL     = "APP_SPLIT_COOKIE_TTL"
	EnvOverrideParam = "APP_SPLIT_OVERRIDE_PARAM"
	EnvFlushInterval = "APP_SPLIT_FLUSH_INTERVAL"
)

const (
	defaultCookieName    = "lr_split"
	defaultCookieTTL     = 30 * 24 * time.Hour
	defaultOverrideParam = "lr_variant"
	defaultFlushInterval = 30 * time.Second

	// MaxVariants bounds a test so the variants table stays readable.
	MaxVariants = 5
	// TotalWeight is what the variant weights must add up to.
	TotalWeight = 100
)

// Variant kinds. Production is the app itself; the others are other deploys.
const (
	KindProduction = "production"
	KindBranch     = "branch"
	KindRelease    = "release"
)

// Test statuses.
const (
	StatusRunning = "running"
	StatusStopped = "stopped"
)

// Variant is one arm of a split test. Ref is a branch name for a branch
// variant and a deploy URL service name for a release variant.
type Variant struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Kind   string `json:"kind"`
	Ref    string `json:"ref,omitempty"`
	Weight int    `json:"weight"`
}

var cookieNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// Defaults are the process-wide fallbacks for a test's cookie settings.
type Defaults struct {
	CookieName    string
	CookieTTL     time.Duration
	OverrideParam string
	FlushInterval time.Duration
}

// DefaultsFromEnv reads the APP_SPLIT_* env vars over the built-in defaults.
func DefaultsFromEnv(getenv func(string) string) Defaults {
	d := Defaults{CookieName: defaultCookieName, CookieTTL: defaultCookieTTL, OverrideParam: defaultOverrideParam, FlushInterval: defaultFlushInterval}
	if v := strings.TrimSpace(getenv(EnvCookieName)); cookieNameRe.MatchString(v) {
		d.CookieName = v
	}
	if v := strings.TrimSpace(getenv(EnvOverrideParam)); cookieNameRe.MatchString(v) {
		d.OverrideParam = v
	}
	if dur, err := time.ParseDuration(strings.TrimSpace(getenv(EnvCookieTTL))); err == nil && dur >= time.Minute {
		d.CookieTTL = dur
	}
	if dur, err := time.ParseDuration(strings.TrimSpace(getenv(EnvFlushInterval))); err == nil && dur >= time.Second {
		d.FlushInterval = dur
	}
	return d
}

// OSEnv reads the process environment.
func OSEnv(k string) string { return os.Getenv(k) }

// ValidCookieName reports whether n is safe as a cookie or query parameter name.
func ValidCookieName(n string) bool { return cookieNameRe.MatchString(n) }

// Validate checks a variant set: 2 to MaxVariants arms, exactly one
// production arm, unique labels, non-negative weights summing to TotalWeight.
func Validate(vs []Variant) error {
	if len(vs) < 2 || len(vs) > MaxVariants {
		return fmt.Errorf("a split test needs between 2 and %d variants", MaxVariants)
	}
	sum, prod := 0, 0
	labels := make(map[string]bool, len(vs))
	ids := make(map[string]bool, len(vs))
	for _, v := range vs {
		if strings.TrimSpace(v.Label) == "" {
			return errors.New("every variant needs a label")
		}
		key := strings.ToLower(v.Label)
		if labels[key] || ids[v.ID] {
			return fmt.Errorf("variant %q is listed twice", v.Label)
		}
		labels[key], ids[v.ID] = true, true
		switch v.Kind {
		case KindProduction:
			prod++
		case KindBranch, KindRelease:
			if strings.TrimSpace(v.Ref) == "" {
				return fmt.Errorf("variant %q needs a %s to send visitors to", v.Label, v.Kind)
			}
		default:
			return fmt.Errorf("variant %q has unknown kind %q", v.Label, v.Kind)
		}
		if v.Weight < 0 || v.Weight > TotalWeight {
			return fmt.Errorf("variant %q weight must be between 0 and %d", v.Label, TotalWeight)
		}
		sum += v.Weight
	}
	if prod != 1 {
		return errors.New("exactly one variant must be the production deploy")
	}
	if sum != TotalWeight {
		return fmt.Errorf("weights add up to %d, they must add up to %d", sum, TotalWeight)
	}
	return nil
}

// Find returns the variant whose ID or label (case-insensitive) matches key.
func Find(vs []Variant, key string) (Variant, bool) {
	key = strings.TrimSpace(key)
	if key == "" {
		return Variant{}, false
	}
	for _, v := range vs {
		if v.ID == key || strings.EqualFold(v.Label, key) {
			return v, true
		}
	}
	return Variant{}, false
}

// Draw picks a variant by weight with rng.
func Draw(vs []Variant, rng *rand.Rand) Variant { //nolint:forbidigo // traffic split draw, not a secret
	n := rng.IntN(TotalWeight) //nolint:forbidigo // traffic split draw, not a secret
	for _, v := range vs {
		if n < v.Weight {
			return v
		}
		n -= v.Weight
	}
	return vs[0]
}

// Assignment is the outcome of Decide.
type Assignment struct {
	Variant Variant
	// SetCookie is true when the visitor's cookie must be written: a fresh
	// draw or a QA override. A kept assignment leaves the cookie alone.
	SetCookie bool
}

// Decide picks the variant for one visitor. A forced key (the QA override)
// wins, then a still-valid cookie value, then a fresh weighted draw. A cookie
// naming a removed or zero-weight variant is treated as absent.
func Decide(vs []Variant, forced, cookie string, rng *rand.Rand) Assignment { //nolint:forbidigo // traffic split draw, not a secret
	if v, ok := Find(vs, forced); ok {
		return Assignment{Variant: v, SetCookie: v.ID != cookie}
	}
	if v, ok := Find(vs, cookie); ok && v.Weight > 0 && v.ID == cookie {
		return Assignment{Variant: v}
	}
	return Assignment{Variant: Draw(vs, rng), SetCookie: true}
}
