package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type getJanitorInput struct {
	Preview bool `json:"preview,omitempty" jsonschema:"also dry-run a pass and list what would be removed"`
}

type janitorPreviewItem struct {
	Stage     string `json:"stage"`
	Action    string `json:"action"`
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
}

type getJanitorOutput struct {
	Mode           string               `json:"mode"`
	FreePercent    float64              `json:"free_percent"`
	Pressure       bool                 `json:"pressure"`
	ReclaimedBytes int64                `json:"reclaimed_bytes"`
	LastPass       string               `json:"last_pass,omitempty"`
	Quarantined    []string             `json:"quarantined"`
	Preview        []janitorPreviewItem `json:"preview,omitempty"`
}

func janitorMode(p apiclient.JanitorPolicy) string {
	switch {
	case !p.Enabled:
		return "off"
	case p.DryRun:
		return "preview"
	}
	return "removing"
}

func registerJanitorTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "get_janitor",
		Description: "Automatic cleanup state: mode, free disk, bytes reclaimed, quarantined volumes. preview dry-runs a pass. Rollback targets and live volumes are never touched.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in getJanitorInput) (*mcp.CallToolResult, getJanitorOutput, error) {
		st, err := client.GetJanitor(ctx)
		if err != nil {
			return nil, getJanitorOutput{}, fmt.Errorf("get janitor: %w", err)
		}
		out := getJanitorOutput{Mode: janitorMode(st.Policy), FreePercent: st.FreePercent, Pressure: st.Pressure, ReclaimedBytes: st.TotalReclaimed, Quarantined: []string{}}
		if st.LastRun != nil {
			out.LastPass = st.LastRun.StartedAt.Format("2006-01-02T15:04:05Z07:00")
		}
		for _, q := range st.Quarantine {
			out.Quarantined = append(out.Quarantined, q.Name)
		}
		if in.Preview {
			rep, err := client.RunJanitor(ctx, true, true)
			if err != nil {
				return nil, getJanitorOutput{}, fmt.Errorf("preview janitor: %w", err)
			}
			for _, s := range rep.Stages {
				for _, it := range s.Items {
					out.Preview = append(out.Preview, janitorPreviewItem{Stage: s.Stage, Action: it.Action, Name: it.Name, SizeBytes: it.SizeBytes})
				}
			}
		}
		return nil, out, nil
	})
}
