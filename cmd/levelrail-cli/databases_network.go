package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func databasesNetworkUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases network show <database> [flags]                       reachability, allowed sources, scope and TLS
  %[1]s databases network allow <database> --source CIDR [flags]         add an allowed source (previews, then --confirm applies)
  %[1]s databases network deny <database> --source CIDR [flags]          remove an allowed source
  %[1]s databases network make-private <database> --confirm [flags]      stop publishing the port and drop its rules
  %[1]s databases network scope <database> --scope S [flags]             platform, project or environment (--dry-run first)
  %[1]s databases network tls <database> --require|--optional [flags]    require TLS for connections

Run "%[1]s databases network <subcommand> -h" for a subcommand's own flags.
`, prog)
}

// runDatabasesNetwork dispatches "databases network <verb>".
func runDatabasesNetwork(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, databasesNetworkUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, databasesNetworkUsage(prog))
		return exitOK
	case "show":
		return runDatabasesNetworkShow(prog, rest, stdout, stderr, lookupEnv)
	case "allow":
		return runDatabasesNetworkRule(prog, rest, stdout, stderr, lookupEnv, true)
	case "deny":
		return runDatabasesNetworkRule(prog, rest, stdout, stderr, lookupEnv, false)
	case "make-private":
		return runDatabasesNetworkMakePrivate(prog, rest, stdout, stderr, lookupEnv)
	case "scope":
		return runDatabasesNetworkScope(prog, rest, stdout, stderr, lookupEnv)
	case "tls":
		return runDatabasesNetworkTLS(prog, rest, stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown databases network subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, databasesNetworkUsage(prog))
		return exitUsage
	}
}

func runDatabasesNetworkShow(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseDBAccess(prog, "databases network show", "databases network show <database> [flags]", args, 1, "a database name", stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	n, err := c.client.GetDatabaseNetwork(context.Background(), c.rest[0])
	if err != nil {
		return c.fail(fmt.Errorf("show network of database %q: %w", c.rest[0], err))
	}
	return c.result(n, func() { printDatabaseNetwork(stdout, n) })
}

func printDatabaseNetwork(out io.Writer, n apiclient.DatabaseNetwork) {
	_, _ = fmt.Fprintf(out, "%s\n\n", n.Verdict.Text)
	_, _ = fmt.Fprintf(out, "internal address: %s:%d", n.Internal.Host, n.Internal.Port)
	if n.Internal.Address != "" {
		_, _ = fmt.Fprintf(out, " (%s)", n.Internal.Address)
	}
	_, _ = fmt.Fprintln(out)
	for _, net := range n.Networks {
		_, _ = fmt.Fprintf(out, "  network %s (%s) %s\n", net.Name, net.Kind, net.Address)
	}
	if n.Published != nil {
		_, _ = fmt.Fprintf(out, "published: host port %d, bind %s, %s\n", n.Published.HostPort, strings.Join(n.Published.Bind, ","), n.Published.Class)
	}
	_, _ = fmt.Fprintf(out, "scope: %s    tls required: %v (server: %s)\n", n.Scope.Current, n.TLS.Required, dashIfEmpty(n.TLS.State))
	if len(n.Clients) > 0 {
		tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "\nAPP\tVIA\tPROJECT\tENVIRONMENT\tIN SCOPE")
		for _, cl := range n.Clients {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%v\n", cl.App, cl.Via, dashIfEmpty(cl.ProjectName), dashIfEmpty(cl.Environment), cl.InScope)
		}
		_ = tw.Flush()
	}
	if len(n.Rules.Allow) > 0 {
		_, _ = fmt.Fprintln(out, "\nallowed sources:")
		for _, r := range n.Rules.Allow {
			_, _ = fmt.Fprintf(out, "  %s  %s\n", r.Source, r.Description)
		}
	}
	for _, m := range n.Rules.Missing {
		_, _ = fmt.Fprintf(out, "drift: rule for %s is missing from the firewall\n", m)
	}
	for _, e := range n.Rules.Extra {
		_, _ = fmt.Fprintf(out, "drift: the firewall also allows %s\n", e)
	}
}

func runDatabasesNetworkRule(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), allow bool) int {
	verb := "deny"
	if allow {
		verb = "allow"
	}
	var source, desc string
	var confirm, clearAll bool
	c, code, ok := parseDBAccess(prog, "databases network "+verb, "databases network "+verb+" <database> --source CIDR [flags]", args, 1, "a database name", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&source, "source", "", "source IP or CIDR")
		fs.StringVar(&desc, "description", "", "what this source is, for the next reader")
		fs.BoolVar(&confirm, "confirm", false, "apply the change; without it only a preview is shown")
		if !allow {
			fs.BoolVar(&clearAll, "clear", false, "remove the last source and the whole restriction (the port becomes reachable by anyone who can route to it)")
		}
	})
	if !ok {
		return code
	}
	source = strings.TrimSpace(source)
	if source == "" {
		return c.fail(errors.New("--source is required"))
	}
	ctx := context.Background()
	name := c.rest[0]
	net, err := c.client.GetDatabaseNetwork(ctx, name)
	if err != nil {
		return c.fail(fmt.Errorf("read network of database %q: %w", name, err))
	}
	next := mergeSources(net.Rules.Allow, source, desc, allow)
	if len(next) == 0 {
		if !clearAll {
			return c.fail(errors.New("that would remove the last allowed source; use --clear to drop the restriction, or make-private to stop publishing the port"))
		}
		if err := c.client.RemoveDatabaseRules(ctx, name); err != nil {
			return c.fail(fmt.Errorf("remove rules of database %q: %w", name, err))
		}
		return c.result(map[string]any{"cleared": true}, func() { _, _ = fmt.Fprintln(stdout, "restriction removed") })
	}
	req := apiclient.DatabaseRulesRequest{Allow: next, Confirm: confirm}
	var plan apiclient.DatabaseRulesPlan
	if confirm {
		plan, err = c.client.ApplyDatabaseRules(ctx, name, req)
	} else {
		plan, err = c.client.PreviewDatabaseRules(ctx, name, req)
	}
	if err != nil {
		return c.fail(fmt.Errorf("%s source on database %q: %w", verb, name, err))
	}
	return c.result(plan, func() {
		_, _ = fmt.Fprintln(stdout, plan.Drops)
		for _, w := range plan.Warnings {
			_, _ = fmt.Fprintf(stdout, "warning: %s\n", w)
		}
		if plan.Applied {
			_, _ = fmt.Fprintf(stdout, "applied: allowed sources are now %s\n", strings.Join(plan.Allow, ", "))
		} else {
			_, _ = fmt.Fprintln(stdout, "preview only, add --confirm to apply")
		}
	})
}

func mergeSources(cur []apiclient.DatabaseRule, source, desc string, add bool) []apiclient.DatabaseRule {
	out := make([]apiclient.DatabaseRule, 0, len(cur)+1)
	for _, r := range cur {
		if r.Source == source {
			continue
		}
		out = append(out, r)
	}
	if add {
		out = append(out, apiclient.DatabaseRule{Source: source, Description: desc})
	}
	return out
}

func runDatabasesNetworkMakePrivate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var confirm bool
	c, code, ok := parseDBAccess(prog, "databases network make-private", "databases network make-private <database> --confirm [flags]", args, 1, "a database name", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.BoolVar(&confirm, "confirm", false, "required: the published port is closed and the database container is replaced")
	})
	if !ok {
		return code
	}
	if !confirm {
		return c.fail(errors.New("this closes the published port and restarts the database container; add --confirm to proceed"))
	}
	if err := c.client.MakeDatabasePrivate(context.Background(), c.rest[0]); err != nil {
		return c.fail(fmt.Errorf("make database %q private: %w", c.rest[0], err))
	}
	return c.result(map[string]any{"private": true}, func() { _, _ = fmt.Fprintf(stdout, "database %q is private\n", c.rest[0]) })
}

func runDatabasesNetworkScope(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var scope string
	var dryRun, confirm bool
	c, code, ok := parseDBAccess(prog, "databases network scope", "databases network scope <database> --scope platform|project|environment [flags]", args, 1, "a database name", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&scope, "scope", "", "platform, project or environment")
		fs.BoolVar(&dryRun, "dry-run", false, "list which apps would lose reachability and change nothing")
		fs.BoolVar(&confirm, "confirm", false, "apply even though some apps lose reachability")
	})
	if !ok {
		return code
	}
	res, err := c.client.SetDatabaseScope(context.Background(), c.rest[0], scope, dryRun, confirm)
	if err != nil {
		return c.fail(fmt.Errorf("set scope of database %q: %w", c.rest[0], err))
	}
	return c.result(res, func() {
		for _, v := range res.Verdicts {
			state := "keeps access"
			if !v.Allowed {
				state = "LOSES access: " + v.Reason
			}
			_, _ = fmt.Fprintf(stdout, "  %s %s\n", v.App.Name, state)
		}
		switch {
		case res.Applied:
			_, _ = fmt.Fprintf(stdout, "scope set to %s\n", res.Scope)
		case res.ConfirmRequired:
			_, _ = fmt.Fprintln(stdout, "not applied: some apps would lose reachability, review the list and add --confirm")
		default:
			_, _ = fmt.Fprintln(stdout, "dry run, nothing changed")
		}
	})
}

func runDatabasesNetworkTLS(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var require, optional bool
	c, code, ok := parseDBAccess(prog, "databases network tls", "databases network tls <database> --require|--optional [flags]", args, 1, "a database name", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.BoolVar(&require, "require", false, "refuse connections that do not use TLS")
		fs.BoolVar(&optional, "optional", false, "accept connections with or without TLS")
	})
	if !ok {
		return code
	}
	if require == optional {
		return c.fail(errors.New("pass exactly one of --require or --optional"))
	}
	if err := c.client.SetDatabaseRequireTLS(context.Background(), c.rest[0], require); err != nil {
		return c.fail(fmt.Errorf("set tls requirement of database %q: %w", c.rest[0], err))
	}
	return c.result(map[string]any{"require_tls": require}, func() {
		_, _ = fmt.Fprintf(stdout, "tls required: %v\n", require)
	})
}
