package main

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerScheduledTaskTools registers only the read side of scheduled
// tasks: an app's cron-driven commands, and each one's most recent run
// outcome. Creating, editing, deleting, or running one now stays a
// human-in-the-loop action in the CLI/dashboard, matching feature
// flags' own read-only MCP surface (tools_flags.go) for the same
// reason: "run now" has a real, immediate side effect inside a live
// container.
func registerScheduledTaskTools(server *mcp.Server, client *apiclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_scheduled_tasks",
		Description: "List an app's scheduled tasks: command, cron schedule, enabled state, and the most recent run's outcome (status, exit code, consecutive failures). Read-only; does not create, edit, delete, or run a task.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, []apiclient.ScheduledTaskResource, error) {
		tasks, err := client.ListScheduledTasks(ctx, in.Name)
		if err != nil {
			return nil, nil, fmt.Errorf("list scheduled tasks for app %q: %w", in.Name, err)
		}
		return nil, tasks, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_scheduled_task",
		Description: "Get one scheduled task's full detail: command, cron schedule, enabled state, and the most recent run's status, exit code, and captured output. Read-only; does not create, edit, delete, or run a task.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in scheduledTaskInput) (*mcp.CallToolResult, apiclient.ScheduledTaskResource, error) {
		task, err := client.GetScheduledTask(ctx, in.Name, in.ID)
		if err != nil {
			return nil, apiclient.ScheduledTaskResource{}, fmt.Errorf("get scheduled task %q for app %q: %w", in.ID, in.Name, err)
		}
		return nil, task, nil
	})
}

type scheduledTaskInput struct {
	Name string `json:"name" jsonschema:"the app's name that owns the task"`
	ID   string `json:"id" jsonschema:"the scheduled task's id"`
}
