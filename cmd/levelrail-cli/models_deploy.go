package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const envHFToken = "HF_TOKEN"

// runModelsDeploy implements "models deploy": POST /api/v1/models.
func runModelsDeploy(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "models deploy", "print the model and its one-time API key as JSON to stdout and nothing else", stderr)
	var req apiclient.CreateModelRequest
	var gpus, devices string
	var hfFromEnv bool
	fs.StringVar(&req.Name, "name", "", "model name (lowercase letters, digits, hyphens)")
	fs.StringVar(&req.Engine, "engine", "", "inference engine: ollama, vllm or llamacpp")
	fs.StringVar(&req.Model, "model", "", "model reference: an Ollama tag (llama3.1:8b) or a HuggingFace repo (org/name)")
	fs.StringVar(&req.NodeID, "node", "", "node ID to run on (default: the control plane's own host)")
	fs.StringVar(&gpus, "gpus", "all", "number of GPUs, or \"all\"")
	fs.StringVar(&devices, "gpu-devices", "", "comma-separated GPU indexes or UUIDs (overrides --gpus)")
	fs.IntVar(&req.ContextLength, "context", 0, "maximum context length in tokens (0 keeps the engine default)")
	fs.StringVar(&req.Quantization, "quantization", "", "quantization method for vllm (for example awq, gptq, fp8)")
	fs.StringVar(&req.Domain, "domain", "", "hostname for the OpenAI-compatible endpoint (default: a zero-config hostname when APP_PUBLIC_HOST is a public IP)")
	fs.BoolVar(&hfFromEnv, "hf-token-from-env", false, "read a HuggingFace token from the "+envHFToken+" environment variable and store it encrypted")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, modelsDeployUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if req.Name == "" || req.Engine == "" || req.Model == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--name, --engine and --model are required"))
	}
	count, err := parseGPUCountFlag(gpus)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	req.GPUCount = count
	if devices != "" {
		req.GPUDeviceIDs = strings.Split(devices, ",")
	}
	if hfFromEnv {
		tok, found := lookupEnv(envHFToken)
		if !found || strings.TrimSpace(tok) == "" {
			return reportError(stdout, stderr, jsonOut, newValidationError("--hf-token-from-env set but %s is empty", envHFToken))
		}
		req.HFToken = strings.TrimSpace(tok)
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	created, err := client.CreateModel(context.Background(), req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("deploy model %q: %w", req.Name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, created, func() {
		_, _ = fmt.Fprintf(stdout, "model %q deployed (%s on %s)\n", created.Name, created.Engine, modelNodeLabel(created.NodeID))
		if created.EndpointURL != "" {
			_, _ = fmt.Fprintf(stdout, "base url:  %s\n", created.EndpointURL)
		}
		_, _ = fmt.Fprintf(stdout, "api key:   %s\n", created.APIKey)
		_, _ = fmt.Fprintln(stdout, "Save the API key now, it is not shown again. Use \"models rotate-key\" to issue a new one.")
	})
}

func parseGPUCountFlag(v string) (int, error) {
	if v == "" || v == "all" {
		return -1, nil
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil || n < 1 {
		return 0, newValidationError("--gpus must be \"all\" or a positive number, got %q", v)
	}
	return n, nil
}

func modelsDeployUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s models deploy --name NAME --engine ENGINE --model MODEL [flags]

Deploys an AI model as a container on a GPU node. The engine downloads the
weights into a persistent volume, the model is loaded, and an
OpenAI-compatible endpoint is exposed with an auto-generated API key,
printed once.

Examples:
  %[1]s models deploy --name chat --engine ollama --model llama3.1:8b
  %[1]s models deploy --name llama --engine vllm --model meta-llama/Llama-3.1-8B-Instruct --hf-token-from-env --gpus 2

Flags:
  --name string              model name
  --engine string            ollama, vllm or llamacpp
  --model string             Ollama tag or HuggingFace repo
  --node string              node ID (default: this host)
  --gpus string              number of GPUs or "all" (default "all")
  --gpu-devices string       comma-separated GPU indexes or UUIDs
  --context int              maximum context length in tokens
  --quantization string      vllm quantization method
  --domain string            hostname for the endpoint
  --hf-token-from-env        read a HuggingFace token from %[5]s
  --token string             API token (default: %[2]s env var, then the credentials file)
  --api-url string           control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string           named credentials profile to read
  --json                     print the result as JSON
  --output string            output format: json, table, or text
  --query string             JMESPath expression to filter the result
  -h, --help                 show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL, envHFToken)
}
