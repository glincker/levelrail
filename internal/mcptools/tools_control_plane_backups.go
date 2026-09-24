package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type controlPlaneBackupsOutput struct {
	Backups []apiclient.ControlPlaneBackup `json:"backups"`
}

func registerControlPlaneBackupTools(server *mcp.Server, client *apiclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_control_plane_backups",
		Description: "List snapshots of the control plane's own database: name, size, creation time, sha256. Read-only; not app database backups.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, controlPlaneBackupsOutput, error) {
		backups, err := client.ListControlPlaneBackups(ctx)
		if err != nil {
			return nil, controlPlaneBackupsOutput{}, fmt.Errorf("list control plane backups: %w", err)
		}
		return nil, controlPlaneBackupsOutput{Backups: backups}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_control_plane_backup",
		Description: "Take a snapshot of the control plane's own database now and return its name, size and sha256. Mutating (writes a backup file, changes no app or config).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, apiclient.ControlPlaneBackup, error) {
		backup, err := client.CreateControlPlaneBackup(ctx)
		if err != nil {
			return nil, apiclient.ControlPlaneBackup{}, fmt.Errorf("create control plane backup: %w", err)
		}
		return nil, backup, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "verify_control_plane_backup",
		Description: "Verify a control plane database snapshot by name (see list_control_plane_backups): runs integrity checks and returns ok plus each check's name, result and detail. A failed check is a normal result with ok false. Mutating only in that it records the verification time on the backup; restores nothing.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in controlPlaneBackupNameInput) (*mcp.CallToolResult, apiclient.ControlPlaneBackupVerification, error) {
		res, err := client.VerifyControlPlaneBackup(ctx, in.Name)
		if err != nil {
			return nil, apiclient.ControlPlaneBackupVerification{}, fmt.Errorf("verify control plane backup %q: %w", in.Name, err)
		}
		return nil, res, nil
	})
}

type controlPlaneBackupNameInput struct {
	Name string `json:"name" jsonschema:"snapshot name as returned by list_control_plane_backups"`
}
