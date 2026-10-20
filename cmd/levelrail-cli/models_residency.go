package main

import (
	"context"
	"fmt"
	"io"
	"time"
)

// runModelsResidency implements "models residency": PUT /api/v1/models/{name}/residency.
func runModelsResidency(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "models residency", "print {\"ok\": true} as JSON to stdout on success and nothing else", stderr)
	var mode string
	var idleTTL time.Duration
	fs.StringVar(&mode, "mode", "", "always or on_demand (required)")
	fs.DurationVar(&idleTTL, "idle-ttl", 0, "idle time before an on_demand engine stops (0 keeps the platform default)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s models residency <name> --mode always|on_demand [--idle-ttl 15m]\n\nOn-demand models stop their engine after the idle period, freeing VRAM, and start it again on the first\nrequest (which waits while the engine loads). Ollama keeps models loaded forever, so stopping the\ncontainer is how it is unloaded.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "models residency", "model name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	if mode == "" || idleTTL < 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("--mode is required and --idle-ttl must not be negative"))
	}
	if err := client.SetModelResidency(context.Background(), name, mode, int(idleTTL/time.Second)); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set residency of model %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, map[string]bool{"ok": true}, func() {
		_, _ = fmt.Fprintf(stdout, "model %q is now %s\n", name, mode)
	})
}

func runModelsWake(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runModelsAction(prog, args, stdout, stderr, lookupEnv, modelAction{
		verb: "wake", doing: "Starts an on-demand model's engine now instead of waiting for the first request.",
		done: "model %q waking",
		call: func(ctx context.Context, c *Client, name string) error { return c.WakeModel(ctx, name) },
	})
}

func runModelsSleep(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runModelsAction(prog, args, stdout, stderr, lookupEnv, modelAction{
		verb: "sleep", doing: "Stops an on-demand model's engine now, freeing its VRAM. It starts again on the next request.",
		done: "model %q going idle",
		call: func(ctx context.Context, c *Client, name string) error { return c.SleepModel(ctx, name) },
	})
}
