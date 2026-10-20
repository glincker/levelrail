package main

import (
	"context"
	"fmt"
	"io"
)

func runModelsDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runModelsAction(prog, args, stdout, stderr, lookupEnv, modelAction{
		verb: "delete", doing: "Removes a model: its container is removed by the reconciler.\nThe downloaded weights volume is kept so a redeploy does not download again.",
		done: "model %q deleted",
		call: func(ctx context.Context, c *Client, name string) error { return c.DeleteModel(ctx, name) },
	})
}

func runModelsRestart(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runModelsAction(prog, args, stdout, stderr, lookupEnv, modelAction{
		verb: "restart", doing: "Recreates the model's engine container.",
		done: "model %q restarting",
		call: func(ctx context.Context, c *Client, name string) error { return c.RestartModel(ctx, name) },
	})
}

type modelAction struct {
	verb, doing, done string
	call              func(ctx context.Context, c *Client, name string) error
}

func runModelsAction(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), a modelAction) int {
	label := "models " + a.verb
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, label, "print {\"ok\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s %s <name> [flags]\n\n%s\n\nFlags:\n", prog, label, a.doing)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, label, "model name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	if err := a.call(context.Background(), client, name); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s model %q: %w", a.verb, name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, map[string]bool{"ok": true}, func() {
		_, _ = fmt.Fprintf(stdout, a.done+"\n", name)
	})
}

func runModelsRotateKey(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "models rotate-key", "print the new API key as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s models rotate-key <name> [flags]\n\nIssues a new API key and invalidates the old one. The new key is printed once.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "models rotate-key", "model name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	key, err := client.RotateModelAPIKey(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("rotate api key of model %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, key, func() {
		_, _ = fmt.Fprintf(stdout, "new api key: %s\nSave it now, it is not shown again.\n", key.APIKey)
	})
}
