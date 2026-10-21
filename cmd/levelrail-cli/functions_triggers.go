package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func runFunctionsTriggers(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintf(stderr, functionsUsage, prog)
		return exitUsage
	}
	verb := args[0]
	argLabel := ""
	switch args[0] {
	case "list", "webhook", "enqueue":
		argLabel = "function name"
	case "retry", "discard":
		argLabel = "invocation id"
	case "dead-letters":
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown functions triggers subcommand %q\n\n", prog, verb)
		_, _ = fmt.Fprintf(stderr, functionsUsage, prog)
		return exitUsage
	}
	label := "functions triggers " + verb
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, label, "print the result as JSON to stdout and nothing else", stderr)
	path := fs.String("path", "/", "request path on the function (webhook, enqueue)")
	method := fs.String("method", http.MethodPost, "HTTP method (enqueue)")
	data := fs.String("data", "", "request body (enqueue)")
	rotate := fs.Bool("rotate", false, "generate a new secret (webhook)")
	disable := fs.Bool("disable", false, "turn the webhook off (webhook)")
	function := fs.String("function", "", "only this function (dead-letters)")
	status := fs.String("status", "dead", "queued, running, done or dead (dead-letters)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, `Usage:
  %[1]s functions triggers list <name>
  %[1]s functions triggers webhook <name> [--path /] [--rotate] [--disable]
  %[1]s functions triggers enqueue <name> [--path /] [--method POST] [--data BODY]
  %[1]s functions triggers dead-letters [--function <name>] [--status dead]
  %[1]s functions triggers retry <id>
  %[1]s functions triggers discard <id>
`+"\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args[1:], apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	target := ""
	if argLabel != "" {
		if target, ok = requireOneArg(fs, stderr, prog, label, argLabel); !ok {
			return exitUsage
		}
	}

	switch verb {
	case "list":
		res, err := client.GetFunctionTriggers(ctx, target)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("get triggers of %q: %w", target, err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, res, func() {
			if res.Webhook == nil {
				_, _ = fmt.Fprintln(stdout, "webhook: off")
			} else {
				_, _ = fmt.Fprintf(stdout, "webhook: %s (forwards to %s, replay window %ds)\n", res.Webhook.URL, res.Webhook.Path, res.Webhook.ReplayWindowSeconds)
			}
			for _, s := range res.Schedules {
				_, _ = fmt.Fprintf(stdout, "schedule: %-20s %s %s enabled=%t\n", s.Name, s.Schedule, s.Timezone, s.Enabled)
			}
			_, _ = fmt.Fprintf(stdout, "queue: %d queued, %d running, %d done, %d dead\n", res.Queue["queued"], res.Queue["running"], res.Queue["done"], res.Queue["dead"])
		})
	case "webhook":
		if *disable {
			if err := client.DeleteFunctionWebhook(ctx, target); err != nil {
				return reportError(stdout, stderr, jsonOut, fmt.Errorf("disable webhook of %q: %w", target, err))
			}
			_, _ = fmt.Fprintf(stdout, "webhook of %q turned off\n", target)
			return exitOK
		}
		res, err := client.PutFunctionWebhook(ctx, target, *path, *rotate)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("set webhook of %q: %w", target, err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, res, func() {
			_, _ = fmt.Fprintf(stdout, "url: <control plane>%s\nforwards to: %s\n", res.URL, res.Path)
			if res.Secret != "" {
				_, _ = fmt.Fprintf(stdout, "secret (shown once): %s\n", res.Secret)
			}
			_, _ = fmt.Fprintf(stdout, "sign: X-Signature-Timestamp: <unix seconds>, X-Signature: sha256=hex(HMAC-SHA256(secret, timestamp + \".\" + body)), within %ds\n", res.ReplayWindowSeconds)
		})
	case "enqueue":
		res, err := client.EnqueueFunctionCall(ctx, target, strings.ToUpper(*method), *path, *data)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("enqueue call to %q: %w", target, err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, res, func() { _, _ = fmt.Fprintf(stdout, "queued %s\n", res.ID) })
	case "dead-letters":
		rows, err := client.ListQueuedFunctionCalls(ctx, *function, *status, 0)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("list invocations: %w", err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, rows, func() {
			if len(rows) == 0 {
				_, _ = fmt.Fprintf(stdout, "no %s invocations\n", *status)
				return
			}
			for _, r := range rows {
				_, _ = fmt.Fprintf(stdout, "%-28s %-16s %s %s  %d/%d attempts  HTTP %d  %s\n", r.ID, r.Function, r.Method, r.Path, r.Attempts, r.MaxAttempts, r.LastStatusCode, r.LastError)
			}
		})
	case "retry":
		if err := client.RetryFunctionInvocation(ctx, target); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("retry %q: %w", target, err))
		}
		_, _ = fmt.Fprintf(stdout, "%s queued again\n", target)
	default:
		if err := client.DeleteFunctionInvocation(ctx, target); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("discard %q: %w", target, err))
		}
		_, _ = fmt.Fprintf(stdout, "%s discarded\n", target)
	}
	return exitOK
}
