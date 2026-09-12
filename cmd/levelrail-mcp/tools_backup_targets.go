package main

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerBackupTargetTools(server *mcp.Server, client *apiclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_backup_targets",
		Description: "List every connected S3-compatible backup destination on the control plane: name, provider, endpoint, region, bucket. No credential fields, ever. Read-only; does not create, edit, delete, or test a target.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.BackupTargetResource, error) {
		targets, err := client.ListBackupTargets(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list backup targets: %w", err)
		}
		return nil, targets, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "test_backup_target_connection",
		Description: "Probe a connected backup target's stored credentials against its configured bucket right now, catching a bad or stale credential or a renamed/deleted bucket before the next scheduled backup fails against it silently. Exercises the connection only: never uploads, downloads, or deletes an object, and never changes the target's own configuration.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in backupTargetIDInput) (*mcp.CallToolResult, backupTargetTestResult, error) {
		if err := client.TestBackupTarget(ctx, in.ID); err != nil {
			return nil, backupTargetTestResult{}, fmt.Errorf("test backup target %q: %w", in.ID, err)
		}
		return nil, backupTargetTestResult{OK: true}, nil
	})
}

type backupTargetIDInput struct {
	ID string `json:"id" jsonschema:"the backup target's id"`
}

// backupTargetTestResult is test_backup_target_connection's own success
// shape: the underlying REST endpoint returns 204 No Content, so there is
// no apiclient resource type to reuse here.
type backupTargetTestResult struct {
	OK bool `json:"ok" jsonschema:"true when the connection check succeeded"`
}
