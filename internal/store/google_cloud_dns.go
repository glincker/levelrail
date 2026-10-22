package store

import (
	"context"
	"fmt"
)

// GoogleCloudDNSSecretsKey is the internal/secrets serviceName the Cloud DNS
// service account JSON is stored under.
func GoogleCloudDNSSecretsKey() string {
	return "google-cloud-dns"
}

// GoogleCloudDNSKeyEnvKey is the envKey of the service account JSON.
const GoogleCloudDNSKeyEnvKey = "service_account_json"

// GoogleCloudDNSSettings is the single platform-wide row. ProjectID and
// ClientEmail are copied from the key for display; the key is never stored here.
type GoogleCloudDNSSettings struct {
	Enabled     bool
	ProjectID   string
	ClientEmail string
}

// GetGoogleCloudDNSSettings returns the single google_cloud_dns_settings row.
func (db *DB) GetGoogleCloudDNSSettings(ctx context.Context) (GoogleCloudDNSSettings, error) {
	var (
		enabled         int
		project, client string
	)
	err := db.QueryRowContext(ctx, `
		SELECT enabled, project_id, client_email FROM google_cloud_dns_settings WHERE id = 1
	`).Scan(&enabled, &project, &client)
	if err != nil {
		return GoogleCloudDNSSettings{}, fmt.Errorf("store: get google cloud dns settings: %w", err)
	}
	return GoogleCloudDNSSettings{Enabled: enabled != 0, ProjectID: project, ClientEmail: client}, nil
}

// UpdateGoogleCloudDNSSettings replaces the single row's fields.
func (db *DB) UpdateGoogleCloudDNSSettings(ctx context.Context, s GoogleCloudDNSSettings) error {
	enabled := 0
	if s.Enabled {
		enabled = 1
	}
	_, err := db.ExecContext(ctx, `
		UPDATE google_cloud_dns_settings SET enabled = ?, project_id = ?, client_email = ? WHERE id = 1
	`, enabled, s.ProjectID, s.ClientEmail)
	if err != nil {
		return fmt.Errorf("store: update google cloud dns settings: %w", err)
	}
	return nil
}
