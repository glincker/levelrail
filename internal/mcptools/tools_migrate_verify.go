package mcptools

import (
	"context"
	"fmt"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type migrationVerifyOutput struct {
	Verdict     string   `json:"verdict"`
	Differences []string `json:"differences"`
}

// registerMigrationVerifyTool exposes the latest parity report read-only.
// Running a check needs the source credential, so it stays in the dashboard and CLI.
func registerMigrationVerifyTool(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "get_migration_verify_report",
		Description: "Latest pre-cutover parity report with fix hints.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, migrationVerifyOutput, error) {
		rep, err := client.LatestMigrationVerify(ctx)
		if err != nil {
			return nil, migrationVerifyOutput{}, fmt.Errorf("get migration verify report: %w", err)
		}
		out := migrationVerifyOutput{Differences: []string{},
			Verdict: fmt.Sprintf("%s (%d blocking, %d warning, %d info)", rep.Verdict, rep.Counts.Blocking, rep.Counts.Warning, rep.Counts.Info)}
		for _, d := range rep.Differences {
			who := d.Subject
			if d.App != "" {
				who = d.App + "/" + d.Subject
			}
			out.Differences = append(out.Differences, strings.Join([]string{string(d.Severity), string(d.Category), who}, " ")+": "+d.Detail+". Fix: "+d.Fix)
		}
		return nil, out, nil
	})
}
