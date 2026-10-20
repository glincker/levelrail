package mcptools

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type queryLogsInput struct {
	App      string `json:"app" jsonschema:"the app's name"`
	DeployID string `json:"deploy_id,omitempty" jsonschema:"only lines from this deploy attempt's window (from list_deploy_attempts)"`
	Level    string `json:"level,omitempty" jsonschema:"minimum level: trace, debug, info, warn, error or fatal; lines with no detectable level are excluded"`
	Since    string `json:"since,omitempty" jsonschema:"start of the window: a duration like 30m or an RFC3339 time; default 1h"`
	Until    string `json:"until,omitempty" jsonschema:"end of the window: a duration ago or an RFC3339 time; default now"`
	Text     string `json:"text,omitempty" jsonschema:"only lines containing this phrase"`
	MaxLines int    `json:"max_lines,omitempty" jsonschema:"most recent lines to return, default 100; output is also capped in bytes"`
}

func registerQueryLogsTool(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "query_logs",
		Description: "Search an app's logs by level, time window, deploy and text, returning a capped excerpt of the newest matches plus counts. Prefer this over get_app_logs.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in queryLogsInput) (*mcp.CallToolResult, apiclient.LogExcerpt, error) {
		ex, err := client.QueryLogsCompact(ctx, apiclient.LogQuery{
			App: in.App, Deploy: in.DeployID, Level: in.Level, Since: in.Since, Until: in.Until, Text: in.Text, MaxLines: in.MaxLines,
		}, apiclient.LogMaxBytesFromEnv(os.LookupEnv), time.Now())
		if err != nil {
			return nil, apiclient.LogExcerpt{}, fmt.Errorf("query logs: %w", err)
		}
		return nil, ex, nil
	})
}
