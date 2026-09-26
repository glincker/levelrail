package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runModelsPreflight implements "models preflight": POST /api/v1/models/preflight.
func runModelsPreflight(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "models preflight", "print the preflight result as JSON to stdout and nothing else", stderr)
	var req apiclient.ModelPreflightRequest
	var hfFromEnv bool
	fs.StringVar(&req.Engine, "engine", "", "engine the model will run on: ollama, vllm or llamacpp")
	fs.StringVar(&req.Quant, "quant", "", "GGUF quantization to check, for example Q4_K_M")
	fs.StringVar(&req.File, "file", "", "a single repository file to check")
	fs.StringVar(&req.NodeID, "node", "", "node ID to check fit and disk against (default: the control plane's own host)")
	fs.BoolVar(&hfFromEnv, "hf-token-from-env", false, "read a HuggingFace token from the "+envHFToken+" environment variable (used for this check only)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s models preflight <repo> [flags]\n\nChecks a Hugging Face repository before deploying: existence, gated access,\nlicense, download size, GGUF quantizations with a fit estimate, and free disk\non the target node. GPU fit numbers are estimates.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, repo, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "models preflight", "Hugging Face repo (owner/name[:quant])"}, lookupEnv)
	if !ok {
		return exitCode
	}
	req.Repo = repo
	if hfFromEnv {
		tok, found := lookupEnv(envHFToken)
		if !found || strings.TrimSpace(tok) == "" {
			return reportError(stdout, stderr, jsonOut, newValidationError("--hf-token-from-env set but %s is empty", envHFToken))
		}
		req.HFToken = strings.TrimSpace(tok)
	}
	res, err := client.PreflightModel(context.Background(), req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("preflight %q: %w", repo, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printModelPreflightHuman(stdout, res) })
}

func humanBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

func optBytes(b *int64) string {
	if b == nil {
		return "unknown"
	}
	return humanBytes(*b)
}

func printModelPreflightHuman(out io.Writer, r apiclient.ModelPreflightResult) {
	_, _ = fmt.Fprintf(out, "%s: %s\n%s\n", r.Repo, r.Status, r.Message)
	if r.NextStep != "" {
		_, _ = fmt.Fprintf(out, "next: %s\n", r.NextStep)
	}
	if !r.Exists {
		return
	}
	if r.License != "" {
		_, _ = fmt.Fprintf(out, "license: %s\n", r.License)
	}
	_, _ = fmt.Fprintf(out, "files: %d, %s in total\n%s\n", r.FileCount, humanBytes(r.TotalBytes), r.EngineHint)
	if len(r.Quants) > 0 {
		tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "\nQUANT\tSIZE\tFIT\t")
		for _, q := range r.Quants {
			mark := ""
			if q.Recommended {
				mark = "recommended"
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", q.Name, humanBytes(q.Bytes), q.Fit, mark)
		}
		_ = tw.Flush()
		if r.RecommendationNote != "" {
			_, _ = fmt.Fprintln(out, r.RecommendationNote)
		}
	}
	_, _ = fmt.Fprintf(out, "\nnode free VRAM: %s, free disk: %s\n", optBytes(r.Node.VRAMFreeBytes), optBytes(r.Node.DiskFreeBytes))
	_, _ = fmt.Fprintf(out, "disk: %s (needs %s). %s\n", r.Disk.Status, humanBytes(r.Disk.RequiredBytes), r.Disk.Message)
	for _, w := range r.Warnings {
		_, _ = fmt.Fprintf(out, "warning: %s\n", w)
	}
	_, _ = fmt.Fprintln(out, r.EstimateNote)
}
