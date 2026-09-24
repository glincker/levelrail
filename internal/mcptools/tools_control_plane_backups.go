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
}
