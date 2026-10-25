package datamigrate

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/GLINCKER/levelrail/internal/reconcile/database"
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
	Restore []PreflightCheck `json:"restore,omitempty"`
	// MissingRoles are the policy roles absent from the target at preflight time.
	MissingRoles []string     `json:"missing_roles,omitempty"`
	Verification Verification `json:"verification"`
	Redacted     string       `json:"source"`
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
	target := c.targetState(ctx, plan.TargetName)
	res := HostedPreflight(HostedPreflightInput{Plan: plan, Source: hs, Probe: probe, Target: target, Config: cfg, DiskMargin: diskMargin, MBPerSecond: mbps})
	out := HostedResult{Preflight: res, Probe: probe, Redacted: hs.Redacted}
	if target.Known {
		out.MissingRoles = MissingRoles(probe.PolicyRoles, target.Roles)
	}
	return hs, out, nil
}

// targetState reads the managed database's roles and table count. A failure
// leaves Known false so the role check blocks instead of guessing.
func (c *Copier) targetState(ctx context.Context, name string) TargetState {
	out, err := execCapture(ctx, c.Runtime, database.ContainerName(name), TargetStateCommand())
	if err != nil {
		c.logger().Warn("datamigrate: read target state failed", slog.String("database", name), slog.String("error", Redact(err.Error())))
		return TargetState{}
	}
	return ParseTargetState(out)
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

	var created []PreflightCheck
	if plan.CreateMissingRoles && len(res.MissingRoles) > 0 {
		if created, err = c.createRoles(ctx, plan.TargetName, res.MissingRoles); err != nil {
			return res, errors.New(hs.Scrub(err.Error()))
		}
	}

	filter := &dumpFilter{}
	cp := *c
	cp.Restorer = filteringRestorer{inner: c.Restorer, filter: filter}
	target := Target{Name: plan.TargetName, Engine: EnginePostgres, Version: res.Preflight.TargetVersion}
	v, err := cp.Copy(ctx, target, hs.Source)
	res.Verification = v
	res.Restore = append(created, filter.Findings()...)
	res.Verification.Findings = res.Restore
	if err != nil {
		c.logger().Debug("datamigrate: hosted move failed", slog.String("error", hs.Scrub(err.Error())))
		return res, errors.New(hs.Scrub(tidyExecError(err).Error()))
	}
	if out, oerr := execCapture(ctx, c.Runtime, database.ContainerName(plan.TargetName), ObjectCountsCommand()); oerr == nil {
		res.Verification.Objects = ParseObjectCounts(out)
	}
	return res, nil
}

// createRoles makes the missing policy roles on the target and reports it.
func (c *Copier) createRoles(ctx context.Context, name string, roles []string) ([]PreflightCheck, error) {
	cmd, err := CreateRolesCommand(roles)
	if err != nil {
		return nil, err
	}
	if _, err := execCapture(ctx, c.Runtime, database.ContainerName(name), cmd); err != nil {
		return nil, fmt.Errorf("create the missing roles on the target: %w", tidyExecError(err))
	}
	return []PreflightCheck{{ID: "roles_created", Severity: SeverityWarn,
		Message:    "Created role(s) on the target as NOLOGIN NOINHERIT: " + strings.Join(roles, ", ") + ".",
		NextAction: "Grant them what the app needs; they have no password."}}, nil
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
