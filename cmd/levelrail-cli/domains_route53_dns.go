package main

import (
	"context"
	"fmt"
	"io"
)

// domainsRoute53DNSJSONUsage is every domains route53-dns subcommand's
// --json flag description: identical across get/set/clear since each
// returns the same settings shape.
const domainsRoute53DNSJSONUsage = "print the settings as JSON to stdout and nothing else"

// runDomainsRoute53DNS dispatches "domains route53-dns <verb> [flags]"
// to one of get/set/clear, mirroring runDomainsCloudflareDNS's own
// top-level dispatch shape for this second, independent ACME DNS-01
// provider.
func runDomainsRoute53DNS(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, domainsRoute53DNSUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, domainsRoute53DNSUsage(prog))
		return exitOK
	case "get":
		return runDomainsRoute53DNSGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runDomainsRoute53DNSSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "clear":
		return runDomainsRoute53DNSClear(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown domains route53-dns subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, domainsRoute53DNSUsage(prog))
		return exitUsage
	}
}

func domainsRoute53DNSUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains route53-dns get [flags]                                            show the current settings
  %[1]s domains route53-dns set --aws-access-key-id ID --aws-secret-access-key KEY [flags]  configure and enable
  %[1]s domains route53-dns clear [flags]                                          disable and forget the credentials

Configures the AWS IAM access key pair ACME uses for the DNS-01
challenge via Route53, a second, independent provider from Cloudflare
DNS-01 (only one is ever active per reconcile pass; Cloudflare takes
precedence if both are enabled). DNS-01 is the only challenge type that
can prove control of a wildcard domain (e.g. "*.example.com"). A domain
becomes wildcard-eligible just by having a leading "*." label; there is
no separate flag for that.

Run "%[1]s domains route53-dns <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runDomainsRoute53DNSGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains route53-dns get", domainsRoute53DNSJSONUsage, stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains route53-dns get [flags]\n\nShows the current Route53 DNS-01 settings.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	settings, err := client.GetRoute53DNS(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get route53 dns settings: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, settings, func() { printRoute53DNSSettings(stdout, settings) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runDomainsRoute53DNSSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains route53-dns set", domainsRoute53DNSJSONUsage, stderr)
	var accessKeyIDFlag, secretAccessKeyFlag, regionFlag, hostedZoneIDFlag string
	fs.StringVar(&accessKeyIDFlag, "aws-access-key-id", "", "AWS IAM access key ID (required)")
	fs.StringVar(&secretAccessKeyFlag, "aws-secret-access-key", "", "AWS IAM secret access key (required)")
	fs.StringVar(&regionFlag, "aws-region", "", "AWS region (optional; libdns/route53 resolves it itself when empty)")
	fs.StringVar(&hostedZoneIDFlag, "hosted-zone-id", "", "Route53 hosted zone ID (optional; resolved by matching the domain when empty)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains route53-dns set --aws-access-key-id ID --aws-secret-access-key KEY [flags]\n\nConfigures and enables Route53 DNS-01 for wildcard domains.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	if accessKeyIDFlag == "" || secretAccessKeyFlag == "" {
		_, _ = fmt.Fprintf(stderr, "%s: domains route53-dns set requires --aws-access-key-id and --aws-secret-access-key\n\n", prog)
		fs.Usage()
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	settings, err := client.SetRoute53DNS(context.Background(), updateRoute53DNSRequest{
		Enabled:         true,
		Region:          regionFlag,
		HostedZoneID:    hostedZoneIDFlag,
		AccessKeyID:     accessKeyIDFlag,
		SecretAccessKey: secretAccessKeyFlag,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set route53 dns settings: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, settings, func() { printRoute53DNSSettings(stdout, settings) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runDomainsRoute53DNSClear(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains route53-dns clear", domainsRoute53DNSJSONUsage, stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains route53-dns clear [flags]\n\nDisables Route53 DNS-01 and forgets the stored credentials.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	settings, err := client.DisconnectRoute53DNS(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("clear route53 dns settings: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, settings, func() { printRoute53DNSSettings(stdout, settings) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printRoute53DNSSettings(out io.Writer, s route53DNSResource) {
	_, _ = fmt.Fprintf(out, "enabled:               %v\n", s.Enabled)
	_, _ = fmt.Fprintf(out, "region:                %s\n", s.Region)
	_, _ = fmt.Fprintf(out, "hosted_zone_id:        %s\n", s.HostedZoneID)
	_, _ = fmt.Fprintf(out, "has_access_key_id:     %v\n", s.HasAccessKeyID)
	_, _ = fmt.Fprintf(out, "has_secret_access_key: %v\n", s.HasSecretAccessKey)
}
