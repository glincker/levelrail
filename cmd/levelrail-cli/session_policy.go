package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func sessionPolicyUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s session-policy list                         show every team's session policy
  %[1]s session-policy set <team-id> [--idle 30m] [--max-age 12h] [--allow CIDR ...] [--require-mfa]
                                                    [--max-sessions N] [--on-limit evict_oldest|reject]
                                                    replace a team's policy (omitted values turn that rule off)
  %[1]s session-policy clear <team-id>              remove a team's policy
  %[1]s session-policy effective                    show the policy that applies to you and your address

Locked out by an IP allowlist? On the host run: levelrail access-policy-reset --yes
`, prog)
}

// runSessionPolicy dispatches "session-policy <verb>".
func runSessionPolicy(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c := cliCtx{prog: prog, stdout: stdout, stderr: stderr, lookupEnv: lookupEnv}
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, sessionPolicyUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, sessionPolicyUsage(prog))
		return exitOK
	case "list":
		return c.runAPI("session-policy list", "session-policy list [flags]", rest, nil,
			func(ctx context.Context, cl *Client, _ *flag.FlagSet) (any, func(), error) {
				ps, err := cl.ListSessionPolicies(ctx)
				return ps, c.printSessionPolicies(ps...), err
			})
	case "set":
		var req apiclient.SessionPolicyRequest
		var allow stringListFlag
		return c.runAPI("session-policy set", "session-policy set <team-id> [policy flags] [flags]", rest,
			func(fs *flag.FlagSet) {
				fs.StringVar(&req.IdleTimeout, "idle", "", "end sessions unused for this long, at least 5m (empty: off)")
				fs.StringVar(&req.MaxLifetime, "max-age", "", "end sessions older than this, at least 15m (empty: off)")
				fs.Var(&allow, "allow", "IP or CIDR allowed to use sessions and tokens, repeatable (none: any address)")
				fs.BoolVar(&req.RequireMFA, "require-mfa", false, "require a second factor for password sign-ins")
				fs.IntVar(&req.MaxSessions, "max-sessions", 0, "cap concurrent sessions per user, 0 for no cap")
				fs.StringVar(&req.OnLimit, "on-limit", "", "over the cap: evict_oldest (default) or reject the new sign-in")
			},
			func(ctx context.Context, cl *Client, fs *flag.FlagSet) (any, func(), error) {
				team, err := oneArg(fs, "team id")
				if err != nil {
					return nil, nil, err
				}
				req.IPAllowlist = append([]string{}, allow...)
				p, err := cl.SetSessionPolicy(ctx, team, req)
				return p, c.printSessionPolicies(p), err
			})
	case "clear":
		return c.runAPI("session-policy clear", "session-policy clear <team-id> [flags]", rest, nil,
			func(ctx context.Context, cl *Client, fs *flag.FlagSet) (any, func(), error) {
				team, err := oneArg(fs, "team id")
				if err != nil {
					return nil, nil, err
				}
				if err := cl.ClearSessionPolicy(ctx, team); err != nil {
					return nil, nil, err
				}
				_, _ = fmt.Fprintf(c.stdout, "session policy for %s removed\n", team)
				return nil, nil, nil
			})
	case "effective":
		return c.runAPI("session-policy effective", "session-policy effective [flags]", rest, nil,
			func(ctx context.Context, cl *Client, _ *flag.FlagSet) (any, func(), error) {
				e, err := cl.EffectiveSessionPolicy(ctx)
				return e, func() {
					_, _ = fmt.Fprintf(c.stdout, "idle timeout:  %s\nmax lifetime:  %s\nrequire mfa:   %t\nmax sessions:  %s\nallowlists:    %d\nyour address:  %s (allowed: %t)\n",
						orOff(e.IdleTimeout), orOff(e.MaxLifetime), e.RequireMFA, sessionCapLabel(e.MaxSessions, e.OnLimit), e.AllowLists, e.YourAddress, e.Allowed)
				}, err
			})
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown session-policy subcommand %q\n\n%s", prog, args[0], sessionPolicyUsage(prog))
	return exitUsage
}

func orOff(s string) string {
	if s == "" {
		return "off"
	}
	return s
}

func (c cliCtx) printSessionPolicies(ps ...apiclient.SessionPolicy) func() {
	return func() {
		tw := tabwriter.NewWriter(c.stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "TEAM\tIDLE\tMAX AGE\tMFA\tSESSIONS\tALLOWLIST")
		for _, p := range ps {
			list := strings.Join(p.IPAllowlist, ",")
			if list == "" {
				list = "any"
			}
			_, _ = fmt.Fprintf(tw, "%s (%s)\t%s\t%s\t%t\t%s\t%s\n", p.TeamName, p.TeamID, orOff(p.IdleTimeout), orOff(p.MaxLifetime), p.RequireMFA, sessionCapLabel(p.MaxSessions, p.OnLimit), list)
		}
		_ = tw.Flush()
	}
}

func sessionCapLabel(n int, onLimit string) string {
	if n <= 0 {
		return "off"
	}
	return fmt.Sprintf("%d (%s)", n, onLimit)
}
