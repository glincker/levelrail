package alerting

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// RegistryRefreshHealth is one cloud registry credential's refresh state.
type RegistryRefreshHealth struct {
	Name      string
	Preset    string
	LastError string
	ExpiresAt *time.Time
}

// RegistryRefreshSource lists preset registry credentials and their refresh state.
type RegistryRefreshSource interface {
	RegistryRefreshHealth(ctx context.Context) ([]RegistryRefreshHealth, error)
}

// RegistryRefreshFixPath is the settings page whose Refresh now button fixes the alert.
const RegistryRefreshFixPath = "/settings/registry-credentials"

// EvaluateRegistryRefreshFailing runs one KindRegistryRefreshFailing rule.
// A credential is failing while its last refresh errored.
func EvaluateRegistryRefreshFailing(ctx context.Context, source RegistryRefreshSource, r Rule, now time.Time) (Rule, string, error) {
	creds, err := source.RegistryRefreshHealth(ctx)
	if err != nil {
		return r, "", fmt.Errorf("alerting: evaluate rule %q: read registry refresh health: %w", r.ID, err)
	}
	var bad []string
	for _, c := range creds {
		if c.LastError == "" {
			continue
		}
		line := fmt.Sprintf("%s (%s): %s", c.Name, c.Preset, truncateNotice(c.LastError, 160))
		if c.ExpiresAt != nil {
			line += ", current login expires " + c.ExpiresAt.UTC().Format(time.RFC3339)
		}
		bad = append(bad, line)
	}
	next := r
	next.LastEvaluatedAt = &now
	v := float64(len(bad))
	next.LastValue = &v
	var notice string
	if len(bad) > 0 {
		notice = "registry credential refresh failing: " + strings.Join(bad, "; ") + ". Fix: open " + RegistryRefreshFixPath + " and press Refresh now"
	}
	return advanceState(next, r, len(bad) > 0, 0, now), notice, nil
}
