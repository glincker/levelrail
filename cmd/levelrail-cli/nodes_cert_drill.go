package main

import (
	"context"
	"fmt"
	"io"
)

func nodesCertDrillUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes cert drill <id> [flags]

Rotates node <id>'s agent certificate end to end and reports each step:
preflight, renewal request, new certificate, node still online. The old
certificate stays valid and is never revoked, so a failed drill leaves the
node as it was. Exits non-zero when any step fails.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the result as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runNodesCertDrill(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes cert drill", "print the result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesCertDrillUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	id, ok := requireOneArg(fs, stderr, prog, "nodes cert drill", "node id")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.DrillNodeCert(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("certificate rotation drill for node %q: %w", id, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() {
		for _, s := range res.Steps {
			_, _ = fmt.Fprintf(stdout, "%-16s %-8s %s\n", s.ID, s.Status, s.Detail)
		}
		if res.OK {
			_, _ = fmt.Fprintf(stdout, "drill passed: certificate generation %d to %d\n", res.Before.Generation, res.After.Generation)
		}
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	if !res.OK {
		return exitCheckFailed
	}
	return exitOK
}
