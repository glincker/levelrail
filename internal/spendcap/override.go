package spendcap

import (
	"errors"
	"os"
	"strings"
	"time"
)

// EnvOverrideTTL sets how long a root override of the deploy guard lasts,
// as a Go duration such as "1h".
const EnvOverrideTTL = "APP_SPEND_OVERRIDE_TTL"

const (
	defaultOverrideTTL = time.Hour
	maxOverrideTTL     = 7 * 24 * time.Hour
	// MaxReasonLen bounds the audited reason text.
	MaxReasonLen = 500
)

// Override is one audited, expiring permission to deploy a blocked app.
type Override struct {
	ID        int64     `json:"id"`
	App       string    `json:"app"`
	Period    string    `json:"period"`
	Reason    string    `json:"reason"`
	Actor     string    `json:"actor"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Active reports whether the override still permits deploys at now.
func (o Override) Active(now time.Time) bool { return now.Before(o.ExpiresAt) }

// OverrideTTL reads the override lifetime from the environment.
func OverrideTTL() time.Duration {
	d, err := time.ParseDuration(strings.TrimSpace(os.Getenv(EnvOverrideTTL)))
	if err != nil || d <= 0 || d > maxOverrideTTL {
		return defaultOverrideTTL
	}
	return d
}

// NewOverride validates the reason and stamps the expiry.
func NewOverride(app, reason, actor string, now time.Time, ttl time.Duration) (Override, error) {
	reason = strings.TrimSpace(reason)
	switch {
	case app == "":
		return Override{}, errors.New("app is required")
	case reason == "":
		return Override{}, errors.New("a reason is required for an override")
	case len(reason) > MaxReasonLen:
		return Override{}, errors.New("reason is too long")
	}
	return Override{App: app, Period: Period(now), Reason: reason, Actor: actor, CreatedAt: now.UTC(), ExpiresAt: now.UTC().Add(ttl)}, nil
}
