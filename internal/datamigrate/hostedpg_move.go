package datamigrate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

// ErrWritesNotFrozen is returned when a move is attempted before the caller
// acknowledged that the source stopped taking writes.
var ErrWritesNotFrozen = errors.New("writes to the source are not frozen: put the old app in maintenance mode and set writes_frozen before moving")

// ErrPreflightBlocked wraps the blocking findings of a hosted preflight.
var ErrPreflightBlocked = errors.New("hosted database preflight is blocked")

// HostedResult is the outcome of PreflightHosted or MoveHosted.
type HostedResult struct {
	Preflight PreflightResult `json:"preflight"`
	Probe     HostedProbe     `json:"probe"`
	// Restore holds what the restore changed to tolerate provider objects.
	Restore      []PreflightCheck `json:"restore,omitempty"`
	Verification Verification     `json:"verification"`
	Redacted     string           `json:"source"`
}

// PreflightHosted validates the URL, probes the database with read-only
// queries and returns the findings. Nothing is dumped or written.
func (c *Copier) PreflightHosted(ctx context.Context, plan HostedPlan, cfg HostedConfig, diskMargin, mbps float64) (HostedSource, HostedResult, error) {
	hs, err := ParseHostedURL(string(plan.URL), cfg)
	if err != nil {
		return HostedSource{}, HostedResult{}, err
	}
	insp := &Inspector{Runtime: c.Runtime, Logger: c.Logger, HelperNetwork: c.HelperNetwork}
	probe, err := insp.ProbeHosted(ctx, hs)
	if err != nil {
		return hs, HostedResult{Redacted: hs.Redacted}, err
	}
	res := HostedPreflight(HostedPreflightInput{Plan: plan, Source: hs, Probe: probe, Config: cfg, DiskMargin: diskMargin, MBPerSecond: mbps})
	return hs, HostedResult{Preflight: res, Probe: probe, Redacted: hs.Redacted}, nil
}

// MoveHosted copies a hosted Postgres database into a managed one: refuse
// without writes_frozen, preflight, dump with --no-owner --no-privileges,
// restore through the filtering restorer and compare per-table row counts.
// The helper image tag is the target major, which preflight keeps at or above
// the source's. Errors are scrubbed of the URL and password.
func (c *Copier) MoveHosted(ctx context.Context, plan HostedPlan, cfg HostedConfig, diskMargin, mbps float64) (HostedResult, error) {
	if !plan.WritesFrozen {
		return HostedResult{}, ErrWritesNotFrozen
	}
	hs, res, err := c.PreflightHosted(ctx, plan, cfg, diskMargin, mbps)
	if err != nil {
		return res, err
	}
	if res.Preflight.Blocked {
		return res, fmt.Errorf("%w: %s", ErrPreflightBlocked, blockedSummary(res.Preflight))
	}

	filter := &dumpFilter{}
	cp := *c
	cp.Restorer = filteringRestorer{inner: c.Restorer, filter: filter}
	target := Target{Name: plan.TargetName, Engine: EnginePostgres, Version: res.Preflight.TargetVersion}
	v, err := cp.Copy(ctx, target, hs.Source)
	res.Verification = v
	res.Restore = filter.Findings()
	if err != nil {
		return res, errors.New(hs.Scrub(err.Error()))
	}
	return res, nil
}

func blockedSummary(p PreflightResult) string {
	var parts []string
	for _, ch := range p.Checks {
		if ch.Severity == SeverityBlock {
			parts = append(parts, ch.ID+": "+ch.Message)
		}
	}
	return strings.Join(parts, " ")
}

// HostedConfigFromProcessEnv is HostedConfigFromEnv over the process environment.
func HostedConfigFromProcessEnv() (HostedConfig, error) { return HostedConfigFromEnv(os.Getenv) }
