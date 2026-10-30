package main

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const mailSystemEmailUsageLines = `  %[1]s mail system-email status            show whether system emails go through your mail server
  %[1]s mail system-email enable <domain> [--to ADDR] [--port 587|465] [--replace-existing]   send invites, resets and alerts as noreply@<domain> (asks your password)
  %[1]s mail system-email disable            put the previous email provider back and suspend noreply (asks your password)
  %[1]s mail system-email rotate             set a new noreply password; it is sealed, never shown (asks your password)
  %[1]s mail system-email test [--to ADDR]   send a test email through your mail server
`

// runMailSystemEmail handles "mail system-email".
func runMailSystemEmail(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintf(stderr, "%s: mail system-email needs a subcommand\n\n%s", prog, mailUsage(prog))
		return 2
	}
	sub, rest := args[0], args[1:]
	label := "mail system-email " + sub
	ctx := context.Background()
	var to string
	var port int
	var replace bool
	toFlag := func(fs *flag.FlagSet) {
		fs.StringVar(&to, "to", "", "test recipient (default: your own account email)")
	}
	switch sub {
	case "status":
		c, code, ok := parseDBAccess(prog, label, label, rest, 0, "", stdout, stderr, lookupEnv, nil)
		if !ok {
			return code
		}
		st, err := c.client.SystemEmail(ctx)
		if err != nil {
			return c.fail(fmt.Errorf("%s: %w", label, err))
		}
		return c.result(st, func() { printSystemEmailState(stdout, st) })
	case "enable":
		c, code, ok := parseDBAccess(prog, label, label+" <domain> [--to ADDR] [--port 587|465] [--replace-existing]", rest, 1, "a domain", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
			toFlag(fs)
			fs.IntVar(&port, "port", 587, "submission port on the mail server: 587 (STARTTLS) or 465 (implicit TLS)")
			fs.BoolVar(&replace, "replace-existing", false, "replace the email provider that is configured now; disable puts it back")
		})
		if !ok {
			return code
		}
		res, err := withStepUp(stderr, func(p *apiclient.StepUpProof) (apiclient.SystemEmailResult, error) {
			return c.client.EnableSystemEmail(ctx, apiclient.SystemEmailEnableRequest{Domain: c.rest[0], To: to, Port: port, ReplaceExisting: replace, StepUp: p})
		})
		if err != nil {
			return c.fail(fmt.Errorf("%s: %w", label, err))
		}
		return c.result(res, func() { printSystemEmailResult(stdout, res) })
	case "disable", "rotate":
		c, code, ok := parseDBAccess(prog, label, label, rest, 0, "", stdout, stderr, lookupEnv, nil)
		if !ok {
			return code
		}
		res, err := withStepUp(stderr, func(p *apiclient.StepUpProof) (apiclient.SystemEmailResult, error) {
			return c.client.SystemEmailAction(ctx, sub, "", p)
		})
		if err != nil {
			return c.fail(fmt.Errorf("%s: %w", label, err))
		}
		return c.result(res, func() { printSystemEmailResult(stdout, res) })
	case "test":
		c, code, ok := parseDBAccess(prog, label, label+" [--to ADDR]", rest, 0, "", stdout, stderr, lookupEnv, toFlag)
		if !ok {
			return code
		}
		res, err := c.client.SystemEmailAction(ctx, "test", to, nil)
		if err != nil {
			return c.fail(fmt.Errorf("%s: %w", label, err))
		}
		return c.result(res, func() { printSystemEmailResult(stdout, res) })
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown subcommand %q\n\n%s", prog, label, mailUsage(prog))
	return 2
}

func printSystemEmailState(out io.Writer, st apiclient.SystemEmailState) {
	_, _ = fmt.Fprintf(out, "enabled: %t\nactive: %t\n", st.Enabled, st.Active)
	if st.Domain != "" {
		_, _ = fmt.Fprintf(out, "domain: %s\nfrom: %s\nsubmission: %s (%s, certificate pinned: %t)\n", st.Domain, st.FromAddress, st.Submission, st.TLSMode, st.Pinned)
	}
	if st.LastTest != nil {
		_, _ = fmt.Fprintf(out, "last test: ok=%t to %s: %s\n", st.LastTest.OK, st.LastTest.To, st.LastTest.Message)
	}
	if st.DNS != nil {
		_, _ = fmt.Fprintf(out, "dns: spf=%s dkim=%s dmarc=%s\n", st.DNS.SPF, st.DNS.DKIM, st.DNS.DMARC)
	}
	if st.Warning != "" {
		_, _ = fmt.Fprintf(out, "warning: %s\n", st.Warning)
	}
}

func printSystemEmailResult(out io.Writer, res apiclient.SystemEmailResult) {
	for _, s := range res.Steps {
		mark := "ok"
		if !s.OK {
			mark = "FAILED"
		}
		_, _ = fmt.Fprintf(out, "%s: %s: %s\n", s.ID, mark, s.Message)
	}
	printSystemEmailState(out, res.State)
}
