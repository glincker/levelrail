package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// runAPIDocs implements "api-docs": GET /api/v1/openapi.json, the same
// route metadata the web dashboard's Settings > API explorer renders.
// There is no interactive "try it" here, a terminal isn't the right
// place for that; this just dumps the route table for scripting or a
// quick lookup without opening a browser.
func runAPIDocs(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "api-docs", "print the route table as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, apiDocsUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	spec, err := client.GetOpenAPISpec(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get openapi spec: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, spec, func() { printAPIDocsHuman(stdout, spec) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printAPIDocsHuman(out io.Writer, spec openAPISpecResource) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "METHOD\tPATH\tABILITY\tGROUP")
	for _, r := range spec.Routes {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.Method, r.Path, r.Ability, r.Group)
	}
	_ = tw.Flush()
	_, _ = fmt.Fprintf(out, "\n%d routes, %d with a worked request/response example.\n", spec.Count, spec.ExampleCount)
}

func apiDocsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s api-docs [flags]

Prints every registered HTTP route on this control plane: method, path,
required ability, resource group, and (where generated) a short
description. The same data backing Settings > API explorer in the web
dashboard, where you can also try a request interactively.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the route table as JSON, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
