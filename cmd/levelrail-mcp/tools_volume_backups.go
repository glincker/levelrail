package main

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerVolumeBackupTools(server *mcp.Server, client *apiclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_app_volume_backups",
		Description: "List an app service volume's backup history, newest first: status, size, target, and error for each attempt. The volume counterpart of a database's own backup history. Read-only; does not trigger a new backup.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in volumeBackupsInput) (*mcp.CallToolResult, []apiclient.BackupHistoryResource, error) {
		history, err := client.ListVolumeBackups(ctx, in.Name, in.Volume, apiclient.ListBackupsOptions{
			Limit:  in.Limit,
			Before: in.Before,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("list volume backups for app %q volume %q: %w", in.Name, in.Volume, err)
		}
		return nil, history, nil
	})
}

type volumeBackupsInput struct {
	Name   string `json:"name" jsonschema:"the app's name"`
	Volume string `json:"volume" jsonschema:"the service volume's name"`
	Limit  int    `json:"limit,omitempty" jsonschema:"max rows to return, default the server's own default"`
	Before string `json:"before,omitempty" jsonschema:"only return backups started before this RFC3339 timestamp, for paging backward"`
}
