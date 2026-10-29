package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type storageDestinationIDInput struct {
	ID string `json:"id" jsonschema:"the storage destination's id"`
}

type logArchivePolicyInput struct {
	AppName       string `json:"app_name,omitempty" jsonschema:"app to archive; omit for every app"`
	TargetID      string `json:"target_id" jsonschema:"storage destination id"`
	Interval      string `json:"interval,omitempty" jsonschema:"how often to ship new logs, 5m to 24h (default 1h)"`
	RetentionDays int    `json:"retention_days,omitempty" jsonschema:"delete archived objects older than this many days; 0 keeps forever"`
	Enabled       *bool  `json:"enabled,omitempty" jsonschema:"false pauses the policy (default true)"`
}

type logArchiveDumpInput struct {
	AppName  string `json:"app_name,omitempty" jsonschema:"app to dump; omit for every app"`
	TargetID string `json:"target_id" jsonschema:"storage destination id"`
	From     string `json:"from" jsonschema:"range start, RFC3339"`
	To       string `json:"to" jsonschema:"range end, RFC3339"`
}

type logArchiveRunsInput struct {
	AppName string `json:"app_name,omitempty" jsonschema:"app to filter by; omit for every run"`
}

type archivedLogsInput struct {
	TargetID string `json:"target_id" jsonschema:"storage destination id"`
	AppName  string `json:"app_name,omitempty" jsonschema:"app to filter by; omit for every app"`
	Cursor   string `json:"cursor,omitempty" jsonschema:"next-page cursor from a previous call"`
}

func registerLogArchiveTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "list_storage_destinations",
		Description: "List connected storage destinations (S3 compatible or a node directory): name, kind, bucket, region, used bytes, quota, estimated cost and policy count. No credentials.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.StorageDestination, error) {
		out, err := client.ListStorageDestinations(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list storage destinations: %w", err)
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "test_storage_destination",
		Description: "Write, read back and delete a probe object in a storage destination's bucket to prove its credentials work. Returns per-step results and a failure reason.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in storageDestinationIDInput) (*mcp.CallToolResult, apiclient.StorageProbeResult, error) {
		out, err := client.TestStorageDestination(ctx, in.ID)
		if err != nil {
			return nil, apiclient.StorageProbeResult{}, fmt.Errorf("test storage destination %q: %w", in.ID, err)
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "list_log_archive_policies",
		Description: "List log archive policies (which apps ship node-local logs to which storage destination, how often, retention) with each one's last success and last error.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.LogArchivePolicy, error) {
		out, err := client.ListLogArchivePolicies(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list log archive policies: %w", err)
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "set_log_archive_policy",
		Description: "Create or replace the log archive policy for one app (or all when app_name is omitted): ship node logs to a storage destination on a schedule. Only new logs; use start_log_archive_dump for history.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in logArchivePolicyInput) (*mcp.CallToolResult, apiclient.LogArchivePolicy, error) {
		out, err := client.SetLogArchivePolicy(ctx, apiclient.LogArchivePolicyRequest{
			AppName: in.AppName, TargetID: in.TargetID, Interval: in.Interval, RetentionDays: in.RetentionDays, Enabled: in.Enabled,
		})
		if err != nil {
			return nil, apiclient.LogArchivePolicy{}, fmt.Errorf("set log archive policy: %w", err)
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "start_log_archive_dump",
		Description: "Archive a past time range of an app's (or every app's) logs to a storage destination now. Poll list_log_archive_runs for the outcome.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in logArchiveDumpInput) (*mcp.CallToolResult, apiclient.LogArchiveRun, error) {
		out, err := client.StartLogArchiveDump(ctx, apiclient.LogArchiveDumpRequest{AppName: in.AppName, TargetID: in.TargetID, From: in.From, To: in.To})
		if err != nil {
			return nil, apiclient.LogArchiveRun{}, fmt.Errorf("start log archive dump: %w", err)
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "list_log_archive_runs",
		Description: "List recent log archive runs (scheduled and manual dumps) with status, object and line counts, and any error.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in logArchiveRunsInput) (*mcp.CallToolResult, []apiclient.LogArchiveRun, error) {
		out, err := client.ListLogArchiveRuns(ctx, in.AppName, in.AppName == "")
		if err != nil {
			return nil, nil, fmt.Errorf("list log archive runs: %w", err)
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "list_archived_logs",
		Description: "List archived log objects (key, size, modified time) in a storage destination, newest partitions last, optionally for one app. Read-only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in archivedLogsInput) (*mcp.CallToolResult, apiclient.LogArchiveObjects, error) {
		out, err := client.ListLogArchiveObjects(ctx, in.TargetID, in.AppName, in.Cursor)
		if err != nil {
			return nil, apiclient.LogArchiveObjects{}, fmt.Errorf("list archived logs: %w", err)
		}
		return nil, out, nil
	})
}
