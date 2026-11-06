package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
)

// runAppsDeploysReceipt implements "apps deploys receipt <name> <deploy-id>".
func runAppsDeploysReceipt(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps deploys receipt", "print the receipt JSON to stdout and nothing else", stderr)
	var out, format string
	fs.StringVar(&out, "out", "", "write the receipt to this file instead of stdout")
	fs.StringVar(&format, "format", "json", "receipt format: json or md")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsDeploysReceiptUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, _, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) != 2 {
		_, _ = fmt.Fprintf(stderr, "%s: apps deploys receipt requires an app name and a deploy id\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	if format != "json" && format != "md" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--format must be json or md"))
	}
	name, deployID := rest[0], rest[1]
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	data, err := client.DownloadDeployReceipt(context.Background(), name, deployID, format)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get receipt for deploy %q of app %q: %w", deployID, name, err))
	}
	if out == "" {
		_, _ = stdout.Write(data)
		if !strings.HasSuffix(string(data), "\n") {
			_, _ = fmt.Fprintln(stdout)
		}
		return exitOK
	}
	if err := os.WriteFile(out, data, 0o600); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("write receipt to %s: %w", out, err))
	}
	_, _ = fmt.Fprintf(stdout, "receipt written to %s\n", out)
	return exitOK
}

func appsDeploysReceiptUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps deploys receipt <name> <deploy-id> [--out file] [--format json|md] [flags]

Downloads the deploy receipt: commit, image digest and how it was obtained,
requester, approver, SBOM summary and gate timeline. The receipt is unsigned
and holds names and digests only, never env values or tokens.

Flags:
  --out string          write to this file instead of stdout
  --format string       json (default) or md
  --token string        API token (default: %[2]s env var, then the credentials file)
  --api-url string      control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string      named credentials profile to read (overrides APP_PROFILE, default "default")
  -h, --help            show this help
`, prog, apiTokenEnvHelp, apiURLEnvHelp, defaultAPIURL)
}
