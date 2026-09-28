package mcptools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

type checkModelFitInput struct {
	Engine        string   `json:"engine,omitempty" jsonschema:"ollama, vllm or llamacpp; required unless name is set"`
	Model         string   `json:"model,omitempty" jsonschema:"model reference, for example llama3.1:8b or org/name; required unless name is set"`
	Name          string   `json:"name,omitempty" jsonschema:"rate an already deployed model instead of a candidate"`
	Quantization  string   `json:"quantization,omitempty" jsonschema:"vLLM quantization method such as awq"`
	ContextLength int      `json:"context_length,omitempty" jsonschema:"context length in tokens; 0 assumes the default"`
	GPUCount      int      `json:"gpu_count,omitempty" jsonschema:"number of GPUs; 0 or negative means all"`
	GPUDeviceIDs  []string `json:"gpu_device_ids,omitempty" jsonschema:"GPU indexes or UUIDs"`
	WeightsBytes  int64    `json:"weights_bytes,omitempty" jsonschema:"exact download size from a preflight, otherwise estimated from the name"`
	NodeID        *string  `json:"node_id,omitempty" jsonschema:"rate only this node; empty string is the control plane's own host; omit for every GPU node"`
}

func registerModelFitTool(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "check_model_fit",
		Description: "Estimate whether a model fits in each GPU node's free VRAM (fits, tight, wont_fit or unknown): weights plus KV cache at the context length plus engine overhead, ranked best node first. Give engine and model for a candidate, or name for a deployed model. Figures are estimates, not guarantees; the note field says so. Read-only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in checkModelFitInput) (*mcp.CallToolResult, apiclient.ModelFitReport, error) {
		if in.Name != "" {
			out, err := client.GetModelFit(ctx, in.Name)
			if err != nil {
				return nil, apiclient.ModelFitReport{}, fmt.Errorf("check fit of model %q: %w", in.Name, err)
			}
			return nil, out, nil
		}
		out, err := client.CheckModelFit(ctx, apiclient.ModelFitRequest{
			Engine: in.Engine, Model: in.Model, Quantization: in.Quantization, ContextLength: in.ContextLength,
			GPUCount: in.GPUCount, GPUDeviceIDs: in.GPUDeviceIDs, WeightsBytes: in.WeightsBytes, NodeID: in.NodeID,
		})
		if err != nil {
			return nil, apiclient.ModelFitReport{}, fmt.Errorf("check model fit: %w", err)
		}
		return nil, out, nil
	})
}
