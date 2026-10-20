package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// DefaultGitHubActionsEnvironment is the GitHub environment name used until an operator sets one.
const DefaultGitHubActionsEnvironment = "production"

// AppGitHubActions is what the GitHub Actions deploy action reports back for one app.
type AppGitHubActions struct {
	ServiceName       string
	Environment       string
	ReportDeployments bool
	ReportChecks      bool
}

// GetAppGitHubActions returns serviceName's settings, or the defaults when none are saved.
func (db *DB) GetAppGitHubActions(ctx context.Context, serviceName string) (AppGitHubActions, error) {
	out := AppGitHubActions{ServiceName: serviceName}
	err := db.QueryRowContext(ctx, `
		SELECT environment, report_deployments, report_checks
		FROM app_github_actions WHERE service_name = ?
	`, serviceName).Scan(&out.Environment, &out.ReportDeployments, &out.ReportChecks)
	if errors.Is(err, sql.ErrNoRows) {
		return AppGitHubActions{
			ServiceName: serviceName, Environment: DefaultGitHubActionsEnvironment,
			ReportDeployments: true, ReportChecks: true,
		}, nil
	}
	if err != nil {
		return AppGitHubActions{}, fmt.Errorf("store: get github actions settings for %q: %w", serviceName, err)
	}
	return out, nil
}

// SetAppGitHubActions saves or replaces the settings for s.ServiceName.
func (db *DB) SetAppGitHubActions(ctx context.Context, s AppGitHubActions) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO app_github_actions (service_name, environment, report_deployments, report_checks, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(service_name) DO UPDATE SET
			environment = excluded.environment,
			report_deployments = excluded.report_deployments,
			report_checks = excluded.report_checks,
			updated_at = excluded.updated_at
	`, s.ServiceName, s.Environment, s.ReportDeployments, s.ReportChecks, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: set github actions settings for %q: %w", s.ServiceName, err)
	}
	return nil
}
