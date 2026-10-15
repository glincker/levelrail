package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func appsTracesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps traces list <app> [--errors] [--min-ms N] [--from T] [--to T] [--limit N] [flags]
  %[1]s apps traces get <app> <trace-id> [flags]   print one trace as a tree of spans
  %[1]s apps tracing enable <app> [flags]          create (or rotate) the app's OTLP ingest key and print the env to set
  %[1]s apps tracing status <app> [flags]          show whether tracing is on and where to send spans
  %[1]s apps tracing disable <app> [flags]         remove the ingest key; the app's exporter gets 401s

Spans are received over OTLP/HTTP (protobuf or JSON) and kept on this node for
the logs retention window.
`, prog)
}

func runAppsTraces(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		_, _ = fmt.Fprint(stdout, appsTracesUsage(prog))
		if len(args) == 0 {
			return exitUsage
		}
		return exitOK
	}
	switch args[0] {
	case "list":
		return runTracesList(prog, args[1:], stdout, stderr, lookupEnv)
	case "get":
		return runTracesGet(prog, args[1:], stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown apps traces subcommand %q\n\n%s", prog, args[0], appsTracesUsage(prog))
	return exitUsage
}

func runAppsTracing(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		_, _ = fmt.Fprint(stdout, appsTracesUsage(prog))
		if len(args) == 0 {
			return exitUsage
		}
		return exitOK
	}
	verb := args[0]
	if verb != "enable" && verb != "status" && verb != "disable" {
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps tracing subcommand %q\n\n%s", prog, verb, appsTracesUsage(prog))
		return exitUsage
	}
	label := "apps tracing " + verb
	usage := fmt.Sprintf("Usage:\n  %s %s <app> [flags]\n%s", prog, label, commonFlagsHelp)
	c, code, ok := parseCLICall(prog, label, usage, args[1:], stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	app, ok := requireOneArg(c.fs, stderr, prog, label, "app name")
	if !ok {
		return exitUsage
	}
	ctx := context.Background()
	switch verb {
	case "enable":
		res, err := c.client.EnableTracing(ctx, app)
		if err != nil {
			return c.fail(fmt.Errorf("enable tracing for %q: %w", app, err))
		}
		return c.render(res, func() {
			_, _ = fmt.Fprintf(stdout, "tracing is on for %s. Set these on the app, then redeploy:\n\n", app)
			keys := make([]string, 0, len(res.Env))
			for k := range res.Env {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				_, _ = fmt.Fprintf(stdout, "  %s=%s\n", k, res.Env[k])
			}
			_, _ = fmt.Fprintln(stdout, "\nThe key is shown once. Running this again rotates it.")
		})
	case "disable":
		if err := c.client.DisableTracing(ctx, app); err != nil {
			return c.fail(fmt.Errorf("disable tracing for %q: %w", app, err))
		}
		return c.render(map[string]bool{"disabled": true}, func() {
			_, _ = fmt.Fprintf(stdout, "tracing is off for %s\n", app)
		})
	default:
		res, err := c.client.GetTracing(ctx, app)
		if err != nil {
			return c.fail(fmt.Errorf("get tracing for %q: %w", app, err))
		}
		return c.render(res, func() {
			state := "off"
			if res.Enabled {
				state = "on"
			}
			_, _ = fmt.Fprintf(stdout, "tracing: %s\nendpoint: %s\n", state, res.Endpoint)
		})
	}
}

func runTracesList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var o apiclient.TraceListOptions
	label := "apps traces list"
	usage := fmt.Sprintf("Usage:\n  %s %s <app> [flags]\n\nFlags:\n  --errors       only traces with an error span\n  --min-ms int   only traces at least this long\n  --from string  RFC3339 start (default one hour ago)\n  --to string    RFC3339 end\n  --limit int    at most this many (default 50)\n%s", prog, label, commonFlagsHelp)
	c, code, ok := parseCLICall(prog, label, usage, args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.BoolVar(&o.ErrorsOnly, "errors", false, "only traces with an error")
		fs.IntVar(&o.MinMillis, "min-ms", 0, "minimum duration in milliseconds")
		fs.StringVar(&o.From, "from", "", "RFC3339 start")
		fs.StringVar(&o.To, "to", "", "RFC3339 end")
		fs.IntVar(&o.Limit, "limit", 0, "maximum traces")
	})
	if !ok {
		return code
	}
	app, ok := requireOneArg(c.fs, stderr, prog, label, "app name")
	if !ok {
		return exitUsage
	}
	traces, err := c.client.ListTraces(context.Background(), app, o)
	if err != nil {
		return c.fail(fmt.Errorf("list traces for %q: %w", app, err))
	}
	return c.render(traces, func() {
		if len(traces) == 0 {
			_, _ = fmt.Fprintln(stdout, "no traces in that window")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "TRACE\tSTARTED\tDURATION\tSPANS\tERRORS\tROOT")
		for _, t := range traces {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\t%s\n", t.TraceID, time.Unix(0, t.StartNs).UTC().Format(time.RFC3339),
				time.Duration(t.DurationNs).Round(time.Microsecond), t.Spans, t.Errors, t.RootName)
		}
		_ = tw.Flush()
	})
}

func runTracesGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	label := "apps traces get"
	usage := fmt.Sprintf("Usage:\n  %s %s <app> <trace-id> [flags]\n%s", prog, label, commonFlagsHelp)
	c, code, ok := parseCLICall(prog, label, usage, args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	if c.fs.NArg() != 2 {
		_, _ = fmt.Fprintf(stderr, "%s: %s needs an app name and a trace id\n", prog, label)
		return exitUsage
	}
	app, id := c.fs.Arg(0), c.fs.Arg(1)
	spans, err := c.client.GetTrace(context.Background(), app, id)
	if err != nil {
		return c.fail(fmt.Errorf("get trace %q for %q: %w", id, app, err))
	}
	return c.render(spans, func() { printSpanTree(stdout, spans) })
}

// printSpanTree prints spans as an indented tree, children under parents.
func printSpanTree(out io.Writer, spans []apiclient.TraceSpan) {
	children := map[string][]apiclient.TraceSpan{}
	known := map[string]bool{}
	for _, s := range spans {
		known[s.SpanID] = true
	}
	var roots []apiclient.TraceSpan
	for _, s := range spans {
		if s.ParentID == "" || !known[s.ParentID] {
			roots = append(roots, s)
			continue
		}
		children[s.ParentID] = append(children[s.ParentID], s)
	}
	var origin int64
	if len(spans) > 0 {
		origin = spans[0].StartNs
	}
	var walk func(s apiclient.TraceSpan, depth int)
	walk = func(s apiclient.TraceSpan, depth int) {
		mark := ""
		if s.StatusCode == 2 {
			mark = "  ERROR"
			if s.StatusMessage != "" {
				mark += ": " + s.StatusMessage
			}
		}
		_, _ = fmt.Fprintf(out, "%s%s  +%s  %s%s\n", strings.Repeat("  ", depth), s.Name,
			time.Duration(s.StartNs-origin).Round(time.Microsecond), time.Duration(s.DurationNs).Round(time.Microsecond), mark)
		for _, c := range children[s.SpanID] {
			walk(c, depth+1)
		}
	}
	for _, r := range roots {
		walk(r, 0)
	}
}
