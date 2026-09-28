package main

import (
	"context"
	"fmt"
	"io"
)

// runModelsSwapGroup implements "models swap-group": PUT
// /api/v1/models/{name}/swap-group.
func runModelsSwapGroup(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "models swap-group", "print {\"ok\": true} as JSON to stdout on success and nothing else", stderr)
	var group string
	clearGroup := fs.Bool("clear", false, "remove the model from its swap group")
	fs.StringVar(&group, "group", "", "swap group name: models sharing it on the same node/GPU cannot both be resident")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s models swap-group <name> --group GROUP\n  %s models swap-group <name> --clear\n\nModels in the same swap group on the same node share one GPU: waking one stops the group's\ncurrent resident model first to free VRAM.\n\nFlags:\n", prog, prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "models swap-group", "model name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	if *clearGroup {
		group = ""
	} else if group == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--group is required, or pass --clear to remove the model from its group"))
	}
	if err := client.SetModelSwapGroup(context.Background(), name, group); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set swap group of model %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, map[string]bool{"ok": true}, func() {
		if group == "" {
			_, _ = fmt.Fprintf(stdout, "model %q is no longer in a swap group\n", name)
			return
		}
		_, _ = fmt.Fprintf(stdout, "model %q is now in swap group %q\n", name, group)
	})
}
