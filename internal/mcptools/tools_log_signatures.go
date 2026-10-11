package mcptools

import (
	"context"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type logSignaturesInput struct {
	App      string `json:"app" jsonschema:"app name"`
	Since    string `json:"since,omitempty" jsonschema:"duration like 24h or RFC3339; default 1h"`
	Level    string `json:"level,omitempty" jsonschema:"minimum level, default error"`
	Baseline bool   `json:"baseline,omitempty" jsonschema:"mark signatures absent from the window before"`
}

type logSignatureOut struct {
	ID       string `json:"id"`
	Template string `json:"template"`
	Count    int    `json:"count"`
	Last     string `json:"last_seen"`
	New      bool   `json:"new,omitempty"`
	Sample   string `json:"sample"`
}

type logSignaturesOutput struct {
	Scanned    int               `json:"scanned"`
	Truncated  bool              `json:"truncated,omitempty"`
	Signatures []logSignatureOut `json:"signatures"`
}

func registerLogSignaturesTool(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "log_signatures",
		Description: "Top clusters of similar error log lines for an app, with counts and a sample.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in logSignaturesInput) (*mcp.CallToolResult, logSignaturesOutput, error) {
		now := time.Now()
		from := now.Add(-time.Hour)
		if in.Since != "" {
			t, err := apiclient.ParseLogTime(in.Since, now)
			if err != nil {
				return nil, logSignaturesOutput{}, fmt.Errorf("since: %w", err)
			}
			from = t
		}
		res, err := client.GetLogSignatures(ctx, in.App, apiclient.LogSignatureQuery{
			From: from, To: now, Level: in.Level, Limit: 10, Baseline: in.Baseline,
		})
		if err != nil {
			return nil, logSignaturesOutput{}, fmt.Errorf("log signatures: %w", err)
		}
		out := logSignaturesOutput{Scanned: res.Scanned, Truncated: res.Truncated, Signatures: make([]logSignatureOut, 0, len(res.Signatures))}
		for _, s := range res.Signatures {
			out.Signatures = append(out.Signatures, logSignatureOut{
				ID: s.ID, Template: s.Template, Count: s.Count, Last: s.LastSeen.UTC().Format(time.RFC3339), New: s.New, Sample: s.Sample,
			})
		}
		return nil, out, nil
	})
}
