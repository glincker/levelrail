package main

import (
	"context"
	"fmt"
	"io"
	"time"
)

const httpsWaitPoll = 3 * time.Second

// runSettingsIngressHTTPS implements "settings ingress https [status|enable]":
// the CLI counterpart of the dashboard's one-click Enable HTTPS card.
func runSettingsIngressHTTPS(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	verb := "status"
	if len(args) > 0 && (args[0] == "status" || args[0] == "enable") {
		verb, args = args[0], args[1:]
	}
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "settings ingress https "+verb, "print the HTTPS status as JSON to stdout and nothing else", stderr)
	var email string
	var staging bool
	var wait time.Duration
	if verb == "enable" {
		fs.StringVar(&email, "email", "", "ACME account contact address (required)")
		fs.BoolVar(&staging, "staging", false, "use Let's Encrypt staging: untrusted certificate, but no real rate limits")
		fs.DurationVar(&wait, "wait", 0, "wait up to this long for the certificate (for example 2m)")
	}
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s settings ingress https status [flags]\n  %s settings ingress https enable --email EMAIL [--staging] [--wait 2m] [flags]\n\nEnable points the dashboard at <dashed-ip>.sslip.io and issues a real\nLet's Encrypt certificate for it, with no DNS setup.\n\nFlags:\n", prog, prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()

	var status httpsStatusResource
	var err error
	if verb == "enable" {
		if email == "" {
			_, _ = fmt.Fprintf(stderr, "%s: --email is required\n", prog)
			return exitUsage
		}
		status, err = client.EnableHTTPS(ctx, enableHTTPSRequest{Email: email, Staging: staging})
		if err == nil && wait > 0 {
			status, err = waitForHTTPS(ctx, client, status, wait)
		}
	} else {
		status, err = client.GetHTTPSStatus(ctx)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("https %s: %w", verb, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, status, func() { printHTTPSStatusHuman(stdout, status) })
}

func waitForHTTPS(ctx context.Context, client *Client, status httpsStatusResource, wait time.Duration) (httpsStatusResource, error) {
	deadline := time.Now().Add(wait)
	for status.State == "pending" && time.Now().Before(deadline) {
		time.Sleep(httpsWaitPoll)
		next, err := client.GetHTTPSStatus(ctx)
		if err != nil {
			return status, err
		}
		status = next
	}
	return status, nil
}

func printHTTPSStatusHuman(out io.Writer, s httpsStatusResource) {
	_, _ = fmt.Fprintf(out, "state:      %s\n", s.State)
	if s.Domain != "" {
		_, _ = fmt.Fprintf(out, "domain:     %s\n", s.Domain)
	}
	if s.State == "off" && s.SuggestedDomain != "" {
		_, _ = fmt.Fprintf(out, "suggested:  %s (run: settings ingress https enable --email you@example.com)\n", s.SuggestedDomain)
	}
	if s.State == "issued" {
		_, _ = fmt.Fprintf(out, "issuer:     %s\n", s.Issuer)
		if s.NotAfter != nil {
			_, _ = fmt.Fprintf(out, "expires:    %s\n", s.NotAfter.Format(time.RFC3339))
		}
		if s.Staging {
			_, _ = fmt.Fprintln(out, "note:       staging certificate, browsers will not trust it")
		} else {
			_, _ = fmt.Fprintf(out, "dashboard:  https://%s\n", s.Domain)
		}
	}
	if s.Error != "" {
		_, _ = fmt.Fprintf(out, "error:      %s\n", s.Error)
		_, _ = fmt.Fprintf(out, "hint:       %s\n", httpsHintText(s.Hint))
	}
}

func httpsHintText(hint string) string {
	switch hint {
	case "rate_limited":
		return "Let's Encrypt rate limit hit. Wait, or try --staging while debugging."
	case "unreachable":
		return "the CA could not reach port 80 on this server. Open ports 80 and 443 in the firewall and cloud security group."
	case "dns":
		return "the hostname did not resolve to this server. Check that the public IP is correct."
	case "caa":
		return "a CAA record forbids this CA from issuing for the hostname."
	}
	return "see the control plane log for the full CA response."
}
