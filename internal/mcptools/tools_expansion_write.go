package mcptools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	defaultExtendHours = 24
	maxExtendHours     = 168
)

type extendPreviewInput struct {
	Name    string `json:"name" jsonschema:"app name"`
	PR      int    `json:"pr_number" jsonschema:"pull request number of the preview"`
	Hours   int    `json:"hours,omitempty" jsonschema:"hours to add, default 24, max 168"`
	Confirm string `json:"confirm" jsonschema:"must equal name"`
}

type runRestoreDrillInput struct {
	BackupID string `json:"backup_id" jsonschema:"backup to restore into a scratch target"`
	Confirm  string `json:"confirm" jsonschema:"must equal backup_id"`
}

type restoreDrillStarted struct {
	DrillID  string `json:"drill_id"`
	BackupID string `json:"backup_id"`
	Status   string `json:"status"`
}

// requireConfirm rejects a write unless confirm echoes the resource name exactly.
func requireConfirm(tool, field, want, got string) error {
	if strings.TrimSpace(want) == "" {
		return fmt.Errorf("%s: %s is required", tool, field)
	}
	if got != want {
		return fmt.Errorf("%s: confirm must equal %s %q to proceed", tool, field, want)
	}
	return nil
}

func registerExpansionWriteTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "extend_preview",
		Description: "Extend a pull-request preview's lifetime. Needs confirm equal to the app name; only exists with --allow-write.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in extendPreviewInput) (*mcp.CallToolResult, apiclient.PreviewEnvironmentResource, error) {
		if err := requireConfirm("extend_preview", "name", in.Name, in.Confirm); err != nil {
			return nil, apiclient.PreviewEnvironmentResource{}, err
		}
		if in.PR <= 0 {
			return nil, apiclient.PreviewEnvironmentResource{}, errors.New("extend_preview: pr_number must be a positive pull request number")
		}
		hours := clampLimit(in.Hours, defaultExtendHours, maxExtendHours)
		p, err := client.ExtendPreviewEnvironment(ctx, in.Name, in.PR, hours)
		if err != nil {
			return nil, apiclient.PreviewEnvironmentResource{}, fmt.Errorf("extend preview %d of app %q: %w", in.PR, in.Name, err)
		}
		return nil, p, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "run_restore_drill",
		Description: "Start a restore drill that restores a backup into a scratch target and verifies it. Needs confirm equal to backup_id; only exists with --allow-write.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in runRestoreDrillInput) (*mcp.CallToolResult, restoreDrillStarted, error) {
		if err := requireConfirm("run_restore_drill", "backup_id", in.BackupID, in.Confirm); err != nil {
			return nil, restoreDrillStarted{}, err
		}
		id, err := client.StartBackupDrill(ctx, in.BackupID)
		if err != nil {
			return nil, restoreDrillStarted{}, fmt.Errorf("start restore drill for backup %q: %w", in.BackupID, err)
		}
		return nil, restoreDrillStarted{DrillID: id, BackupID: in.BackupID, Status: "started"}, nil
	})
}
