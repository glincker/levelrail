package main

import (
	"context"
	"fmt"
	"io"
)

func runCerts(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] != "budget" {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s certs budget [flags]\n", prog)
		return exitUsage
	}
	label := "certs budget"
	usage := fmt.Sprintf("Usage:\n  %s %s [flags]\n\nCertificates issued by the public CA in the last 7 days, against the warning level in the runtime setting certificates.weekly_issue_warn. Exits 0 either way; it only informs.\n\nFlags:\n%s", prog, label, commonFlagsHelp)
	c, code, ok := parseCLICall(prog, label, usage, args[1:], stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	b, err := c.client.GetCertBudget(context.Background())
	if err != nil {
		return c.fail(fmt.Errorf("certificate budget: %w", err))
	}
	return c.render(b, func() {
		_, _ = fmt.Fprintf(stdout, "%d certificates issued in the last %d days (warning at %d)\n", b.Issued, b.WindowDays, b.Warn)
		if b.Over {
			_, _ = fmt.Fprintf(stdout, "over the warning level; opt apps out with: %s apps auto-host <app> --off\n", prog)
		}
	})
}
