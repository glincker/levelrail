package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runModelsFit implements "models fit": POST /api/v1/models/fit, or
// GET /api/v1/models/{name}/fit with --name.
func runModelsFit(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "models fit", "print the fit report as JSON to stdout and nothing else", stderr)
	var req apiclient.ModelFitRequest
	var name, gpus, devices, node string
	fs.StringVar(&name, "name", "", "rate an already deployed model against every GPU node")
	fs.StringVar(&req.Engine, "engine", "", "inference engine: ollama, vllm or llamacpp")
	fs.StringVar(&req.Model, "model", "", "model reference: an Ollama tag (llama3.1:8b) or a HuggingFace repo (org/name)")
	fs.StringVar(&node, "node", "", "rate only this node ID (default: every GPU node)")
	fs.StringVar(&gpus, "gpus", "all", "number of GPUs, or \"all\"")
	fs.StringVar(&devices, "gpu-devices", "", "comma-separated GPU indexes or UUIDs (overrides --gpus)")
	fs.IntVar(&req.ContextLength, "context", 0, "context length in tokens (0 assumes the default)")
	fs.StringVar(&req.Quantization, "quantization", "", "vllm quantization method")
	fs.Int64Var(&req.WeightsBytes, "weights-bytes", 0, "exact download size in bytes, from a preflight (default: estimated from the name)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %[1]s models fit --engine ENGINE --model MODEL [flags]\n  %[1]s models fit --name NAME\n\nEstimates whether a model fits in each node's free VRAM: weights plus KV cache plus\nengine overhead. The result is an estimate, not a guarantee.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if name == "" && (req.Engine == "" || req.Model == "") {
		return reportError(stdout, stderr, jsonOut, newValidationError("give --name of a deployed model, or both --engine and --model"))
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	var (
		rep apiclient.ModelFitReport
		err error
	)
	if name != "" {
		rep, err = client.GetModelFit(context.Background(), name)
	} else {
		count, cerr := parseGPUCountFlag(gpus)
		if cerr != nil {
			return reportError(stdout, stderr, jsonOut, cerr)
		}
		req.GPUCount = count
		if devices != "" {
			req.GPUDeviceIDs = strings.Split(devices, ",")
		}
		if fs.Lookup("node").Value.String() != "" {
			req.NodeID = &node
		}
		rep, err = client.CheckModelFit(context.Background(), req)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("check model fit: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, rep, func() { printModelFit(stdout, rep) })
}

func printModelFit(out io.Writer, r apiclient.ModelFitReport) {
	if len(r.Nodes) == 0 {
		_, _ = fmt.Fprintln(out, "no node has reported an NVIDIA GPU")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NODE\tVERDICT\tESTIMATE")
	for _, n := range r.Nodes {
		label := n.Name
		if n.Current {
			label += " (current)"
		}
		if !n.Eligible {
			label += " (not schedulable)"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", label, n.Verdict, n.Arithmetic)
	}
	_ = tw.Flush()
	for _, n := range r.Nodes {
		if n.Reason != "" {
			_, _ = fmt.Fprintf(out, "\n%s: %s\n", n.Name, n.Reason)
		}
		for _, s := range n.Suggestions {
			_, _ = fmt.Fprintf(out, "%s: %s\n", n.Name, s)
		}
	}
	_, _ = fmt.Fprintf(out, "\n%s\n", r.Note)
}
