package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerBackupVerificationTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "list_backup_verifications",
		Description: "List a database backup's verification attempt history, newest first: checksum match, size match, format validity for each attempt.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in backupVerificationsInput) (*mcp.CallToolResult, []apiclient.BackupVerificationResource, error) {
		verifications, err := client.ListBackupVerifications(ctx, in.Name, in.BackupID)
		if err != nil {
			return nil, nil, fmt.Errorf("list verifications for backup %q of database %q: %w", in.BackupID, in.Name, err)
		}
		return nil, verifications, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_latest_backup_verification",
		Description: "Get the most recent verification attempt for a database backup, or an empty result if none has ever been run.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in backupVerificationsInput) (*mcp.CallToolResult, apiclient.BackupVerificationResource, error) {
		verifications, err := client.ListBackupVerifications(ctx, in.Name, in.BackupID)
		if err != nil {
			return nil, apiclient.BackupVerificationResource{}, fmt.Errorf("get latest verification for backup %q of database %q: %w", in.BackupID, in.Name, err)
		}
		if len(verifications) == 0 {
			return nil, apiclient.BackupVerificationResource{}, nil
		}
		return nil, verifications[0], nil
	})
}

type databasePITRInput struct {
	Name string `json:"name" jsonschema:"database name"`
}

// databasePITROutput is a slim view of the status so the tool stays cheap to list.
type databasePITROutput struct {
	Enabled     bool   `json:"enabled"`
	RestoreMode string `json:"restore_mode,omitempty"`
	WindowStart string `json:"window_start,omitempty"`
	WindowEnd   string `json:"window_end,omitempty"`
	WindowError string `json:"window_error,omitempty"`
}

func registerDatabasePITRTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "get_database_pitr_status",
		Description: "Get a database's point-in-time restore state: enabled, recoverable window, and whether restore is in place or into a new database.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in databasePITRInput) (*mcp.CallToolResult, databasePITROutput, error) {
		status, err := client.GetPITRStatus(ctx, in.Name)
		if err != nil {
			return nil, databasePITROutput{}, fmt.Errorf("get point-in-time restore status for database %q: %w", in.Name, err)
		}
		return nil, databasePITROutput{
			Enabled: status.Enabled, RestoreMode: status.RestoreMode,
			WindowStart: status.WindowStart, WindowEnd: status.WindowEnd, WindowError: status.WindowError,
		}, nil
	})
}

type backupVerificationsInput struct {
	Name     string `json:"name" jsonschema:"the database's name"`
	BackupID string `json:"backup_id" jsonschema:"id of the backup to show verification history for"`
}
