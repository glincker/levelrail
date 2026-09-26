package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func modelsKeysUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s models keys list <model> [flags]             list a model's API keys
  %[1]s models keys create <model> --name N [flags]  create a named key (printed once)
  %[1]s models keys revoke <model> <key-id> [flags]  stop a key working at once
  %[1]s models keys rotate <model> <key-id> [flags]  issue a replacement, old key works for a grace window
`, prog)
}

func runModelsKeys(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, modelsKeysUsage(prog))
		return exitUsage
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, modelsKeysUsage(prog))
		return exitOK
	case "list":
		return runModelsKeysList(prog, rest, stdout, stderr, lookupEnv)
	case "create":
		return runModelsKeysCreate(prog, rest, stdout, stderr, lookupEnv)
	case "revoke":
		return runModelsKeysRevoke(prog, rest, stdout, stderr, lookupEnv)
	case "rotate":
		return runModelsKeysRotate(prog, rest, stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown models keys subcommand %q\n\n%s", prog, sub, modelsKeysUsage(prog))
		return exitUsage
	}
}

func runModelsKeysList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "models keys list", "print the keys as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s models keys list <model> [flags]\n\nLists a model's API keys with status, limits and last use. Key material is never shown.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "models keys list", "model name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	keys, err := client.ListModelKeys(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list keys of model %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, keys, func() { printModelKeysTable(stdout, keys) })
}

func fmtKeyTime(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.Local().Format("2006-01-02 15:04")
}

func fmtLimit(n int) string {
	if n <= 0 {
		return "-"
	}
	return fmt.Sprint(n)
}

func printModelKeysTable(out io.Writer, keys []apiclient.ModelKeyResource) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tPREFIX\tSTATUS\tRPM\tTPM\tTPD\tPARALLEL\tEXPIRES\tLAST USED\tCREATED BY")
	for _, k := range keys {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", k.ID, k.Name, k.KeyPrefix, k.Status,
			fmtLimit(k.RPM), fmtLimit(k.TPM), fmtLimit(k.TPD), fmtLimit(k.MaxParallel), fmtKeyTime(k.ExpiresAt), fmtKeyTime(k.LastUsedAt), dash(k.CreatedBy))
	}
	_ = tw.Flush()
}

func runModelsKeysCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "models keys create", "print the key and its one-time secret as JSON to stdout and nothing else", stderr)
	var req apiclient.CreateModelKeyRequest
	var expires time.Duration
	var paths, allowModels string
	fs.StringVar(&req.Name, "name", "", "key name (letters, digits, dots, dashes, underscores)")
	fs.DurationVar(&expires, "expires-in", 0, "expire the key after this long, for example 720h (default: never)")
	fs.IntVar(&req.RPM, "rpm", 0, "requests per minute (0 = unlimited)")
	fs.IntVar(&req.TPM, "tpm", 0, "tokens per minute, enforced after the fact from metered usage (0 = unlimited)")
	fs.IntVar(&req.TPD, "tpd", 0, "tokens per day, enforced after the fact from metered usage (0 = unlimited)")
	fs.IntVar(&req.MaxParallel, "max-parallel", 0, "maximum parallel requests (0 = unlimited)")
	fs.StringVar(&paths, "allow-paths", "", "comma-separated gateway paths the key may call (default: all served paths)")
	fs.StringVar(&allowModels, "allow-models", "", "comma-separated model names the key may request (default: any)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s models keys create <model> --name N [flags]\n\nCreates a named API key. The key is printed once.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, model, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "models keys create", "model name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	if req.Name == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--name is required"))
	}
	if expires > 0 {
		t := time.Now().Add(expires).UTC()
		req.ExpiresAt = &t
	}
	req.AllowPaths, req.AllowModels = splitList(paths), splitList(allowModels)
	created, err := client.CreateModelKey(context.Background(), model, req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create key for model %q: %w", model, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, created, func() {
		_, _ = fmt.Fprintf(stdout, "key %s (%s)\napi key: %s\nSave it now, it is not shown again.\n", created.Name, created.ID, created.APIKey)
	})
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func runModelsKeysRevoke(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "models keys revoke", "print {\"ok\": true} as JSON to stdout on success and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s models keys revoke <model> <key-id> [flags]\n\nStops the key working at once.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, model, id, jsonOut, of, exitCode, ok := parseTwoArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, twoArgCmd{prog, "models keys revoke", "a model name and a key id"}, lookupEnv)
	if !ok {
		return exitCode
	}
	if err := client.RevokeModelKey(context.Background(), model, id); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("revoke key %q of model %q: %w", id, model, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, map[string]bool{"ok": true}, func() {
		_, _ = fmt.Fprintf(stdout, "key %q revoked\n", id)
	})
}

func runModelsKeysRotate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "models keys rotate", "print the new key and its one-time secret as JSON to stdout and nothing else", stderr)
	var grace time.Duration
	fs.DurationVar(&grace, "grace", -1, "how long the old key keeps working, 0 for none (default: the server's rotation grace)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s models keys rotate <model> <key-id> [flags]\n\nIssues a replacement with the same name and limits. The new key is printed once.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, model, id, jsonOut, of, exitCode, ok := parseTwoArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, twoArgCmd{prog, "models keys rotate", "a model name and a key id"}, lookupEnv)
	if !ok {
		return exitCode
	}
	var g *time.Duration
	if grace >= 0 {
		g = &grace
	}
	created, err := client.RotateModelKey(context.Background(), model, id, g)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("rotate key %q of model %q: %w", id, model, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, created, func() {
		_, _ = fmt.Fprintf(stdout, "new key %s (%s)\napi key: %s\nSave it now, it is not shown again.\n", created.Name, created.ID, created.APIKey)
	})
}

func runModelsUsage(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "models usage", "print the usage report as JSON to stdout and nothing else", stderr)
	var since time.Duration
	fs.DurationVar(&since, "since", 24*time.Hour, "how far back to report, for example 168h")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s models usage <name> [flags]\n\nShows gateway requests, tokens, errors and latency per key.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "models usage", "model name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	rep, err := client.GetModelUsage(context.Background(), name, since)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get usage of model %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, rep, func() { printModelUsage(stdout, rep) })
}

func printModelUsage(out io.Writer, r apiclient.ModelUsageReport) {
	t := r.Totals
	_, _ = fmt.Fprintf(out, "requests: %d (2xx %d, 4xx %d, 5xx %d, rate limited %d)\n", t.Requests, t.Status2xx, t.Status4xx, t.Status5xx, t.RateLimited)
	_, _ = fmt.Fprintf(out, "tokens:   %d in, %d out (from %d requests that reported usage)\n", t.InputTokens, t.OutputTokens, t.UsageRequests)
	_, _ = fmt.Fprintf(out, "latency:  avg %d ms, avg time to first byte %d ms\n\n", t.AvgDurationMs, t.AvgTTFTMs)
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "KEY\tSTATUS\tREQUESTS\tERRORS\tIN\tOUT")
	for _, k := range r.Keys {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%d\t%d\n", k.Name, k.Status, k.Requests, k.Status4xx+k.Status5xx, k.InputTokens, k.OutputTokens)
	}
	_ = tw.Flush()
	_, _ = fmt.Fprintf(out, "\n%s\n", r.Note)
}
