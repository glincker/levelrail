package mcptools

import (
	"context"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type modelNameInput struct {
	Name string `json:"name" jsonschema:"the model's name"`
}

type modelKeyInput struct {
	Name  string `json:"name" jsonschema:"the model's name"`
	KeyID string `json:"key_id" jsonschema:"the key id from list_model_keys"`
}

type modelLogsInput struct {
	Name  string `json:"name" jsonschema:"the model's name"`
	Since string `json:"since,omitempty" jsonschema:"how far back to search as a Go duration, e.g. 30m or 2h; defaults to 1h"`
	Query string `json:"query,omitempty" jsonschema:"optional full-text search phrase"`
	Tail  int    `json:"tail,omitempty" jsonschema:"return only the last N entries; defaults to and is capped at 200"`
}

type deployModelInput struct {
	Name          string   `json:"name" jsonschema:"model name: lowercase letters, digits and hyphens"`
	Engine        string   `json:"engine" jsonschema:"inference engine: ollama, vllm or llamacpp"`
	Model         string   `json:"model" jsonschema:"an Ollama tag such as llama3.1:8b, or a HuggingFace repo such as meta-llama/Llama-3.1-8B-Instruct"`
	NodeID        string   `json:"node_id,omitempty" jsonschema:"GPU node to run on; empty means the control plane's own host"`
	GPUCount      int      `json:"gpu_count,omitempty" jsonschema:"number of GPUs; 0 or -1 means all GPUs on the node"`
	GPUDeviceIDs  []string `json:"gpu_device_ids,omitempty" jsonschema:"specific GPU indexes or UUIDs; overrides gpu_count"`
	ContextLength int      `json:"context_length,omitempty" jsonschema:"maximum context length in tokens; 0 keeps the engine default"`
	Quantization  string   `json:"quantization,omitempty" jsonschema:"quantization method for vllm, e.g. awq"`
	Domain        string   `json:"domain,omitempty" jsonschema:"hostname for the OpenAI-compatible endpoint"`
	HFToken       string   `json:"hf_token,omitempty" jsonschema:"HuggingFace access token for gated models, stored encrypted and never returned"`
}

type modelUsageInput struct {
	Name  string `json:"name" jsonschema:"the model's name"`
	Since string `json:"since,omitempty" jsonschema:"how far back to report as a Go duration, e.g. 24h or 168h; defaults to 24h"`
}

type modelActionResult struct {
	OK bool `json:"ok" jsonschema:"true when the request was accepted"`
}

func registerModelTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "list_models",
		Description: "List AI models deployed on GPU nodes: engine, model, node, and reconcile status (Downloading, Loading, ModelLoaded, NoGPUOnNode, and so on) with progress detail. Read-only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.ModelResource, error) {
		out, err := client.ListModels(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list models: %w", err)
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_model",
		Description: "Get one AI model: status with reason and progress message, OpenAI-compatible base URL, and API key prefix. The API key itself is never returned here.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in modelNameInput) (*mcp.CallToolResult, apiclient.ModelResource, error) {
		m, err := client.GetModel(ctx, in.Name)
		if err != nil {
			return nil, apiclient.ModelResource{}, fmt.Errorf("get model %q: %w", in.Name, err)
		}
		return nil, m, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "list_gpu_nodes",
		Description: "List nodes that report NVIDIA GPUs: driver version, per-GPU VRAM total and used, utilization, whether Docker has the nvidia container runtime, the fix when it does not, how many models run there, and GPU reservations: reserved and free GPU counts plus which apps and models hold them (a workload only places on a node with enough free GPUs). Read-only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.GPUNodeResource, error) {
		out, err := client.ListGPUNodes(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list gpu nodes: %w", err)
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_model_logs",
		Description: "Search a model engine's already-stored logs in a time window; model download and load progress appears here. A bounded historical search, not a live tail: at most 200 entries per call.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in modelLogsInput) (*mcp.CallToolResult, []apiclient.LogEntryResource, error) {
		window := defaultLogsWindow
		if in.Since != "" {
			d, err := time.ParseDuration(in.Since)
			if err != nil {
				return nil, nil, fmt.Errorf("get logs for model %q: invalid since %q: %w", in.Name, in.Since, err)
			}
			window = d
		}
		to := time.Now()
		entries, err := client.QueryModelLogs(ctx, in.Name, to.Add(-window), to, in.Query)
		if err != nil {
			return nil, nil, fmt.Errorf("get logs for model %q: %w", in.Name, err)
		}
		return nil, tailLogEntries(entries, in.Tail), nil
	})

	addTool(server, &mcp.Tool{
		Name:        "deploy_model",
		Description: "Deploy an AI model as a container on a GPU node. Asynchronous: returns once saved, then the engine downloads weights into a persistent volume and loads the model; watch get_model until its status reason is ModelLoaded. The response carries the OpenAI-compatible base URL and a one-time API key that cannot be retrieved again (rotate_model_api_key issues a new one).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in deployModelInput) (*mcp.CallToolResult, apiclient.CreateModelResponse, error) {
		out, err := client.CreateModel(ctx, apiclient.CreateModelRequest{
			Name: in.Name, Engine: in.Engine, Model: in.Model, NodeID: in.NodeID, GPUCount: in.GPUCount,
			GPUDeviceIDs: in.GPUDeviceIDs, ContextLength: in.ContextLength, Quantization: in.Quantization,
			Domain: in.Domain, HFToken: in.HFToken,
		})
		if err != nil {
			return nil, apiclient.CreateModelResponse{}, fmt.Errorf("deploy model %q: %w", in.Name, err)
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "delete_model",
		Description: "Delete an AI model. Its container is removed asynchronously; the downloaded weights volume is kept so a redeploy does not download again.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in modelNameInput) (*mcp.CallToolResult, modelActionResult, error) {
		if err := client.DeleteModel(ctx, in.Name); err != nil {
			return nil, modelActionResult{}, fmt.Errorf("delete model %q: %w", in.Name, err)
		}
		return nil, modelActionResult{OK: true}, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "restart_model",
		Description: "Recreate a model's engine container, for example after fixing the node's GPU runtime.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in modelNameInput) (*mcp.CallToolResult, modelActionResult, error) {
		if err := client.RestartModel(ctx, in.Name); err != nil {
			return nil, modelActionResult{}, fmt.Errorf("restart model %q: %w", in.Name, err)
		}
		return nil, modelActionResult{OK: true}, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "rotate_model_api_key",
		Description: "Issue a new API key for a model, invalidating the old one immediately. The new key is returned once and cannot be retrieved again.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in modelNameInput) (*mcp.CallToolResult, apiclient.ModelAPIKeyResource, error) {
		out, err := client.RotateModelAPIKey(ctx, in.Name)
		if err != nil {
			return nil, apiclient.ModelAPIKeyResource{}, fmt.Errorf("rotate api key of model %q: %w", in.Name, err)
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "list_model_keys",
		Description: "List a model's named API keys: prefix, status (active, rotating, expired, revoked), limits, expiry and last use. Key material is never returned. Read-only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in modelNameInput) (*mcp.CallToolResult, []apiclient.ModelKeyResource, error) {
		out, err := client.ListModelKeys(ctx, in.Name)
		if err != nil {
			return nil, nil, fmt.Errorf("list keys of model %q: %w", in.Name, err)
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "revoke_model_key",
		Description: "Revoke one named API key of a model at once. Clients using it get 401 immediately and it cannot be undone; create or rotate a key to replace it. Destructive.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in modelKeyInput) (*mcp.CallToolResult, modelActionResult, error) {
		if err := client.RevokeModelKey(ctx, in.Name, in.KeyID); err != nil {
			return nil, modelActionResult{}, fmt.Errorf("revoke key %q of model %q: %w", in.KeyID, in.Name, err)
		}
		return nil, modelActionResult{OK: true}, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_model_usage",
		Description: "Gateway usage of a model over a window: requests, status classes, rate-limited count, input and output tokens, average latency and time to first byte, hourly series and a per key breakdown. Tokens only cover responses that carried a usage object; the note field says so. Read-only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in modelUsageInput) (*mcp.CallToolResult, apiclient.ModelUsageReport, error) {
		var since time.Duration
		if in.Since != "" {
			d, err := time.ParseDuration(in.Since)
			if err != nil {
				return nil, apiclient.ModelUsageReport{}, fmt.Errorf("get usage of model %q: invalid since %q: %w", in.Name, in.Since, err)
			}
			since = d
		}
		out, err := client.GetModelUsage(ctx, in.Name, since)
		if err != nil {
			return nil, apiclient.ModelUsageReport{}, fmt.Errorf("get usage of model %q: %w", in.Name, err)
		}
		return nil, out, nil
	})
}
