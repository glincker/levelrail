package main

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerBackupVerificationTools(server *mcp.Server, client *apiclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_backup_verifications",
		Description: "List a database backup's verification attempt history, newest first: checksum match, size match, format validity for each attempt. Read-only; does not trigger a new verification.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in backupVerificationsInput) (*mcp.CallToolResult, []apiclient.BackupVerificationResource, error) {
		verifications, err := client.ListBackupVerifications(ctx, in.Name, in.BackupID)
		if err != nil {
			return nil, nil, fmt.Errorf("list verifications for backup %q of database %q: %w", in.BackupID, in.Name, err)
		}
		return nil, verifications, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_latest_backup_verification",
		Description: "Get the most recent verification attempt for a database backup, or an empty result if none has ever been run. Read-only; does not trigger a new verification.",
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

type backupVerificationsInput struct {
	Name     string `json:"name" jsonschema:"the database's name"`
	BackupID string `json:"backup_id" jsonschema:"id of the backup to show verification history for"`
}
