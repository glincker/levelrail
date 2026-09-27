package mcptools

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

type modelEngineMetricsInput struct {
	Name  string `json:"name" jsonschema:"the model's name"`
	Since string `json:"since,omitempty" jsonschema:"how far back to read as a Go duration, e.g. 1h or 24h; defaults to 1h"`
}

func registerModelServingTools(server *mcp.Server, client *apiclient.Client) {
	registerModelFitTool(server, client)
	addTool(server, &mcp.Tool{
		Name:        "get_model_engine_metrics",
		Description: "The inference engine's own metrics for a model: KV cache usage, queued and running requests, prefix cache hit rate, tokens per second and time to first token (vLLM and llama.cpp), or VRAM residency and CPU offload (Ollama), with a health summary. Metrics an engine cannot expose are flagged supported=false. Read-only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in modelEngineMetricsInput) (*mcp.CallToolResult, apiclient.EngineMetricsReport, error) {
		var since time.Duration
		if in.Since != "" {
			d, err := time.ParseDuration(in.Since)
			if err != nil {
				return nil, apiclient.EngineMetricsReport{}, fmt.Errorf("get engine metrics of model %q: invalid since %q: %w", in.Name, in.Since, err)
			}
			since = d
		}
		out, err := client.GetModelEngineMetrics(ctx, in.Name, since)
		if err != nil {
			return nil, apiclient.EngineMetricsReport{}, fmt.Errorf("get engine metrics of model %q: %w", in.Name, err)
		}
		return nil, out, nil
	})
}
