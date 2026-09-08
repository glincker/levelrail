package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// runCertificates dispatches "certificates <verb> [flags]", currently
// just "list": GET /api/v1/certificates (internal/api/certificates.go's
// own handleListCertificates doc comment), every TLS certificate the
// control plane's embedded ingress currently manages.
func runCertificates(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, certificatesUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, certificatesUsage(prog))
		return exitOK
	case "list":
		return runCertificatesList(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown certificates subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, certificatesUsage(prog))
		return exitUsage
	}
}

func certificatesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s certificates list [flags]   list every TLS certificate the embedded ingress manages

Run "%[1]s certificates <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runCertificatesList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "certificates list", "print certificates as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, certificatesListUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	certs, err := client.ListCertificates(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list certificates: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, certs, func() { printCertificatesTable(stdout, certs) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printCertificatesTable(out io.Writer, certs []certificateResource) {
	if len(certs) == 0 {
		_, _ = fmt.Fprintln(out, "no certificates")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "DOMAIN\tSTATUS\tISSUER\tNOT_AFTER")
	for _, c := range certs {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", c.Domain, c.Status, c.Issuer, c.NotAfter.Format("2006-01-02"))
	}
	_ = tw.Flush()
}

func certificatesListUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s certificates list [flags]

Lists every TLS certificate the embedded ingress currently manages.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print certificates as a JSON array to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
