package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func emailUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s email tokens list                                  every token a template can use
  %[1]s email templates list                               every email the platform sends
  %[1]s email templates get <key>                          one template with its default
  %[1]s email templates set <key> [--subject S] [--preheader P] [--body B | --body-file F]
  %[1]s email templates preview <key> [--scheme light|dark]   render with sample data
  %[1]s email templates reset <key>                        back to the built-in default
  %[1]s email groups list                                  saved audiences with live counts
  %[1]s email groups create <name> [--all] [--role R] [--team T] [--project P] [--domain D] [--inactive-days N] [--add ADDR] [--exclude ADDR]
  %[1]s email groups show <group>                          rules, count and a masked first-20 preview
  %[1]s email groups delete <group>
  %[1]s email settings get|set [flags]                     delivery provider, sender, limits and security
  %[1]s email test [--to ADDRESS]                          send one real test email and show the provider response
  %[1]s email check-domain [--domain D]                    check SPF, DKIM and DMARC for the sender domain
  %[1]s email log [--status failed] [--provider P] [--kind K] [--q TEXT] [--limit N]   recent deliveries
  %[1]s email send --group G [--to ADDR] --template T [--subject S] [--body B | --body-file F] [--at TIME] [--dry-run] [--yes]

Tokens use {{name}} and are checked when a template is saved. Announcements
skip anyone who unsubscribed; security and account email always arrives.
`, prog)
}

// runEmail dispatches "email <group> <verb>".
func runEmail(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, emailUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, emailUsage(prog))
		return exitOK
	case "tokens":
		return runEmailTokens(prog, rest, stdout, stderr, lookupEnv)
	case "templates":
		return runEmailTemplates(prog, rest, stdout, stderr, lookupEnv)
	case "groups":
		return runEmailGroups(prog, rest, stdout, stderr, lookupEnv)
	case "settings":
		return runEmailSettings(prog, rest, stdout, stderr, lookupEnv)
	case "test":
		return runEmailTest(prog, rest, stdout, stderr, lookupEnv)
	case "check-domain":
		return runEmailCheckDomain(prog, rest, stdout, stderr, lookupEnv)
	case "log":
		return runEmailLog(prog, rest, stdout, stderr, lookupEnv)
	case "send":
		return runEmailSend(prog, rest, stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown email subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, emailUsage(prog))
		return exitUsage
	}
}

func runEmailTokens(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] != "list" {
		_, _ = fmt.Fprint(stderr, emailUsage(prog))
		return exitUsage
	}
	c, code, ok := parseDBAccess(prog, "email tokens list", "email tokens list [flags]", args[1:], 0, "", stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	toks, err := c.client.ListMailTokens(context.Background())
	if err != nil {
		return c.fail(fmt.Errorf("list tokens: %w", err))
	}
	return c.result(toks, func() {
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "TOKEN\tSCOPE\tSAMPLE\tMEANING")
		for _, t := range toks {
			_, _ = fmt.Fprintf(tw, "{{%s}}\t%s\t%s\t%s\n", t.Name, t.Scope, t.Sample, t.Description)
		}
		_ = tw.Flush()
	})
}

func runEmailTemplates(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, emailUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	ctx := context.Background()
	switch args[0] {
	case "list":
		c, code, ok := parseDBAccess(prog, "email templates list", "email templates list [flags]", rest, 0, "", stdout, stderr, lookupEnv, nil)
		if !ok {
			return code
		}
		list, err := c.client.ListMailTemplates(ctx)
		if err != nil {
			return c.fail(fmt.Errorf("list templates: %w", err))
		}
		return c.result(list, func() {
			tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "KEY\tKIND\tCUSTOMIZED\tSUBJECT")
			for _, t := range list {
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%v\t%s\n", t.Key, t.Category, t.Customized, t.Current.Subject)
			}
			_ = tw.Flush()
		})
	case "get":
		c, code, ok := parseDBAccess(prog, "email templates get", "email templates get <key> [flags]", rest, 1, "a template key", stdout, stderr, lookupEnv, nil)
		if !ok {
			return code
		}
		t, err := c.client.GetMailTemplate(ctx, c.rest[0])
		if err != nil {
			return c.fail(fmt.Errorf("get template %q: %w", c.rest[0], err))
		}
		return c.result(t, func() { printMailTemplate(stdout, t) })
	case "set":
		return runEmailTemplateSet(prog, rest, stdout, stderr, lookupEnv)
	case "preview":
		var scheme string
		c, code, ok := parseDBAccess(prog, "email templates preview", "email templates preview <key> [--scheme light|dark] [flags]", rest, 1, "a template key", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
			fs.StringVar(&scheme, "scheme", "", "force light or dark colors in the HTML")
		})
		if !ok {
			return code
		}
		p, err := c.client.PreviewMailTemplate(ctx, c.rest[0], scheme)
		if err != nil {
			return c.fail(fmt.Errorf("preview template %q: %w", c.rest[0], err))
		}
		return c.result(p, func() { _, _ = fmt.Fprintf(stdout, "Subject: %s\n\n%s\n", p.Subject, p.Text) })
	case "reset":
		c, code, ok := parseDBAccess(prog, "email templates reset", "email templates reset <key> [flags]", rest, 1, "a template key", stdout, stderr, lookupEnv, nil)
		if !ok {
			return code
		}
		t, err := c.client.ResetMailTemplate(ctx, c.rest[0])
		if err != nil {
			return c.fail(fmt.Errorf("reset template %q: %w", c.rest[0], err))
		}
		return c.result(t, func() { _, _ = fmt.Fprintf(stdout, "template %s is back to its default\n", t.Key) })
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown email templates subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, emailUsage(prog))
		return exitUsage
	}
}

func printMailTemplate(out io.Writer, t apiclient.MailTemplate) {
	_, _ = fmt.Fprintf(out, "key:        %s (%s)\ncustomized: %v\ntokens:     %s\n\nSubject:   %s\nPreheader: %s\n\n%s\n",
		t.Key, t.Category, t.Customized, strings.Join(t.Tokens, ", "), t.Current.Subject, t.Current.Preheader, t.Current.Body)
}

func readBodyFlag(body, file string) (string, error) {
	if file == "" {
		return body, nil
	}
	b, err := os.ReadFile(file) //nolint:gosec // operator-chosen template file
	if err != nil {
		return "", fmt.Errorf("read %s: %w", file, err)
	}
	return string(b), nil
}

func runEmailTemplateSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var subject, preheader, body, bodyFile string
	c, code, ok := parseDBAccess(prog, "email templates set", "email templates set <key> [--subject S] [--preheader P] [--body B | --body-file F] [flags]", args, 1, "a template key", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&subject, "subject", "", "new subject (unset keeps the current one)")
		fs.StringVar(&preheader, "preheader", "", "new preheader (the preview line inbox lists show)")
		fs.StringVar(&body, "body", "", "new body in the small template markup")
		fs.StringVar(&bodyFile, "body-file", "", "read the new body from a file")
	})
	if !ok {
		return code
	}
	text, err := readBodyFlag(body, bodyFile)
	if err != nil {
		return c.fail(err)
	}
	ctx := context.Background()
	cur, err := c.client.GetMailTemplate(ctx, c.rest[0])
	if err != nil {
		return c.fail(fmt.Errorf("get template %q: %w", c.rest[0], err))
	}
	next := cur.Current
	if subject != "" {
		next.Subject = subject
	}
	if preheader != "" {
		next.Preheader = preheader
	}
	if text != "" {
		next.Body = text
	}
	saved, err := c.client.SetMailTemplate(ctx, c.rest[0], next)
	if err != nil {
		return c.fail(fmt.Errorf("set template %q: %w", c.rest[0], err))
	}
	return c.result(saved, func() { _, _ = fmt.Fprintf(stdout, "template %s saved\n", saved.Key) })
}

func runEmailGroups(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, emailUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	ctx := context.Background()
	switch args[0] {
	case "list":
		c, code, ok := parseDBAccess(prog, "email groups list", "email groups list [flags]", rest, 0, "", stdout, stderr, lookupEnv, nil)
		if !ok {
			return code
		}
		groups, err := c.client.ListMailGroups(ctx)
		if err != nil {
			return c.fail(fmt.Errorf("list groups: %w", err))
		}
		return c.result(groups, func() {
			tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "NAME\tID\tRECIPIENTS")
			for _, g := range groups {
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", g.Name, g.ID, apiclient.MailGroupCountLabel(g))
			}
			_ = tw.Flush()
		})
	case "create":
		return runEmailGroupCreate(prog, rest, stdout, stderr, lookupEnv)
	case "show":
		c, code, ok := parseDBAccess(prog, "email groups show", "email groups show <group> [flags]", rest, 1, "a group id or name", stdout, stderr, lookupEnv, nil)
		if !ok {
			return code
		}
		d, err := c.client.GetMailGroup(ctx, c.rest[0])
		if err != nil {
			return c.fail(fmt.Errorf("show group %q: %w", c.rest[0], err))
		}
		return c.result(d, func() {
			_, _ = fmt.Fprintf(stdout, "%s (%s)\nrecipients: %s\n", d.Group.Name, d.Group.ID, apiclient.MailGroupCountLabel(d.Group))
			for _, s := range d.Preview.Sample {
				_, _ = fmt.Fprintf(stdout, "  %s\n", s)
			}
		})
	case "delete":
		c, code, ok := parseDBAccess(prog, "email groups delete", "email groups delete <group> [flags]", rest, 1, "a group id or name", stdout, stderr, lookupEnv, nil)
		if !ok {
			return code
		}
		if err := c.client.DeleteMailGroup(ctx, c.rest[0]); err != nil {
			return c.fail(fmt.Errorf("delete group %q: %w", c.rest[0], err))
		}
		return c.result(map[string]string{"deleted": c.rest[0]}, func() { _, _ = fmt.Fprintf(stdout, "group %s deleted\n", c.rest[0]) })
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown email groups subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, emailUsage(prog))
		return exitUsage
	}
}

func runEmailGroupCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var all bool
	var inactive int
	var roles, teams, projects, domains, add, exclude stringList
	var description string
	c, code, ok := parseDBAccess(prog, "email groups create", "email groups create <name> [flags]", args, 1, "a group name", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.BoolVar(&all, "all", false, "every user")
		fs.Var(&roles, "role", "owner, admin, operator or viewer (repeatable)")
		fs.Var(&teams, "team", "team id (repeatable)")
		fs.Var(&projects, "project", "project id, selects its team (repeatable)")
		fs.Var(&domains, "domain", "sign-in email domain (repeatable)")
		fs.IntVar(&inactive, "inactive-days", 0, "only users who have not signed in for this many days")
		fs.Var(&add, "add", "extra address (repeatable)")
		fs.Var(&exclude, "exclude", "address to leave out (repeatable)")
		fs.StringVar(&description, "description", "", "what the group is for")
	})
	if !ok {
		return code
	}
	g, err := c.client.CreateMailGroup(context.Background(), c.rest[0], description, apiclient.MailRules{
		AllUsers: all, Roles: roles, Teams: teams, Projects: projects, EmailDomains: domains,
		InactiveDays: inactive, ExtraAddresses: add, ExcludeAddresses: exclude,
	})
	if err != nil {
		return c.fail(fmt.Errorf("create group %q: %w", c.rest[0], err))
	}
	return c.result(g, func() {
		_, _ = fmt.Fprintf(stdout, "group %s created (%s), %s\n", g.Name, g.ID, apiclient.MailGroupCountLabel(g))
	})
}

func runEmailSend(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var group, template, subject, body, bodyFile, at string
	var to stringList
	var dryRun, yes bool
	c, code, ok := parseDBAccess(prog, "email send", "email send --group G --template T [flags]", args, 0, "", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&group, "group", "", "audience group id or name")
		fs.Var(&to, "to", "extra recipient address (repeatable)")
		fs.StringVar(&template, "template", "announcement", "announcement template to start from")
		fs.StringVar(&subject, "subject", "", "subject (default: the template's)")
		fs.StringVar(&body, "body", "", "body (default: the template's)")
		fs.StringVar(&bodyFile, "body-file", "", "read the body from a file")
		fs.StringVar(&at, "at", "", "send later, RFC 3339 time")
		fs.BoolVar(&dryRun, "dry-run", false, "show who would receive it and send nothing")
		fs.BoolVar(&yes, "yes", false, "send without asking")
	})
	if !ok {
		return code
	}
	if group == "" && len(to) == 0 {
		_, _ = fmt.Fprintf(stderr, "%s: email send needs --group or --to\n", prog)
		return exitUsage
	}
	ctx := context.Background()
	tpl, err := c.client.GetMailTemplate(ctx, template)
	if err != nil {
		return c.fail(fmt.Errorf("get template %q: %w", template, err))
	}
	text, err := readBodyFlag(body, bodyFile)
	if err != nil {
		return c.fail(err)
	}
	req := apiclient.MailBroadcastRequest{Subject: tpl.Current.Subject, Body: tpl.Current.Body, TemplateKey: template, Audience: group, Recipients: to, ScheduleAt: at}
	if subject != "" {
		req.Subject = subject
	}
	if text != "" {
		req.Body = text
	}
	plan, err := c.client.PlanMailBroadcast(ctx, req)
	if err != nil {
		return c.fail(fmt.Errorf("plan broadcast: %w", err))
	}
	summary := fmt.Sprintf("This will email %d people, %d are suppressed.", plan.Deliverable, plan.Suppressed)
	if dryRun {
		return c.result(plan, func() { _, _ = fmt.Fprintln(stdout, summary) })
	}
	if !plan.ProviderConfigured {
		_, _ = fmt.Fprintf(stderr, "%s: no email provider is configured; run \"%s settings email set\" or open Settings, Email in the dashboard\n", prog, prog)
		return exitUsage
	}
	if !yes {
		_, _ = fmt.Fprintf(stderr, "%s\nSend now? [y/N] ", summary)
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
			_, _ = fmt.Fprintln(stderr, "aborted")
			return exitUsage
		}
	}
	req.ExpectedSend = plan.Deliverable
	b, err := c.client.CreateMailBroadcast(ctx, req)
	if err != nil {
		return c.fail(fmt.Errorf("send broadcast: %w", err))
	}
	return c.result(b, func() {
		_, _ = fmt.Fprintf(stdout, "broadcast %s is %s: %d recipients\n", b.ID, b.Status, b.Total-b.Suppressed)
	})
}
