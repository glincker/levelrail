package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/domaincutover"
)

// runDomainsCutover implements "domains cutover <domain> [--get-certificate]".
func runDomainsCutover(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains cutover", "print the checklist as JSON to stdout and nothing else", stderr)
	var getCert, revert bool
	fs.BoolVar(&getCert, "get-certificate", false, "request the certificate through your connected DNS provider before DNS is switched")
	fs.BoolVar(&revert, "revert-challenge", false, "switch this domain back to HTTP-01 after the switch")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, domainsCutoverUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "domains cutover", "a domain", 1)
	if !ok {
		return exitUsage
	}
	domain := strings.ToLower(rest[0])
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	if revert {
		st, err := client.DomainCutoverRevertChallenge(ctx, domain)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("revert challenge for %q: %w", domain, err))
		}
		if !jsonOut {
			_, _ = fmt.Fprintf(stdout, "%s uses HTTP-01 again (certificate: %s)\n", domain, st.State)
		}
	}
	if getCert {
		st, err := client.DomainCutoverCertificate(ctx, domain)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("certificate for %q: %w", domain, err))
		}
		if !jsonOut {
			_, _ = fmt.Fprintf(stdout, "certificate: %s\n", st.State)
		}
	}
	res, err := client.DomainCutover(ctx, domain)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("cutover %q: %w", domain, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printDomainCutover(stdout, res) })
}

func printDomainCutover(out io.Writer, r domaincutover.Result) {
	_, _ = fmt.Fprintf(out, "%s\n\n", r.Summary)
	for i, s := range r.Steps {
		eta := ""
		if s.ETASeconds > 0 {
			eta = fmt.Sprintf(" (about %d min)", (s.ETASeconds+59)/60)
		}
		_, _ = fmt.Fprintf(out, "%d. [%s] %s%s\n   %s\n", i+1, s.State, s.Title, eta, s.Detail)
		if s.Action != nil && s.Action.CLI != "" {
			_, _ = fmt.Fprintf(out, "   run: %s\n", s.Action.CLI)
		}
	}
	for _, rec := range r.DNS.Records {
		_, _ = fmt.Fprintf(out, "\nrecord: %s %s %s (TTL %d)\n", rec.Type, rec.Name, rec.Value, rec.TTLSeconds)
	}
	if r.Proxied {
		_, _ = fmt.Fprintln(out, "\nCloudflare proxies this name. The cutover works with the proxy on or off: see /cdn-and-caching.")
	}
	switch r.Risk {
	case domaincutover.RiskNone:
		_, _ = fmt.Fprintln(out, "\ndowntime risk: none expected")
	case domaincutover.RiskLikely:
		_, _ = fmt.Fprintln(out, "\ndowntime risk: likely, fix steps 1 and 2 before switching DNS")
	default:
		_, _ = fmt.Fprintln(out, "\ndowntime risk: unknown until the app is probed")
	}
}

func domainsCutoverUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains cutover <domain> [--get-certificate] [--revert-challenge] [flags]

Test before you switch: probes this server directly for <domain> (DNS is not
involved), shows whether the app answers and a trusted certificate is ready,
what DNS points at now, the exact record to change and each step's status.
No downtime is expected only when the app answers and the certificate is ready.

--get-certificate requests the certificate through your connected DNS provider
(DNS-01) for this domain only so it exists before you change DNS. Other
domains keep their challenge. After the switch, --revert-challenge puts this
domain back on HTTP-01.

Flags:
  --get-certificate        request the certificate before DNS is switched
  --revert-challenge       use HTTP-01 for this domain again
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the checklist as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
