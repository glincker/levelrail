package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
)

// domainsTLSCertJSONUsage is every domains tls-cert subcommand's --json
// flag description: identical across get/set/clear since each returns
// the same BYO-certificate-state shape.
const domainsTLSCertJSONUsage = "print the tls cert state as JSON to stdout and nothing else"

// runDomainsTLSCert dispatches "domains tls-cert <verb> [flags]" to one
// of get/set/clear, mirroring runDomainsBasicAuth's own top-level
// dispatch shape for a different per-domain credential.
func runDomainsTLSCert(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, domainsTLSCertUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, domainsTLSCertUsage(prog))
		return exitOK
	case "get":
		return runDomainsTLSCertGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runDomainsTLSCertSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "clear":
		return runDomainsTLSCertClear(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown domains tls-cert subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, domainsTLSCertUsage(prog))
		return exitUsage
	}
}

func domainsTLSCertUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains tls-cert get <app> <domain> [flags]                                     show a domain's BYO certificate state
  %[1]s domains tls-cert set <app> <domain> --cert-file FILE --key-file FILE [flags]     upload a certificate
  %[1]s domains tls-cert clear <app> <domain> [flags]                                    revert to automatic ACME/internal issuance

Uploads an operator-supplied certificate and private key for one of
<app>'s domains, used by the embedded Caddy ingress in place of
automatic ACME/internal issuance on the next reconcile pass. Useful for
internal-only domains with no public DNS, externally issued wildcard
certs, or a certificate already provisioned before DNS cuts over.
<domain> must already be one of <app>'s configured domains (see "%[1]s
apps get <app>" or "%[1]s domains list").

Run "%[1]s domains tls-cert <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runDomainsTLSCertGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains tls-cert get", domainsTLSCertJSONUsage, stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains tls-cert get <app> <domain> [flags]\n\nShows a domain's currently uploaded BYO certificate state (never the certificate or key material itself).\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, "domains tls-cert get", "an app name and a domain", 2)
	if !ok {
		return exitUsage
	}
	appName, domain := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	cert, err := client.GetDomainTLSCert(context.Background(), appName, domain)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get tls cert for domain %q: %w", domain, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, cert, func() { printDomainTLSCertHuman(stdout, cert) })
}

func runDomainsTLSCertSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains tls-cert set", domainsTLSCertJSONUsage, stderr)
	var certFileFlag, keyFileFlag string
	fs.StringVar(&certFileFlag, "cert-file", "", "path to the certificate PEM file (required)")
	fs.StringVar(&keyFileFlag, "key-file", "", "path to the private key PEM file (required)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains tls-cert set <app> <domain> --cert-file FILE --key-file FILE [flags]\n\nUploads a certificate/key pair for <domain>, read from local PEM files.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, "domains tls-cert set", "an app name and a domain", 2)
	if !ok {
		return exitUsage
	}
	appName, domain := rest[0], rest[1]
	if certFileFlag == "" || keyFileFlag == "" {
		_, _ = fmt.Fprintf(stderr, "%s: domains tls-cert set requires --cert-file and --key-file\n\n", prog)
		fs.Usage()
		return exitUsage
	}

	certPEM, err := os.ReadFile(certFileFlag) //nolint:gosec // operator-supplied CLI flag, the same pattern secrets.go's own --file read uses
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("read cert file %q: %w", certFileFlag, err))
	}
	keyPEM, err := os.ReadFile(keyFileFlag) //nolint:gosec // operator-supplied CLI flag, same pattern as certFileFlag above
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("read key file %q: %w", keyFileFlag, err))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	cert, err := client.SetDomainTLSCert(context.Background(), appName, domain, setDomainTLSCertRequest{
		Cert: strings.TrimSpace(string(certPEM)),
		Key:  strings.TrimSpace(string(keyPEM)),
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set tls cert for domain %q: %w", domain, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, cert, func() { printDomainTLSCertHuman(stdout, cert) })
}

func runDomainsTLSCertClear(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains tls-cert clear", domainsTLSCertJSONUsage, stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s domains tls-cert clear <app> <domain> [flags]\n\nRemoves <domain>'s BYO certificate, reverting it to automatic ACME/internal issuance.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, "domains tls-cert clear", "an app name and a domain", 2)
	if !ok {
		return exitUsage
	}
	appName, domain := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	cert, err := client.ClearDomainTLSCert(context.Background(), appName, domain)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("clear tls cert for domain %q: %w", domain, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, cert, func() {
		_, _ = fmt.Fprintf(stdout, "tls certificate removed for domain %q, reverting to automatic issuance\n", domain)
	})
}

func printDomainTLSCertHuman(out io.Writer, c domainTLSCertResource) {
	status := "automatic (ACME/internal)"
	if c.Enabled {
		status = "BYO certificate uploaded"
	}
	_, _ = fmt.Fprintf(out, "domain:      %s\nstatus:      %s\n", c.Domain, status)
	if c.Enabled {
		_, _ = fmt.Fprintf(out, "uploaded_at: %s\nexpires_at:  %s\n", c.UploadedAt, c.ExpiresAt)
	}
}
