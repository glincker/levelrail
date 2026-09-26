package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/attention"
)

const (
	attentionCritical = attention.Critical
	attentionWarning  = attention.Warning
)

type (
	attentionItem  = attention.Item
	attentionInput = attention.Input
)

func buildAttentionItems(in attentionInput) []attentionItem { return attention.Build(in) }

// runAttention implements "attention": the CLI side of the dashboard's
// Status page. Exit code is 1 when any item is critical.
func runAttention(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "attention", "print attention items as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, attentionUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	items, err := attention.Collect(context.Background(), client)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	if err := renderResult(stdout, of.Format, of.Query, items, func() { printAttentionHuman(stdout, items) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	for _, it := range items {
		if it.Severity == attentionCritical {
			return exitUsage
		}
	}
	return exitOK
}

func printAttentionHuman(out io.Writer, items []attentionItem) {
	if len(items) == 0 {
		_, _ = fmt.Fprintln(out, "All systems healthy.")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "SEVERITY\tKIND\tSUBJECT\tDETAIL")
	for _, it := range items {
		detail := it.Detail
		if it.Fixable {
			detail += " [fixable: apps diagnose " + it.Subject + "]"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", it.Severity, it.Kind, it.Subject, detail)
	}
	_ = tw.Flush()
}

func attentionUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s attention [flags]

Lists everything that needs attention right now: failing apps, failed
deploys from the last 24 hours, low disk space (warn under 10 percent free,
critical under 5), offline nodes, expired or expiring certificates, and
doctor warnings or failures.
Exit code is 1 if any item is critical, 0 otherwise.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print attention items as a JSON array to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
