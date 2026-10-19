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

type modelEndpointHealthOutput struct {
	State     string `json:"state"`
	LatencyMS int64  `json:"latency_ms"`
	Failures  int    `json:"failures"`
	Problem   string `json:"problem,omitempty"`
	Fix       string `json:"fix,omitempty"`
}

func registerModelServingTools(server *mcp.Server, client *apiclient.Client) {
	registerModelFitTool(server, client)
	addTool(server, &mcp.Tool{
		Name:        "get_model_endpoint_health",
		Description: "Last health probe of an external inference endpoint: state, latency, failures, fix.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in modelNameInput) (*mcp.CallToolResult, modelEndpointHealthOutput, error) {
		m, err := client.GetModel(ctx, in.Name)
		if err != nil {
			return nil, modelEndpointHealthOutput{}, fmt.Errorf("get endpoint health of model %q: %w", in.Name, err)
		}
		if m.External == nil {
			return nil, modelEndpointHealthOutput{}, fmt.Errorf("model %q is not an external endpoint, use get_model_engine_metrics", in.Name)
		}
		h := m.External.Health
		out := modelEndpointHealthOutput{State: h.State, LatencyMS: h.LatencyMS, Failures: h.Failures}
		if h.Diagnosis != nil {
			out.Problem, out.Fix = h.Diagnosis.Message, h.Diagnosis.Fix
		}
		return nil, out, nil
	})
	addTool(server, &mcp.Tool{
		Name:        "get_model_engine_metrics",
		Description: "Inference engine metrics for a model: KV cache, queue, prefix cache hits, tokens per second, time to first token (vLLM, llama.cpp) or VRAM residency and CPU offload (Ollama), with a health summary. Unavailable metrics are flagged supported=false.",
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
