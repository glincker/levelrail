package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type controlPlaneOffboxBackupsOutput struct {
	Backups []apiclient.ControlPlaneOffboxBackup `json:"backups"`
}

type controlPlaneDrillOutput struct {
	LastDrill               apiclient.ControlPlaneDRDrill `json:"last_drill"`
	NextDrillAt             string                        `json:"next_drill_at,omitempty"`
	DrillSchedule           string                        `json:"drill_schedule"`
	DrillIdentityConfigured bool                          `json:"drill_identity_configured"`
	DrillRunning            bool                          `json:"drill_running"`
}

func registerControlPlaneDRTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "get_control_plane_dr_status",
		Description: "Control plane DR status: destination, schedule, retention, last backup and drill, escrow, checklist, warnings. Never key material.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, apiclient.ControlPlaneDR, error) {
		st, err := client.GetControlPlaneDR(ctx)
		if err != nil {
			return nil, apiclient.ControlPlaneDR{}, fmt.Errorf("get control plane disaster recovery status: %w", err)
		}
		return nil, st, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "list_control_plane_offbox_backups",
		Description: "List the encrypted off-box control plane database backups, newest first: object key, size, creation time and upload completion.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, controlPlaneOffboxBackupsOutput, error) {
		list, err := client.ListControlPlaneOffboxBackups(ctx)
		if err != nil {
			return nil, controlPlaneOffboxBackupsOutput{}, fmt.Errorf("list control plane off-box backups: %w", err)
		}
		return nil, controlPlaneOffboxBackupsOutput{Backups: list}, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_control_plane_drill_status",
		Description: "Last automated restore drill of the control plane backups: when it ran, whether it passed or was partial (checksum only), how long it took, and when the next one is due.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, controlPlaneDrillOutput, error) {
		st, err := client.GetControlPlaneDR(ctx)
		if err != nil {
			return nil, controlPlaneDrillOutput{}, fmt.Errorf("get control plane drill status: %w", err)
		}
		return nil, controlPlaneDrillOutput{
			LastDrill: st.LastDrill, NextDrillAt: st.NextDrillAt, DrillSchedule: st.DrillSchedule,
			DrillIdentityConfigured: st.DrillIdentityConfigured, DrillRunning: st.DrillRunning,
		}, nil
	})
}
