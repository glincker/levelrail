package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
)

const domainsGoogleCloudDNSJSONUsage = "print the result as JSON to stdout and nothing else"

func runDomainsGoogleCloudDNS(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, domainsGoogleCloudDNSUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, domainsGoogleCloudDNSUsage(prog))
		return exitOK
	case "get":
		return runGoogleCloudDNSVerb(prog, "get", args[1:], stdout, stderr, lookupEnv, "", func(c *Client, _ string) (any, func(io.Writer), error) {
			s, err := c.GetGoogleCloudDNS(context.Background())
			return s, func(w io.Writer) { printGoogleCloudDNSSettings(w, s) }, err
		})
	case "clear":
		return runGoogleCloudDNSVerb(prog, "clear", args[1:], stdout, stderr, lookupEnv, "", func(c *Client, _ string) (any, func(io.Writer), error) {
			s, err := c.DisconnectGoogleCloudDNS(context.Background())
			return s, func(w io.Writer) { printGoogleCloudDNSSettings(w, s) }, err
		})
	case "test":
		return runGoogleCloudDNSVerb(prog, "test", args[1:], stdout, stderr, lookupEnv, "", func(c *Client, _ string) (any, func(io.Writer), error) {
			res, err := c.TestGoogleCloudDNS(context.Background())
			return res, func(w io.Writer) { printGoogleCloudDNSTest(w, res) }, err
		})
	case "set":
		return runGoogleCloudDNSVerb(prog, "set", args[1:], stdout, stderr, lookupEnv, "key-file", func(c *Client, keyFile string) (any, func(io.Writer), error) {
			raw, err := os.ReadFile(keyFile) //nolint:gosec // operator-chosen path to their own key file
			if err != nil {
				return nil, nil, fmt.Errorf("read key file: %w", err)
			}
			s, err := c.SetGoogleCloudDNS(context.Background(), updateGoogleCloudDNSRequest{Enabled: true, ServiceAccountJSON: strings.TrimSpace(string(raw))})
			return s, func(w io.Writer) { printGoogleCloudDNSSettings(w, s) }, err
		})
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown domains google-cloud-dns subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, domainsGoogleCloudDNSUsage(prog))
		return exitUsage
	}
}

func domainsGoogleCloudDNSUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains google-cloud-dns get [flags]                     show the connection (project and client email only)
  %[1]s domains google-cloud-dns set --key-file FILE [flags]     store a service account JSON key and enable
  %[1]s domains google-cloud-dns test [flags]                    list the project's managed zones with the stored key
  %[1]s domains google-cloud-dns clear [flags]                   disable and forget the key

Connects Google Cloud DNS for DNS records, automatic records on domain add
and ACME DNS-01 (wildcard certificates). The service account needs the
DNS Administrator role. Cloudflare and Route53 take precedence if enabled.
Only tested against a local fake of the Cloud DNS API.
`, prog)
}

// runGoogleCloudDNSVerb registers the common flags (plus --key-file when keyFlag is set), then calls action.
func runGoogleCloudDNSVerb(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), keyFlag string, action func(*Client, string) (any, func(io.Writer), error)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains google-cloud-dns "+verb, domainsGoogleCloudDNSJSONUsage, stderr)
	var keyFile string
	if keyFlag != "" {
		fs.StringVar(&keyFile, keyFlag, "", "path to the service account JSON key file (required)")
	}
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains google-cloud-dns %s [flags]\n\nFlags:\n", prog, verb)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if keyFlag != "" && keyFile == "" {
		_, _ = fmt.Fprintf(stderr, "%s: domains google-cloud-dns %s requires --%s\n\n", prog, verb, keyFlag)
		fs.Usage()
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	result, render, err := action(client, keyFile)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s google cloud dns: %w", verb, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, result, func() { render(stdout) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printGoogleCloudDNSSettings(out io.Writer, s googleCloudDNSResource) {
	_, _ = fmt.Fprintf(out, "enabled:      %v\n", s.Enabled)
	_, _ = fmt.Fprintf(out, "project_id:   %s\n", s.ProjectID)
	_, _ = fmt.Fprintf(out, "client_email: %s\n", s.ClientEmail)
	_, _ = fmt.Fprintf(out, "has_key:      %v\n", s.HasKey)
}

func printGoogleCloudDNSTest(out io.Writer, r googleCloudDNSTestResult) {
	if !r.OK {
		_, _ = fmt.Fprintf(out, "failed: %s\n", r.Error)
		return
	}
	_, _ = fmt.Fprintf(out, "ok: %d managed zone(s)\n", len(r.Zones))
	for _, z := range r.Zones {
		_, _ = fmt.Fprintf(out, "  %s\n", z)
	}
}
