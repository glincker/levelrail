package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func runEmailSettings(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, settingsEmailUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "get":
		return runSettingsEmailGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runSettingsEmailSet(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		return runSettingsEmail(prog, args, stdout, stderr, lookupEnv)
	}
}

// emailFixHints maps an API failure code from a test send to the next step an operator should take.
var emailFixHints = map[string]string{
	"auth_failed":         "the provider rejected the credentials: re-enter the password or API key (SES SMTP credentials are not the same as IAM access keys)",
	"sender_not_verified": "the provider does not know this sender: verify the from address or domain in the provider console",
	"region_mismatch":     "the sender is verified in a different region: pick the region where the identity is verified",
	"port_blocked":        "the connection to the mail server failed: check the host, port and any firewall (many clouds block port 25)",
	"tls_error":           "the TLS handshake failed: check the TLS mode for this port, or turn off certificate verification only for a trusted private server",
	"rate_limited":        "the provider or the configured send cap is rate limiting: wait and retry, or raise the cap",
	"not_configured":      "no provider is configured: run settings set first",
}

func runEmailTest(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "email test", "print the result as JSON to stdout and nothing else", stderr)
	var to string
	fs.StringVar(&to, "to", "", "recipient address (default: the signed-in user's own address)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s email test [--to ADDRESS]\n\nSends one real email through the same path alerts and password resets use.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	res, err := client.SendTestEmail(context.Background(), to)
	if err != nil {
		var apiErr *apiclient.APIError
		if errors.As(err, &apiErr) {
			if hint, found := emailFixHints[apiErr.Code]; found {
				err = fmt.Errorf("%w\nFix: %s", err, hint)
			}
		}
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("send test email: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		_, _ = fmt.Fprintf(stdout, "sent to %s via %s (attempts: %d)\n", res.To, res.Provider, res.Attempts)
		if res.MessageID != "" {
			_, _ = fmt.Fprintf(stdout, "provider message id: %s\n", res.MessageID)
		}
	})
}

func runEmailCheckDomain(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "email check-domain", "print the checks as JSON to stdout and nothing else", stderr)
	var domain string
	fs.StringVar(&domain, "domain", "", "domain to check (default: the from address's domain)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s email check-domain [--domain DOMAIN]\n\nLooks up SPF, DKIM and DMARC through the control plane's own resolver.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	res, err := client.CheckEmailDomain(context.Background(), domain)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("check email domain: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		_, _ = fmt.Fprintf(stdout, "domain: %s (provider: %s)\n", res.Domain, res.Provider)
		for _, r := range res.Records {
			_, _ = fmt.Fprintf(stdout, "%-5s %-5s %s\n", strings.ToUpper(r.Type), r.Status, r.Message)
		}
	})
}

func runEmailLog(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "email log", "print the deliveries as JSON to stdout and nothing else", stderr)
	var q apiclient.EmailDeliveryQuery
	fs.StringVar(&q.Status, "status", "", "only deliveries with this status: sent, failed or bounced")
	fs.StringVar(&q.Provider, "provider", "", "only deliveries through this provider")
	fs.StringVar(&q.Kind, "kind", "", "only deliveries of this kind")
	fs.StringVar(&q.Query, "q", "", "search recipient, message id or error text")
	fs.IntVar(&q.Limit, "limit", 50, "how many deliveries to list (max 200)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s email log [--status failed] [--provider P] [--kind K] [--q TEXT] [--limit N]\n\nLists recent email deliveries, newest first.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	res, err := client.ListEmailDeliveries(context.Background(), q)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list email deliveries: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res.Deliveries, func() {
		if len(res.Deliveries) == 0 {
			_, _ = fmt.Fprintln(stdout, "no deliveries")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "TIME\tSTATUS\tPROVIDER\tTO\tKIND\tDETAIL")
		for _, d := range res.Deliveries {
			detail := d.MessageID
			if d.Status != "sent" {
				detail = d.ErrorKind
				if detail == "" {
					detail = d.Error
				}
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", d.CreatedAt.Local().Format("2006-01-02 15:04:05"), d.Status, d.Provider, d.To, d.Kind, detail)
		}
		_ = tw.Flush()
	})
}
