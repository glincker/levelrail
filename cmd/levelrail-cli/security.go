package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runSecurity implements "security": the security center's posture,
// sessions and policy over /api/v1/security.
func runSecurity(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, securityUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, securityUsage(prog))
		return exitOK
	case "posture":
		return runSecurityPosture(prog, args[1:], stdout, stderr, lookupEnv)
	case "sessions":
		return runSecuritySessions(prog, args[1:], stdout, stderr, lookupEnv)
	case "policy":
		return runSecurityPolicy(prog, args[1:], stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown security subcommand %q\n\n", prog, args[0])
	_, _ = fmt.Fprint(stderr, securityUsage(prog))
	return exitUsage
}

func securityUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s security posture [flags]                         score and prioritised checklist (the item list needs root)
  %[1]s security sessions list [--user ID] [flags]       live sessions, trusted browsers and API tokens
  %[1]s security sessions revoke <id> [--user ID]        end one session
  %[1]s security sessions revoke-others [--user ID]      end every session but this one
  %[1]s security policy get [flags]                      show the security policy and where each value comes from
  %[1]s security policy set [policy flags] [flags]       change the policy (root)

A token used here must belong to a user and hold write:sensitive or
signin:approve for sessions; --user (another account) needs root.
`, prog)
}

// securityCommand parses the shared API flags plus any extra ones.
func securityCommand(prog, label, jsonHelp string, args []string, stderr io.Writer, lookupEnv func(string) (string, bool), extra func(*flag.FlagSet)) (*apiclient.Client, *flag.FlagSet, bool, outputFlags, int, bool) {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, label, jsonHelp, stderr)
	if extra != nil {
		extra(fs)
	}
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, securityUsage(prog))
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return nil, fs, false, of, exitCode, false
	}
	return apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv), fs, jsonOut, of, exitOK, true
}

func runSecurityPosture(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	client, _, jsonOut, of, code, ok := securityCommand(prog, "security posture", "print the posture as JSON to stdout and nothing else", args, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	p, err := client.GetSecurityPosture(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("security posture: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, p, func() { printPosture(stdout, prog, p) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printPosture(w io.Writer, prog string, p apiclient.SecurityPosture) {
	c := p.Counts
	_, _ = fmt.Fprintf(w, "Security score %d (%s): %d critical, %d high, %d medium, %d low failing; %d passing, %d unknown\n",
		p.Score, p.Grade, c.Critical, c.High, c.Medium, c.Low, c.Passing, c.Unknown)
	printPostureItems(w, prog, "Your account", p.Account)
	if !p.Full {
		_, _ = fmt.Fprintln(w, "The full checklist needs a root token or an admin session.")
		return
	}
	printPostureItems(w, prog, "Platform", p.Items)
}

func printPostureItems(w io.Writer, prog, heading string, items []apiclient.PostureItem) {
	if len(items) == 0 {
		return
	}
	_, _ = fmt.Fprintf(w, "\n%s:\n", heading)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "STATUS\tSEVERITY\tCHECK\tCOUNT\tFIX")
	for _, it := range items {
		fix := ""
		if it.Fix != nil {
			fix = it.Fix.Link
			if it.Fix.CLI != "" {
				fix = prog + " " + it.Fix.CLI
			}
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\n", it.Status, it.Severity, it.ID, it.Count, fix)
	}
	_ = tw.Flush()
}

func runSecuritySessions(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, securityUsage(prog))
		return exitUsage
	}
	sub, rest := args[0], args[1:]
	var user string
	withUser := func(fs *flag.FlagSet) {
		fs.StringVar(&user, "user", "", "act on another account (user ID, needs root)")
	}
	client, fs, jsonOut, of, code, ok := securityCommand(prog, "security sessions "+sub, "print the result as JSON to stdout and nothing else", rest, stderr, lookupEnv, withUser)
	if !ok {
		return code
	}
	ctx := context.Background()
	switch sub {
	case "list":
		s, err := client.ListSecuritySessions(ctx, user)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("list sessions: %w", err))
		}
		if err := renderResult(stdout, of.Format, of.Query, s, func() { printSessions(stdout, s) }); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitCodeForError(err)
		}
	case "revoke":
		if fs.NArg() != 1 {
			_, _ = fmt.Fprintf(stderr, "%s: security sessions revoke takes exactly one session id\n", prog)
			return exitUsage
		}
		if err := client.RevokeSecuritySession(ctx, fs.Arg(0), user); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("revoke session: %w", err))
		}
		_, _ = fmt.Fprintf(stdout, "revoked session %s\n", fs.Arg(0))
	case "revoke-others":
		out, err := client.RevokeOtherSecuritySessions(ctx, user)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("revoke other sessions: %w", err))
		}
		if err := renderResult(stdout, of.Format, of.Query, out, func() {
			_, _ = fmt.Fprintf(stdout, "revoked %d session(s); trusted browsers and waiting sign-ins were reset too\n", out.Revoked)
		}); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitCodeForError(err)
		}
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown security sessions subcommand %q\n\n", prog, sub)
		_, _ = fmt.Fprint(stderr, securityUsage(prog))
		return exitUsage
	}
	return exitOK
}

func ago(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return time.Since(t).Round(time.Minute).String() + " ago"
}

func printSessions(w io.Writer, s apiclient.SecuritySessions) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "SESSION\tBROWSER\tNETWORK\tCREATED\tLAST SEEN\tCURRENT")
	for _, x := range s.Sessions {
		cur := ""
		if x.Current {
			cur = "yes"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", x.ID, x.Browser, x.Network, ago(x.CreatedAt), ago(x.LastSeenAt), cur)
	}
	_ = tw.Flush()
	if len(s.TrustedDevices) > 0 {
		_, _ = fmt.Fprintln(w, "\nTrusted browsers:")
		for _, d := range s.TrustedDevices {
			_, _ = fmt.Fprintf(w, "  %s  %s  %s  last used %s\n", d.ID, d.Label, d.IP, ago(d.LastUsedAt))
		}
	}
	if len(s.Tokens) > 0 {
		_, _ = fmt.Fprintln(w, "\nAPI tokens:")
		for _, t := range s.Tokens {
			last := "never used"
			if t.LastUsedAt != nil {
				last = "last used " + ago(*t.LastUsedAt)
			}
			_, _ = fmt.Fprintf(w, "  %s  %s  %s  %s\n", t.ID, t.Name, strings.Join(t.Abilities, ","), last)
		}
	}
}

func runSecurityPolicy(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || (args[0] != "get" && args[0] != "set") {
		_, _ = fmt.Fprint(stderr, securityUsage(prog))
		return exitUsage
	}
	verb := args[0]
	var scope string
	var maxLife, warn, disable int
	extra := func(fs *flag.FlagSet) {
		if verb != "set" {
			return
		}
		fs.StringVar(&scope, "approval-scope", "", "password_only or all_methods: which sign-ins need new-browser approval")
		fs.IntVar(&maxLife, "max-token-lifetime-days", 0, "longest lifetime a new API token may have, 0 for no limit")
		fs.IntVar(&warn, "warn-unused-days", 0, "flag tokens unused this long in the posture, 0 to stop")
		fs.IntVar(&disable, "disable-unused-days", 0, "disable tokens unused this long after a notice, 0 to stop")
	}
	client, fs, jsonOut, of, code, ok := securityCommand(prog, "security policy "+verb, "print the policy as JSON to stdout and nothing else", args[1:], stderr, lookupEnv, extra)
	if !ok {
		return code
	}
	ctx := context.Background()
	var p apiclient.SecurityPolicy
	var err error
	if verb == "get" {
		p, err = client.GetSecurityPolicy(ctx)
	} else {
		var in apiclient.SecurityPolicyUpdate
		fs.Visit(func(f *flag.Flag) {
			switch f.Name {
			case "approval-scope":
				in.ApprovalScope = &scope
			case "max-token-lifetime-days":
				in.MaxTokenLifetimeDays = &maxLife
			case "warn-unused-days":
				in.WarnUnusedDays = &warn
			case "disable-unused-days":
				in.DisableUnusedDays = &disable
			}
		})
		if in == (apiclient.SecurityPolicyUpdate{}) {
			_, _ = fmt.Fprintf(stderr, "%s: security policy set needs at least one policy flag\n", prog)
			return exitUsage
		}
		p, err = client.UpdateSecurityPolicy(ctx, in)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("security policy %s: %w", verb, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, p, func() { printPolicy(stdout, p) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printPolicy(w io.Writer, p apiclient.SecurityPolicy) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "SETTING\tVALUE\tSOURCE")
	_, _ = fmt.Fprintf(tw, "approval_scope\t%s\t%s\n", p.ApprovalScope, p.Sources["approval_scope"])
	_, _ = fmt.Fprintf(tw, "max_token_lifetime_days\t%d\t%s\n", p.MaxTokenLifetimeDays, p.Sources["max_token_lifetime_days"])
	_, _ = fmt.Fprintf(tw, "warn_unused_days\t%d\t%s\n", p.WarnUnusedDays, p.Sources["warn_unused_days"])
	_, _ = fmt.Fprintf(tw, "disable_unused_days\t%d\t%s\n", p.DisableUnusedDays, p.Sources["disable_unused_days"])
	_, _ = fmt.Fprintf(tw, "new_device_approval\t%t\tenv\n", p.NewDeviceApproval)
	_ = tw.Flush()
}
