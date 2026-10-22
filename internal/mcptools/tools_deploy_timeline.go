package mcptools

import (
	"context"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const eventKindLog = "log"

type deployTimelineInput struct {
	DeployID string `json:"deploy_id" jsonschema:"deploy id"`
}

type deployTimelineEvent struct {
	At     string `json:"at"`
	Code   string `json:"code"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type deployTimelineOutput struct {
	DeployID string                `json:"deploy_id"`
	App      string                `json:"app"`
	Verdict  string                `json:"verdict"`
	Live     bool                  `json:"live"`
	Events   []deployTimelineEvent `json:"events"`
	PhaseMS  map[string]int64      `json:"phase_ms"`
	Traffic  string                `json:"traffic,omitempty"`
	LogLines int                   `json:"log_lines"`
}

func trafficSummary(im *apiclient.DeployEventImpact) string {
	if im == nil {
		return ""
	}
	side := func(w apiclient.DeployEventWindow) string {
		if !w.HasTraffic {
			return "no traffic"
		}
		return fmt.Sprintf("%.2f req/s, %.1f%% 5xx, p95 %.0fms", w.RatePerSec, w.ErrorRate5xx*100, w.P95Ms)
	}
	return "before " + side(im.Before) + "; after " + side(im.After)
}

func registerDeployTimelineTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "get_deploy_timeline",
		Description: "Ordered events of one deploy: trigger, build, cutover, watch signals, rollback, traffic impact.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in deployTimelineInput) (*mcp.CallToolResult, deployTimelineOutput, error) {
		tl, err := client.DeployEventTimeline(ctx, in.DeployID)
		if err != nil {
			return nil, deployTimelineOutput{}, fmt.Errorf("get timeline for deploy %q: %w", in.DeployID, err)
		}
		out := deployTimelineOutput{
			DeployID: tl.DeployID, App: tl.App, Verdict: tl.Verdict, Live: tl.Live,
			Events: make([]deployTimelineEvent, 0, len(tl.Events)), PhaseMS: map[string]int64{}, Traffic: trafficSummary(tl.Impact),
		}
		for _, p := range tl.Phases {
			out.PhaseMS[p.Phase] = p.DurationMS
		}
		for _, e := range tl.Events {
			if e.Kind == eventKindLog {
				out.LogLines++
				continue
			}
			out.Events = append(out.Events, deployTimelineEvent{At: e.At.UTC().Format(time.RFC3339), Code: e.Code, Status: e.Status, Detail: e.Detail})
		}
		return nil, out, nil
	})
}
