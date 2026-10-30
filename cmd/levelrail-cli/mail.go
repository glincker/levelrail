package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/mailrecords"
)

func mailUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s mail status                       show whether the mail server is installed, running and ready
%[3]s  %[1]s mail dns plan <domain> [flags]    dry run: diff the mail records against the live zone
  %[1]s mail dns apply <domain> --plan-hash H --confirm [flags]   write a plan you reviewed
  %[1]s mail dns check <domain> [--host H] [--dkim-selector S]    grade the live mail DNS

Plan and apply flags (same values on both, or the plan hash will not match):
  --host H  --ipv4 IP  --ipv6 IP  --mx-priority N  --preset ID
  --spf-mode server|include  --spf-include HOSTS  --dkim-selector S  --dkim-key-file F
  --dmarc-policy none|quarantine|reject  --dmarc-rua ADDR  --dmarc-pct N
  --mta-sts testing|enforce  --tls-rpt  --autoconfig  --replace-existing KINDS
  --provider P  --plan-hash H  --confirm

Experimental: needs APP_EXPERIMENTAL=mail. The default is add-only: existing
MX, SPF, DKIM and DMARC records are never replaced unless --replace-existing
lists their kind (MX, SPF, DKIM, DMARC, HOST, MTASTS, TLSRPT, AUTOCONFIG).
Presets: %[2]s.
`, prog, strings.Join(mailrecords.PresetIDs(), ", "), fmt.Sprintf(mailPortsUsageLines, prog))
}

type mailFlags struct {
	req                  apiclient.MailDNSRequest
	provider, keyFile    string
	spfIncludes, replace string
	confirm              bool
}

func (m *mailFlags) register(fs *flag.FlagSet, apply bool) {
	s := &m.req.Spec
	fs.StringVar(&s.MailHost, "host", "", "mail host name (default mail.<domain>)")
	fs.StringVar(&s.IPv4, "ipv4", "", "public IPv4 of the mail host")
	fs.StringVar(&s.IPv6, "ipv6", "", "public IPv6 of the mail host")
	fs.IntVar(&s.MXPriority, "mx-priority", 0, "MX priority (default 10)")
	fs.StringVar(&s.Preset, "preset", "", "external provider preset")
	fs.StringVar(&s.SPFMode, "spf-mode", "", "server (mx and the IPs) or include (external senders)")
	fs.StringVar(&m.spfIncludes, "spf-include", "", "comma separated SPF include hosts")
	fs.StringVar(&s.DKIMSelector, "dkim-selector", "", "DKIM selector")
	fs.StringVar(&m.keyFile, "dkim-key-file", "", "file holding the DKIM public key (never the private key)")
	fs.StringVar(&s.DMARCPolicy, "dmarc-policy", "", "none, quarantine or reject (default none)")
	fs.StringVar(&s.DMARCRua, "dmarc-rua", "", "DMARC report address (default dmarc@<domain>)")
	fs.IntVar(&s.DMARCPct, "dmarc-pct", 0, "percent of mail the DMARC policy applies to (default 100)")
	fs.StringVar(&s.MTASTSMode, "mta-sts", "", "testing or enforce: also build the MTA-STS record")
	fs.BoolVar(&s.TLSRPT, "tls-rpt", false, "also build the TLS-RPT record")
	fs.BoolVar(&s.Autoconfig, "autoconfig", false, "also build autoconfig, autodiscover and SRV records")
	fs.StringVar(&m.replace, "replace-existing", "", "comma separated record kinds that may be replaced")
	fs.StringVar(&m.provider, "provider", "", "DNS provider when more than one is connected")
	if apply {
		fs.StringVar(&m.req.PlanHash, "plan-hash", "", "hash printed by plan")
		fs.BoolVar(&m.confirm, "confirm", false, "write the records")
	}
}

func (m *mailFlags) finish(domain string) error {
	m.req.Domain = domain
	m.req.SPFIncludes = splitList(m.spfIncludes)
	m.req.ReplaceExisting = splitList(m.replace)
	if m.keyFile != "" {
		b, err := os.ReadFile(m.keyFile)
		if err != nil {
			return fmt.Errorf("read dkim key file: %w", err)
		}
		m.req.DKIMPublicKey = string(b)
	}
	return nil
}

func runMail(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) >= 1 && args[0] == "ports" {
		return runMailPorts(prog, args[1:], stdout, stderr, lookupEnv)
	}
	if len(args) >= 1 && args[0] == "status" {
		var app string
		c, code, ok := parseDBAccess(prog, "mail status", "mail status [--app name]", args[1:], 0, "", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
			fs.StringVar(&app, "app", "", "mail app to report on when several are installed")
		})
		if !ok {
			return code
		}
		st, err := c.client.MailStatus(context.Background(), app)
		if err != nil {
			return c.fail(fmt.Errorf("mail status: %w", err))
		}
		return c.result(st, func() {
			_, _ = fmt.Fprintf(stdout, "app: %s\ninstalled: %t\ncontainer: %s\nadmin api: %t\ntls: %s\nnext: %s\n", st.App, st.Installed, st.ContainerState, st.AdminReachable, st.TLS, st.NextStep)
		})
	}
	if len(args) < 2 || args[0] != "dns" {
		_, _ = fmt.Fprint(stderr, mailUsage(prog))
		return exitUsage
	}
	rest := args[2:]
	switch args[1] {
	case "plan", "apply":
		apply := args[1] == "apply"
		var m mailFlags
		c, code, ok := parseDBAccess(prog, "mail dns "+args[1], "mail dns "+args[1]+" <domain> [flags]", rest, 1, "a domain", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) { m.register(fs, apply) })
		if !ok {
			return code
		}
		if err := m.finish(c.rest[0]); err != nil {
			return c.fail(err)
		}
		if !apply {
			res, err := c.client.PlanMailDNS(context.Background(), m.provider, m.req)
			if err != nil {
				return c.fail(fmt.Errorf("plan mail dns %s: %w", c.rest[0], err))
			}
			return c.result(res, func() { printMailPlan(stdout, res) })
		}
		if !m.confirm || m.req.PlanHash == "" {
			return c.fail(errors.New("apply needs --plan-hash from a plan you reviewed and --confirm"))
		}
		m.req.Confirm = true
		res, err := c.client.ApplyMailDNS(context.Background(), m.provider, m.req)
		if err != nil {
			return c.fail(fmt.Errorf("apply mail dns %s: %w", c.rest[0], err))
		}
		return c.result(res, func() { _, _ = fmt.Fprintf(stdout, "applied %d record sets for %s\n", res.Applied, c.rest[0]) })
	case "check":
		var host, sel string
		c, code, ok := parseDBAccess(prog, "mail dns check", "mail dns check <domain> [--host H] [--dkim-selector S]", rest, 1, "a domain", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
			fs.StringVar(&host, "host", "", "mail host name to expect")
			fs.StringVar(&sel, "dkim-selector", "", "DKIM selector to verify")
		})
		if !ok {
			return code
		}
		rep, err := c.client.CheckMailDNS(context.Background(), c.rest[0], host, sel)
		if err != nil {
			return c.fail(fmt.Errorf("check mail dns %s: %w", c.rest[0], err))
		}
		return c.result(rep, func() { printMailReport(stdout, rep) })
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown mail dns subcommand %q\n\n%s", prog, args[1], mailUsage(prog))
	return exitUsage
}

func printMailPlan(out io.Writer, p mailrecords.PlanResult) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ACTION\tKIND\tNAME\tTYPE\tVALUE")
	for _, c := range p.Changes {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", c.Action, c.Kind, c.Set.Name, c.Set.Type, strings.Join(c.Set.Values, " | "))
		for _, r := range c.Removed {
			_, _ = fmt.Fprintf(tw, "  removes\t\t\t\t%s\n", r)
		}
	}
	_ = tw.Flush()
	for _, r := range p.Refusals {
		_, _ = fmt.Fprintf(out, "refused: %s\n", r)
	}
	for _, n := range p.Notes {
		_, _ = fmt.Fprintf(out, "%s: %s\n", n.Level, n.Message)
	}
	if p.Blocked {
		_, _ = fmt.Fprintln(out, "plan is blocked; nothing can be applied until the refusals are resolved")
		return
	}
	_, _ = fmt.Fprintf(out, "plan hash: %s\n", p.Hash)
}

func printMailReport(out io.Writer, r mailrecords.Report) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "CHECK\tSTATUS\tDETAIL")
	for _, it := range r.Items {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", it.ID, it.Status, it.Message)
	}
	_ = tw.Flush()
}
