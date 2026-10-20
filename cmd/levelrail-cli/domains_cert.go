package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

var certWaitPoll = 3 * time.Second

const (
	certDefaultWait = 3 * time.Minute
	certPending     = "pending"
	certIssued      = "issued"
	certFailed      = "failed"
)

// certResult is the --json shape of "domains cert".
type certResult struct {
	Preflight apiclient.CertPreflight     `json:"preflight"`
	Status    *apiclient.DomainCertStatus `json:"status,omitempty"`
}

// runDomainsCert implements "domains cert <domain> [--staging] [--wait]": the
// CLI counterpart of the dashboard's Get a real certificate button.
func runDomainsCert(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "domains cert", "print the preflight and certificate state as JSON to stdout and nothing else", stderr)
	var email string
	var staging, wait, preflightOnly, ackPorts bool
	var timeout time.Duration
	fs.StringVar(&email, "email", "", "ACME account contact address (default: the one already set, else your admin account email)")
	fs.BoolVar(&staging, "staging", false, "use Let's Encrypt staging: untrusted certificate, but no real rate limits")
	fs.BoolVar(&wait, "wait", false, "wait until the certificate is issued or fails")
	fs.DurationVar(&timeout, "timeout", certDefaultWait, "how long --wait waits")
	fs.BoolVar(&preflightOnly, "preflight", false, "only run the checks, change nothing")
	fs.BoolVar(&ackPorts, "ports-open", false, "go ahead even if the port 80 self-check failed (you know it is open)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, domainsCertUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "domains cert", "a domain", 1)
	if !ok {
		return exitUsage
	}
	domain := strings.ToLower(rest[0])
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()

	pre, err := client.CertPreflight(ctx, domain, email, staging)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("certificate preflight for %q: %w", domain, err))
	}
	res := certResult{Preflight: pre}
	blocked := !pre.Ready && (!ackPorts || !onlyPortsFailed(pre))
	if preflightOnly || blocked {
		code := renderCertResult(stdout, stderr, of, res)
		if code == exitOK && blocked {
			return exitValidation
		}
		return code
	}
	req := apiclient.GetCertRequest{Email: email, Staging: staging}
	if ackPorts {
		req.Acknowledge = []string{"ports"}
	}
	st, err := client.GetCertificate(ctx, domain, req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get certificate for %q: %w", domain, err))
	}
	if wait {
		if st, err = waitForCert(ctx, client, domain, st, timeout); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("certificate status for %q: %w", domain, err))
		}
	}
	res.Status = &st
	code := renderCertResult(stdout, stderr, of, res)
	if code == exitOK && st.State == certFailed {
		return exitAPIError
	}
	return code
}

func onlyPortsFailed(p apiclient.CertPreflight) bool {
	for _, it := range p.Items {
		if it.State == "fail" && it.Code != "ports" {
			return false
		}
	}
	return true
}

func renderCertResult(stdout, stderr io.Writer, of outputFlags, res certResult) int {
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printCertResult(stdout, res) })
}

func waitForCert(ctx context.Context, client *Client, domain string, st apiclient.DomainCertStatus, timeout time.Duration) (apiclient.DomainCertStatus, error) {
	deadline := time.Now().Add(timeout)
	for st.State == certPending && time.Now().Before(deadline) {
		time.Sleep(certWaitPoll)
		next, err := client.DomainCertStatus(ctx, domain)
		if err != nil {
			return st, err
		}
		st = next
	}
	return st, nil
}

func printCertResult(out io.Writer, res certResult) {
	p := res.Preflight
	_, _ = fmt.Fprintf(out, "preflight for %s (contact %s)%s\n", p.Domain, orDash(p.Email), stagingNote(p.Staging))
	for _, it := range p.Items {
		_, _ = fmt.Fprintf(out, "  %-6s %-14s %s\n", it.State, it.Code, it.Detail)
		if it.Fix != "" && (it.State == "fail" || it.State == "warn") {
			_, _ = fmt.Fprintf(out, "         next: %s\n", it.Fix)
		}
	}
	if p.NextSafeAt != nil {
		_, _ = fmt.Fprintf(out, "next safe time: %s\n", p.NextSafeAt.Format(time.RFC3339))
	}
	if res.Status == nil {
		if !p.Ready {
			_, _ = fmt.Fprintln(out, "not ready: fix the failed items above, then run this again")
		}
		return
	}
	st := res.Status
	_, _ = fmt.Fprintf(out, "\ncertificate: %s\n", st.State)
	switch st.State {
	case certPending:
		_, _ = fmt.Fprintln(out, "the challenge is pending; check again with: domains cert "+st.Domain+" --wait")
	case certIssued:
		_, _ = fmt.Fprintf(out, "issuer:   %s (%s)\n", st.Issuer, st.Kind)
		if st.NotAfter != nil {
			_, _ = fmt.Fprintf(out, "expires:  %s\n", st.NotAfter.Format(time.RFC3339))
		}
		if st.RenewalAt != nil {
			_, _ = fmt.Fprintf(out, "renews:   from %s\n", st.RenewalAt.Format(time.RFC3339))
		}
		if st.Staging {
			_, _ = fmt.Fprintln(out, "note:     staging certificate, browsers will not trust it")
		}
	case certFailed:
		_, _ = fmt.Fprintf(out, "error:    %s\n", st.Explanation)
		_, _ = fmt.Fprintf(out, "CA said:  %s\n", st.Error)
		if st.NextSafeAt != nil {
			_, _ = fmt.Fprintf(out, "next safe time: %s\n", st.NextSafeAt.Format(time.RFC3339))
		}
	}
	if st.AppliesToAll {
		_, _ = fmt.Fprintln(out, "note:     public certificates are a server setting, so this turned them on for every domain")
	}
}

func stagingNote(staging bool) string {
	if staging {
		return ", staging CA (untrusted certificates)"
	}
	return ""
}

func domainsCertUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains cert <domain> [--staging] [--email EMAIL] [--wait] [--timeout 3m] [--preflight] [--ports-open] [flags]

Get a real Let's Encrypt certificate for a domain: runs the preflight (DNS,
port 80, CAA, contact email, rate limits), turns on public certificates and
asks for one. Public certificates are a server setting, so this applies to
every domain on the server. --staging rehearses against the test CA (the
certificate is untrusted by browsers). --preflight only runs the checks.
--wait waits up to --timeout (default 3m) for the result. --ports-open goes
ahead when only the port 80 self-check failed and you know the port is open.

Flags:
`, prog)
}
