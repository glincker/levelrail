package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// runRegistryCredentialsList implements "registry-credentials list": GET
// /api/v1/registry-credentials.
func runRegistryCredentialsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "registry-credentials list", "print registry credentials as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, registryCredentialsListUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	creds, err := client.ListRegistryCredentials(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list registry credentials: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, creds, func() { printRegistryCredentialsTable(stdout, creds) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printRegistryCredentialsTable(out io.Writer, creds []registryCredentialResource) {
	if len(creds) == 0 {
		_, _ = fmt.Fprintln(out, "no registry credentials")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tREGISTRY HOST\tUSERNAME\tCREATED")
	for _, c := range creds {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", c.ID, c.Name, c.RegistryHost, c.Username, c.CreatedAt)
	}
	_ = tw.Flush()
}

func registryCredentialsListUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s registry-credentials list [flags]

Lists every connected registry credential. Passwords are never included.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print registry credentials as a JSON array to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
