package api

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	deploymentsDefaultLimit = 50
	deploymentsMaxLimit     = 200
	deploymentsMaxWindow    = 30 * 24 * time.Hour
)

var (
	deploymentStatusValues = []string{
		store.DeploymentQueued, store.DeploymentHeld, store.DeploymentAwaiting, store.DeploymentBuilding,
		store.DeploymentReady, store.DeploymentFailed, store.DeploymentCanceled,
		store.DeploymentRolledBack, store.DeploymentSuperseded,
	}
	deploymentTriggerValues = []string{
		store.DeploymentTriggerGitPush, store.DeploymentTriggerManual, store.DeploymentTriggerRollback,
		store.DeploymentTriggerAPI, store.DeploymentTriggerPreview, "schedule", "pipeline",
	}
)

// splitMulti flattens repeated and comma-separated query values.
func splitMulti(vals []string) []string {
	var out []string
	for _, raw := range vals {
		for _, p := range strings.Split(raw, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

func validateEnum(field string, vals, allowed []string) error {
	for _, v := range vals {
		ok := false
		for _, a := range allowed {
			if v == a {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("%s %q is not one of %s", field, v, strings.Join(allowed, ", "))
		}
	}
	return nil
}

// parseWindowTime accepts an RFC3339 timestamp or a duration such as 24h or
// 7d meaning that long before now.
func parseWindowTime(raw string, now time.Time) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, nil
	}
	d, err := parseWindowDuration(raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("must be an RFC3339 time or a duration such as 24h or 7d")
	}
	return now.Add(-d), nil
}

func parseWindowDuration(raw string) (time.Duration, error) {
	if days, ok := strings.CutSuffix(raw, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("invalid duration %q", raw)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("invalid duration %q", raw)
	}
	return d, nil
}

// parseDeploymentQuery turns list query parameters into a store filter.
func parseDeploymentQuery(q url.Values, now time.Time) (store.DeploymentFilter, error) {
	f := store.DeploymentFilter{
		Statuses:    splitMulti(q["status"]),
		Triggers:    splitMulti(q["trigger"]),
		App:         q.Get("app"),
		Branch:      q.Get("branch"),
		Environment: q.Get("environment"),
		Query:       q.Get("q"),
		Cursor:      q.Get("cursor"),
		Limit:       deploymentsDefaultLimit,
	}
	if err := validateEnum("status", f.Statuses, deploymentStatusValues); err != nil {
		return f, err
	}
	if err := validateEnum("trigger", f.Triggers, deploymentTriggerValues); err != nil {
		return f, err
	}
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			return f, fmt.Errorf("limit must be a positive integer")
		}
		f.Limit = min(n, deploymentsMaxLimit)
	}
	var err error
	if raw := q.Get("since"); raw != "" {
		if f.Since, err = parseWindowTime(raw, now); err != nil {
			return f, fmt.Errorf("since %w", err)
		}
	}
	if raw := q.Get("until"); raw != "" {
		if f.Until, err = parseWindowTime(raw, now); err != nil {
			return f, fmt.Errorf("until %w", err)
		}
	}
	if raw := q.Get("live"); raw != "" {
		if f.Live, err = strconv.ParseBool(raw); err != nil {
			return f, fmt.Errorf("live must be true or false")
		}
	}
	if raw := q.Get("pr"); raw != "" {
		if f.PR, err = strconv.Atoi(raw); err != nil || f.PR < 1 {
			return f, fmt.Errorf("pr must be a positive integer")
		}
	}
	return f, nil
}
