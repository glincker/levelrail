package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type archivesInput struct{}

type archivesOutput struct {
	Archives []string `json:"archives" jsonschema:"one line per bundle, newest first"`
	Policy   string   `json:"policy" jsonschema:"the retention policy in one line"`
}

func registerArchiveTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "list_archives",
		Description: "Archive bundles of cold audit, deploy and pipeline rows, and the retention policy.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ archivesInput) (*mcp.CallToolResult, archivesOutput, error) {
		list, err := client.ListArchives(ctx)
		if err != nil {
			return nil, archivesOutput{}, fmt.Errorf("list archives: %w", err)
		}
		out := archivesOutput{Archives: []string{}}
		for _, b := range list.Bundles {
			out.Archives = append(out.Archives, fmt.Sprintf("%s: %d rows, %d bytes, created %s", b.ID, b.TotalRows, b.SizeBytes, b.CreatedAt))
		}
		p := list.Policy
		out.Policy = fmt.Sprintf("enabled %t, hot days audit %d deploys %d pipelines %d, keep bundles %d days", p.Enabled, p.AuditHotDays, p.DeployHotDays, p.PipelineHotDays, p.KeepDays)
		return nil, out, nil
	})
}
