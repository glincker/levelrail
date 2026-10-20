package main

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/edgefw"
)

// fwCall is a parsed "firewall ingress" invocation.
type fwCall struct {
	client  *apiclient.Client
	args    []string
	jsonOut bool
	of      outputFlags
}

type fwSpec struct {
	label    string
	argsHelp string
	summary  string
	nargs    int
	define   func(fs *flag.FlagSet)
}

func parseFirewallCall(prog string, args []string, stderr io.Writer, lookupEnv func(string) (string, bool), spec fwSpec) (fwCall, int, bool) {
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, spec.label, policyJSONUsage, stderr)
	if spec.define != nil {
		spec.define(fs)
	}
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s %s %s [flags]\n\n%s\n\nFlags:\n", prog, spec.label, spec.argsHelp, spec.summary)
		fs.PrintDefaults()
	}
	token, apiURL, profile, jsonOut, of, code, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return fwCall{}, code, false
	}
	rest := fs.Args()
	if spec.nargs > 0 {
		var ok2 bool
		if rest, ok2 = requireArgs(fs, stderr, prog, spec.label, spec.argsHelp, spec.nargs); !ok2 {
			return fwCall{}, exitUsage, false
		}
	}
	return fwCall{client: apiClientFromFlags(prog, apiURL, token, profile, lookupEnv), args: rest, jsonOut: jsonOut, of: of}, exitOK, true
}

func (c fwCall) fail(stdout, stderr io.Writer, what string, err error) int {
	return reportError(stdout, stderr, c.jsonOut, fmt.Errorf("%s: %w", what, err))
}

func (c fwCall) emit(stdout, stderr io.Writer, v any, plain func()) int {
	return writeScheduledTaskResult(stdout, stderr, c.of, v, plain)
}

func firewallIngressUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s firewall ingress status [--scope HOST] [--range 1h|6h|24h]
  %[1]s firewall ingress rules list
  %[1]s firewall ingress rules add --name N --action allow|deny|challenge|log|rate_limit --when "FIELD OP VALUE" [--when ...] [flags]
  %[1]s firewall ingress rules update <id> [same flags as add]
  %[1]s firewall ingress rules delete <id>
  %[1]s firewall ingress rules move <id> --to POSITION
  %[1]s firewall ingress block <ip-or-cidr> [--scope HOST] [--note TEXT] [--force]
  %[1]s firewall ingress bypass <ip-or-cidr> [--scope HOST] [--note TEXT]
  %[1]s firewall ingress bans list
  %[1]s firewall ingress bans lift <ip>
  %[1]s firewall ingress bots show
  %[1]s firewall ingress bots set [--bots off|log|challenge|block] [--ai-bots off|log|challenge|block] [--ai-agents a,b,c] [--bans on|off]
  %[1]s firewall ingress attack-mode on [--minutes N] [--challenge]
  %[1]s firewall ingress attack-mode off
  %[1]s firewall ingress traffic [--scope HOST]
  %[1]s firewall ingress audit [--limit N]

The ingress firewall filters web requests before they reach an app. It is
separate from the host firewall commands above, which manage ports on the
server itself. Rules run in order; allow ends evaluation, deny and challenge
end it, log and rate_limit let the request continue unless the limit trips.
A condition is FIELD OP VALUE, for example "path starts_with /admin",
"header:X-Env equals prod", "ip in_cidr 203.0.113.0/24" or "query:debug exists".
Fields: path host method header:NAME query:NAME user_agent ip country.
Operators: equals starts_with contains glob in_cidr exists.
`, prog)
}

func runFirewallIngress(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, firewallIngressUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, firewallIngressUsage(prog))
		return exitOK
	case "status":
		return runFwStatus(prog, rest, stdout, stderr, lookupEnv)
	case "rules":
		return runFwRules(prog, rest, stdout, stderr, lookupEnv)
	case "block":
		return runFwIP(prog, "block", rest, stdout, stderr, lookupEnv)
	case "bypass":
		return runFwIP(prog, "bypass", rest, stdout, stderr, lookupEnv)
	case "bans":
		return runFwBans(prog, rest, stdout, stderr, lookupEnv)
	case "bots":
		return runFwBots(prog, rest, stdout, stderr, lookupEnv)
	case "attack-mode":
		return runFwAttack(prog, rest, stdout, stderr, lookupEnv)
	case "traffic":
		return runFwTraffic(prog, rest, stdout, stderr, lookupEnv)
	case "audit":
		return runFwAudit(prog, rest, stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown firewall ingress subcommand %q\n\n", prog, args[0])
	_, _ = fmt.Fprint(stderr, firewallIngressUsage(prog))
	return exitUsage
}

func runFwStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var scope, rng string
	c, code, ok := parseFirewallCall(prog, args, stderr, lookupEnv, fwSpec{label: "firewall ingress status", summary: "Shows the ingress firewall state and request outcomes.",
		define: func(fs *flag.FlagSet) {
			fs.StringVar(&scope, "scope", "", "limit to one host (default: every host)")
			fs.StringVar(&rng, "range", "1h", "time range: 1h, 6h or 24h")
		}})
	if !ok {
		return code
	}
	st, err := c.client.FirewallEdgeStatus(bg(), scope, rng)
	if err != nil {
		return c.fail(stdout, stderr, "firewall status", err)
	}
	return c.emit(stdout, stderr, st, func() {
		attack := "off"
		if st.Attack.Active {
			attack = "on until " + st.Attack.Until
		}
		_, _ = fmt.Fprintf(stdout, "ingress firewall active: %d of %d rules enabled, bots %s, AI bots %s, auto bans %t (%d active), attack mode %s\n",
			st.RulesEnabled, st.RulesTotal, st.BotMode, st.AIBotMode, st.BansEnabled, st.Bans, attack)
		_, _ = fmt.Fprintf(stdout, "last %s: allowed %d, denied %d, challenged %d, logged %d, rate limited %d\n", st.Range,
			st.Totals["allowed"], st.Totals["denied"], st.Totals["challenged"], st.Totals["logged"], st.Totals["rate_limited"])
		if st.Suggestion.Suggested {
			_, _ = fmt.Fprintf(stdout, "blocked traffic is %d per minute (threshold %d): consider attack mode\n", st.Suggestion.PerMinute, st.Suggestion.Threshold)
		}
	})
}

func printFwRules(out io.Writer, rules []apiclient.FirewallEdgeRule) {
	if len(rules) == 0 {
		_, _ = fmt.Fprintln(out, "no ingress firewall rules")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "POS\tID\tON\tACTION\tSCOPE\tNAME\tWHEN\tHITS")
	for _, r := range rules {
		scope := r.Scope
		if scope == "" {
			scope = "all"
		}
		_, _ = fmt.Fprintf(tw, "%d\t%s\t%t\t%s\t%s\t%s\t%s\t%d\n", r.Position, r.ID, r.Enabled, r.Action, scope, r.Name, whenText(r.Conditions), r.Hits.Total)
	}
	_ = tw.Flush()
}

func whenText(conds []edgefw.Condition) string {
	parts := make([]string, len(conds))
	for i, c := range conds {
		f := string(c.Field)
		if c.Name != "" {
			f += ":" + c.Name
		}
		parts[i] = strings.TrimSpace(f + " " + string(c.Op) + " " + c.Value)
	}
	return strings.Join(parts, " AND ")
}

func runFwBans(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %[1]s firewall ingress bans list\n  %[1]s firewall ingress bans lift <ip>\n", prog)
		return exitUsage
	}
	switch args[0] {
	case "list":
		c, code, ok := parseFirewallCall(prog, args[1:], stderr, lookupEnv, fwSpec{label: "firewall ingress bans list", summary: "Lists automatic temporary bans."})
		if !ok {
			return code
		}
		bans, err := c.client.ListFirewallEdgeBans(bg())
		if err != nil {
			return c.fail(stdout, stderr, "list bans", err)
		}
		return c.emit(stdout, stderr, bans, func() {
			if len(bans) == 0 {
				_, _ = fmt.Fprintln(stdout, "no active bans")
				return
			}
			tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "IP\tREASON\tSTARTED\tENDS\tBLOCKED")
			for _, b := range bans {
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\n", b.IP, b.Reason, b.Started, b.Ends, b.Hits)
			}
			_ = tw.Flush()
		})
	case "lift":
		c, code, ok := parseFirewallCall(prog, args[1:], stderr, lookupEnv, fwSpec{label: "firewall ingress bans lift", argsHelp: "<ip>", summary: "Ends a temporary ban now.", nargs: 1})
		if !ok {
			return code
		}
		if err := c.client.LiftFirewallEdgeBan(bg(), c.args[0]); err != nil {
			return c.fail(stdout, stderr, "lift ban", err)
		}
		return c.emit(stdout, stderr, map[string]bool{"lifted": true}, func() { _, _ = fmt.Fprintf(stdout, "ban on %s lifted\n", c.args[0]) })
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown bans subcommand %q\n", prog, args[0])
	return exitUsage
}

func runFwTraffic(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var scope string
	c, code, ok := parseFirewallCall(prog, args, stderr, lookupEnv, fwSpec{label: "firewall ingress traffic", summary: "Shows the busiest clients, paths and user agents of the last hour (approximate, in memory only).",
		define: func(fs *flag.FlagSet) { fs.StringVar(&scope, "scope", "", "limit to one host") }})
	if !ok {
		return code
	}
	tr, err := c.client.FirewallEdgeTraffic(bg(), scope)
	if err != nil {
		return c.fail(stdout, stderr, "firewall traffic", err)
	}
	return c.emit(stdout, stderr, tr, func() {
		for _, dim := range []string{"ip", "denied_ip", "user_agent", "path", "host"} {
			_, _ = fmt.Fprintf(stdout, "%s\n", dim)
			for _, row := range tr.Tables[dim] {
				_, _ = fmt.Fprintf(stdout, "  %6d  %s\n", row.Count, row.Key)
			}
		}
	})
}

func runFwAudit(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var limit int
	c, code, ok := parseFirewallCall(prog, args, stderr, lookupEnv, fwSpec{label: "firewall ingress audit", summary: "Lists firewall changes, newest first.",
		define: func(fs *flag.FlagSet) { fs.IntVar(&limit, "limit", 50, "how many entries") }})
	if !ok {
		return code
	}
	rows, err := c.client.FirewallEdgeAudit(bg(), limit)
	if err != nil {
		return c.fail(stdout, stderr, "firewall audit", err)
	}
	return c.emit(stdout, stderr, rows, func() {
		for _, e := range rows {
			_, _ = fmt.Fprintf(stdout, "%s  %-28s  %s\n", e.CreatedAt, e.Action, e.ActorName)
		}
		if len(rows) == 0 {
			_, _ = fmt.Fprintln(stdout, "no firewall changes recorded")
		}
	})
}
