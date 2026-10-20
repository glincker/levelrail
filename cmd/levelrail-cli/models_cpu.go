package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func gibText(b int64) string { return fmt.Sprintf("%.1f GiB", float64(b)/(1<<30)) }

func runModelsFitCPU(prog string, stdout, stderr io.Writer, jsonOut bool, of outputFlags, req apiclient.ModelFitRequest, node string, nodeSet bool,
	apiURLFlag, tokenFlag, profileFlag string, lookupEnv func(string) (string, bool)) int {
	if req.CatalogID == "" && req.Model == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("cpu mode needs --catalog ID or --model REPO[:QUANT]"))
	}
	if nodeSet {
		req.NodeID = &node
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	rep, err := client.CheckCPUModelFit(context.Background(), req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("check cpu model fit: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, rep, func() { printCPUFit(stdout, rep) })
}

func printCPUFit(out io.Writer, r apiclient.CPUFitReport) {
	_, _ = fmt.Fprintf(out, "verdict:   %s\n", r.Verdict)
	_, _ = fmt.Fprintf(out, "reason:    %s\n", r.Reason)
	if r.Suggestion != "" {
		_, _ = fmt.Fprintf(out, "suggest:   %s\n", r.Suggestion)
	}
	if r.CapBytes > 0 {
		_, _ = fmt.Fprintf(out, "caps:      %.2f CPUs, %d MiB memory\n", float64(r.CPUMilli)/1000, r.MemoryMiB)
		_, _ = fmt.Fprintf(out, "memory:    %s\n", r.Arithmetic)
		_, _ = fmt.Fprintf(out, "disk:      about %s for the weights\n", gibText(r.DiskBytes))
	}
	if r.TokensPerSecHi > 0 {
		_, _ = fmt.Fprintf(out, "speed:     %.1f to %.1f tokens per second (estimate, run \"models benchmark\" for the real number)\n", r.TokensPerSecLo, r.TokensPerSecHi)
	}
	for _, w := range r.Warnings {
		_, _ = fmt.Fprintf(out, "warning:   %s\n", w)
	}
	if !r.LicenseCurated && r.LicenseNotice != "" {
		_, _ = fmt.Fprintf(out, "licence:   %s Deploy with --accept-license to continue.\n", r.LicenseNotice)
	}
	_, _ = fmt.Fprintf(out, "\n%s\n", r.Note)
}

func runModelsCatalog(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "models catalog", "print the curated list as JSON to stdout and nothing else", stderr)
	var node string
	fs.StringVar(&node, "node", "", "rate the models against this node (default: this host)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s models catalog [flags]\n\nLists the curated small models that can run on CPU, with the licence, the memory\nthey need and an estimated speed on this server. Only Apache-2.0 and MIT models are\ncurated. Weights download from Hugging Face to your server only when you deploy one.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	cat, err := client.GetLocalModelCatalog(context.Background(), node)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("read local model catalog: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, cat, func() { printLocalCatalog(stdout, cat) })
}

func printLocalCatalog(out io.Writer, c apiclient.LocalModelCatalog) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tTIER\tDOWNLOAD\tMIN RAM\tLICENCE\tEST. TOK/S\tON THIS SERVER")
	for _, m := range c.Models {
		speed := "n/a"
		if m.Fit.TokensPerSecHi > 0 {
			speed = fmt.Sprintf("%.1f-%.1f", m.Fit.TokensPerSecLo, m.Fit.TokensPerSecHi)
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%d MiB\t%s\t%s\t%s\n", m.ID, m.Tier, gibText(m.DownloadBytes), m.MinRAMMiB, m.License, speed, m.Fit.Verdict)
	}
	_ = tw.Flush()
	_, _ = fmt.Fprintf(out, "\nVerified on %s. %s\n", c.VerifiedOn, c.Note)
}

func runModelsBenchmark(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "models benchmark", "print the result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s models benchmark <name> [flags]\n\nRuns a short fixed prompt set through the running model and reports prompt and\ngeneration speed, time to first token and peak memory. It has a hard time cap, is\nrate limited, and never starts a stopped model (wake it first).\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "models benchmark", "model name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	res, err := client.BenchmarkModel(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("benchmark model %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printBenchmark(stdout, res) })
}

func printBenchmark(out io.Writer, r apiclient.ModelBenchmarkResult) {
	_, _ = fmt.Fprintf(out, "generation:   %.1f tokens per second (measured)\n", r.GenTPS)
	_, _ = fmt.Fprintf(out, "prompt:       %.1f tokens per second\n", r.PromptTPS)
	_, _ = fmt.Fprintf(out, "first token:  %d ms\n", r.TTFTMillis)
	if r.PeakMemoryBytes > 0 {
		_, _ = fmt.Fprintf(out, "peak memory:  %s of %s\n", gibText(int64(r.PeakMemoryBytes)), gibText(int64(r.MemoryLimitBytes))) //nolint:gosec // container memory sizes fit int64
	}
	_, _ = fmt.Fprintf(out, "took:         %s", (time.Duration(r.DurationMillis) * time.Millisecond).Round(100*time.Millisecond))
	if r.Truncated {
		_, _ = fmt.Fprint(out, " (hit the time cap, partial result)")
	}
	_, _ = fmt.Fprintf(out, "\n\n%s\n", r.Note)
}
