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

func privateNetworksUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s private-networks list [flags]                                  list private networks and allow rules
  %[1]s private-networks map [flags]                                   who can talk to whom, with blocked paths and fixes
  %[1]s private-networks create <name> [--members a,b] [--default-deny] [--project ID] [flags]
  %[1]s private-networks set <name|id> --members a,b [--default-deny] [flags]   replace members and default-deny
  %[1]s private-networks delete <name|id> [flags]                      remove a private network
  %[1]s private-networks allow <app> <app> [--note TEXT] [flags]       allow two apps to reach each other
  %[1]s private-networks deny <rule-id> [flags]                        remove an allow rule
  %[1]s private-networks apply [flags]                                 re-enforce networks and rules on Docker now

Run "%[1]s private-networks <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runPrivateNetworks(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, privateNetworksUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, privateNetworksUsage(prog))
		return exitOK
	case "list":
		return pnRun(prog, "list", rest, stdout, stderr, lookupEnv, 0, nil, pnList)
	case "map":
		return pnRun(prog, "map", rest, stdout, stderr, lookupEnv, 0, nil, pnMap)
	case "create":
		return pnRun(prog, "create", rest, stdout, stderr, lookupEnv, 1, pnMemberFlags, pnCreate)
	case "set":
		return pnRun(prog, "set", rest, stdout, stderr, lookupEnv, 1, pnMemberFlags, pnSet)
	case "delete":
		return pnRun(prog, "delete", rest, stdout, stderr, lookupEnv, 1, nil, pnDelete)
	case "allow":
		return pnRun(prog, "allow", rest, stdout, stderr, lookupEnv, 2, pnNoteFlag, pnAllow)
	case "deny":
		return pnRun(prog, "deny", rest, stdout, stderr, lookupEnv, 1, nil, pnDeny)
	case "apply":
		return pnRun(prog, "apply", rest, stdout, stderr, lookupEnv, 0, nil, pnApply)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown private-networks subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, privateNetworksUsage(prog))
		return exitUsage
	}
}

type pnOpts struct {
	members     csvListFlag
	defaultDeny bool
	project     string
	note        string
	fs          *flag.FlagSet
}

func pnMemberFlags(fs *flag.FlagSet, o *pnOpts) {
	fs.Var(&o.members, "members", "comma separated app names or IDs")
	fs.BoolVar(&o.defaultDeny, "default-deny", false, "isolate members from each other until a rule allows a pair")
	fs.StringVar(&o.project, "project", "", "project ID this network belongs to")
}

func pnNoteFlag(fs *flag.FlagSet, o *pnOpts) {
	fs.StringVar(&o.note, "note", "", "why this path is allowed")
}

type pnAction func(ctx context.Context, c *Client, o *pnOpts, args []string) (any, func(io.Writer), error)

func pnRun(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), nargs int, setup func(*flag.FlagSet, *pnOpts), do pnAction) int {
	label := "private-networks " + verb
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, label, "print the result as JSON to stdout and nothing else", stderr)
	o := &pnOpts{fs: fs}
	if setup != nil {
		setup(fs, o)
	}
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, privateNetworksUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, code, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return code
	}
	rest := fs.Args()
	if len(rest) != nargs {
		_, _ = fmt.Fprintf(stderr, "%s: %s takes %d argument(s)\n\n", prog, label, nargs)
		fs.Usage()
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	data, render, err := do(context.Background(), client, o, rest)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s: %w", label, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, data, func() { render(stdout) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func pnList(ctx context.Context, c *Client, _ *pnOpts, _ []string) (any, func(io.Writer), error) {
	l, err := c.ListPrivateNetworks(ctx)
	return l, func(w io.Writer) { pnPrintList(w, l) }, err
}

func pnPrintList(w io.Writer, l apiclient.PrivateNetworkList) {
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tDEFAULT DENY\tMEMBERS")
	for _, n := range l.Networks {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%t\t%s\n", n.ID, n.Name, n.DefaultDeny, strings.Join(n.Members, ","))
	}
	_ = tw.Flush()
	if len(l.Rules) == 0 {
		return
	}
	_, _ = fmt.Fprintln(w)
	tw = tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "RULE\tFROM\tTO\tNOTE")
	for _, r := range l.Rules {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.ID, r.From, r.To, r.Note)
	}
	_ = tw.Flush()
}

func pnMap(ctx context.Context, c *Client, _ *pnOpts, _ []string) (any, func(io.Writer), error) {
	m, err := c.GetPrivateNetworkMap(ctx)
	return m, func(w io.Writer) { pnPrintMap(w, m) }, err
}

func pnPrintMap(w io.Writer, m apiclient.PrivateNetworkMap) {
	names := make(map[string]string, len(m.Apps))
	for _, a := range m.Apps {
		names[a.ID] = a.Name
	}
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "A\tB\tVERDICT\tVIA\tOBSERVED\tFIX")
	for _, e := range m.Edges {
		verdict, via := "blocked", "-"
		if e.Allowed {
			verdict, via = "allowed", e.Via+" "+e.ViaName+e.RuleID
		}
		fix := e.Fix
		if fix == "" {
			fix = "-"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", names[e.A], names[e.B], verdict, strings.TrimSpace(via), e.Observed, fix)
	}
	_ = tw.Flush()
}

func pnCreate(ctx context.Context, c *Client, o *pnOpts, args []string) (any, func(io.Writer), error) {
	ch, err := c.CreatePrivateNetwork(ctx, apiclient.PrivateNetworkRequest{Name: args[0], ProjectID: o.project, DefaultDeny: o.defaultDeny, Members: o.members})
	return ch, func(w io.Writer) {
		_, _ = fmt.Fprintf(w, "private network %q created (id %s, enforced: %t)\n", ch.Name, ch.ID, ch.Enforced)
	}, err
}

func pnFindNetwork(ctx context.Context, c *Client, ref string) (apiclient.PrivateNetworkResource, error) {
	l, err := c.ListPrivateNetworks(ctx)
	if err != nil {
		return apiclient.PrivateNetworkResource{}, err
	}
	for _, n := range l.Networks {
		if n.ID == ref || n.Name == ref {
			return n, nil
		}
	}
	return apiclient.PrivateNetworkResource{}, fmt.Errorf("no private network %q", ref)
}

func pnSet(ctx context.Context, c *Client, o *pnOpts, args []string) (any, func(io.Writer), error) {
	n, err := pnFindNetwork(ctx, c, args[0])
	if err != nil {
		return nil, nil, err
	}
	req := apiclient.PrivateNetworkRequest{DefaultDeny: n.DefaultDeny, Members: n.Members}
	o.fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "members":
			req.Members = o.members
		case "default-deny":
			req.DefaultDeny = o.defaultDeny
		}
	})
	ch, err := c.UpdatePrivateNetwork(ctx, n.ID, req)
	return ch, func(w io.Writer) {
		_, _ = fmt.Fprintf(w, "private network %q updated (enforced: %t)\n", n.Name, ch.Enforced)
	}, err
}

func pnDelete(ctx context.Context, c *Client, _ *pnOpts, args []string) (any, func(io.Writer), error) {
	n, err := pnFindNetwork(ctx, c, args[0])
	if err != nil {
		return nil, nil, err
	}
	ch, err := c.DeletePrivateNetwork(ctx, n.ID)
	return ch, func(w io.Writer) { _, _ = fmt.Fprintf(w, "private network %q deleted\n", n.Name) }, err
}

func pnAllow(ctx context.Context, c *Client, o *pnOpts, args []string) (any, func(io.Writer), error) {
	ch, err := c.CreatePrivateNetworkRule(ctx, apiclient.PrivateNetworkRuleRequest{From: args[0], To: args[1], Note: o.note})
	return ch, func(w io.Writer) {
		_, _ = fmt.Fprintf(w, "%s and %s may now reach each other (rule %s, enforced: %t)\n", args[0], args[1], ch.ID, ch.Enforced)
	}, err
}

func pnDeny(ctx context.Context, c *Client, _ *pnOpts, args []string) (any, func(io.Writer), error) {
	ch, err := c.DeletePrivateNetworkRule(ctx, args[0])
	return ch, func(w io.Writer) { _, _ = fmt.Fprintf(w, "rule %s removed\n", args[0]) }, err
}

func pnApply(ctx context.Context, c *Client, _ *pnOpts, _ []string) (any, func(io.Writer), error) {
	ch, err := c.ApplyPrivateNetworks(ctx)
	return ch, func(w io.Writer) { _, _ = fmt.Fprintf(w, "enforced: %t\n", ch.Enforced) }, err
}
