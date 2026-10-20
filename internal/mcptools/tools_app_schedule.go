package mcptools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// registerAppScheduleTools adds the read-only view of an app's recurring
// redeploy schedule and its recent evaluation history. Changing a
// schedule stays with the dashboard and CLI, the same split
// registerDeployFreezeTools's own doc comment establishes for freeze
// windows.
func registerAppScheduleTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "get_app_schedule",
		Description: "Show an app's configured recurring redeploy schedule (cron expression, branch, timezone, whether it's enabled, and its next run time), or that none is configured.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, apiclient.AppScheduleResource, error) {
		res, err := client.GetAppSchedule(ctx, in.Name)
		if err != nil {
			return nil, apiclient.AppScheduleResource{}, fmt.Errorf("get app schedule for %q: %w", in.Name, err)
		}
		return nil, res, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_app_schedule_history",
		Description: "Show an app's most recent scheduled-deploy evaluations, newest first: whether each fired, was skipped because a deploy freeze window was active, or failed, with a reason.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, []apiclient.AppScheduleHistoryEntry, error) {
		res, err := client.ListAppScheduleHistory(ctx, in.Name, 0)
		if err != nil {
			return nil, nil, fmt.Errorf("get app schedule history for %q: %w", in.Name, err)
		}
		return nil, res, nil
	})
}
