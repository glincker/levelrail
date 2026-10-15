package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func otelUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s otel destinations list [flags]                    list OTLP destinations and their delivery state
  %[1]s otel destinations add --name N --endpoint U [--signal metrics|logs] [flags]
                                                          add an OTLP/HTTP destination (default signal: metrics)
  %[1]s otel destinations test <id> [flags]               send one probe and report the result
  %[1]s otel destinations remove <id> [flags]             delete a destination and its stored headers

Add accepts the same flags as "%[1]s export destinations create -h" (--app, --header, --label, --interval,
--min-level for logs). Header values are stored encrypted and never shown again. Nothing is exported until
a destination exists, and collection stays pull-based on the node.
`, prog)
}

// runOtel handles "otel destinations ...", a signal-first view over the
// OTLP kinds of the data export destinations.
func runOtel(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] != "destinations" || len(args) < 2 {
		if len(args) > 0 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help") {
			_, _ = fmt.Fprint(stdout, otelUsage(prog))
			return exitOK
		}
		_, _ = fmt.Fprint(stderr, otelUsage(prog))
		return exitUsage
	}
	rest := args[2:]
	switch args[1] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, otelUsage(prog))
		return exitOK
	case "list":
		return runOtelList(prog, rest, stdout, stderr, lookupEnv)
	case "add":
		return runOtelAdd(prog, rest, stdout, stderr, lookupEnv)
	case "test":
		return runExportDestTest(prog, rest, stdout, stderr, lookupEnv)
	case "remove":
		return runExportDestDelete(prog, rest, stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown otel destinations subcommand %q\n\n%s", prog, args[1], otelUsage(prog))
	return exitUsage
}

func isOTLPKind(kind string) bool { return strings.HasSuffix(kind, "_otlp") }

func runOtelList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCLICall(prog, "otel destinations list", otelUsage(prog)+commonFlagsHelp, args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	all, err := c.client.ListExportDestinations(context.Background())
	if err != nil {
		return c.fail(fmt.Errorf("list otel destinations: %w", err))
	}
	dests := make([]apiclient.ExportDestinationResource, 0, len(all))
	for _, d := range all {
		if isOTLPKind(string(d.Kind)) {
			dests = append(dests, d)
		}
	}
	return c.render(dests, func() { printExportTable(stdout, dests) })
}

// runOtelAdd maps --signal onto the OTLP kind and hands the rest to the
// export destination create flow so both surfaces validate identically.
func runOtelAdd(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	signal := "metrics"
	forwarded := make([]string, 0, len(args)+2)
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--signal" || a == "-signal":
			if i+1 >= len(args) {
				_, _ = fmt.Fprintf(stderr, "%s: --signal needs a value (metrics or logs)\n", prog)
				return exitUsage
			}
			signal = args[i+1]
			i++
		case strings.HasPrefix(a, "--signal="):
			signal = strings.TrimPrefix(a, "--signal=")
		default:
			forwarded = append(forwarded, a)
		}
	}
	if signal != "metrics" && signal != "logs" {
		_, _ = fmt.Fprintf(stderr, "%s: --signal must be metrics or logs, got %q\n", prog, signal)
		return exitUsage
	}
	forwarded = append(forwarded, "--kind", signal+"_otlp")
	return runExportDestCreate(prog, forwarded, stdout, stderr, lookupEnv)
}
