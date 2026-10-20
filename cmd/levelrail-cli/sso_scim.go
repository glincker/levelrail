package main

import (
	"context"
	"flag"
	"fmt"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func (c cliCtx) printSCIMTokens(toks ...apiclient.SCIMToken) func() {
	return func() {
		tw := tabwriter.NewWriter(c.stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "ID\tNAME\tPREFIX\tCREATED\tLAST USED\tREVOKED")
		for _, t := range toks {
			used, revoked := "-", "-"
			if t.LastUsedAt != nil {
				used = *t.LastUsedAt
			}
			if t.RevokedAt != nil {
				revoked = *t.RevokedAt
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", t.ID, t.Name, t.Prefix, t.CreatedAt, used, revoked)
		}
		_ = tw.Flush()
	}
}

func (c cliCtx) printSCIMSecret(t apiclient.SCIMTokenCreated) func() {
	return func() {
		_, _ = fmt.Fprintf(c.stdout, "SCIM token %q (id %s). It is shown once, store it in your identity provider now:\n%s\n", t.Name, t.ID, t.Token)
	}
}

// runSCIM dispatches "sso scim tokens <verb>".
func (c cliCtx) runSCIM(args []string) int {
	if len(args) > 1 {
		switch args[0] {
		case "tokens":
			return c.runSCIMTokens(args[1:])
		}
	}
	_, _ = fmt.Fprint(c.stderr, ssoUsage(c.prog))
	return exitUsage
}

func (c cliCtx) runSCIMTokens(args []string) int {
	rest := args[1:]
	switch args[0] {
	case "list":
		return c.runAPI("sso scim tokens list", "sso scim tokens list [flags]", rest, nil,
			func(ctx context.Context, cl *Client, _ *flag.FlagSet) (any, func(), error) {
				toks, err := cl.ListSCIMTokens(ctx)
				return toks, c.printSCIMTokens(toks...), err
			})
	case "create":
		return c.runAPI("sso scim tokens create", "sso scim tokens create <name> [flags]", rest, nil,
			func(ctx context.Context, cl *Client, fs *flag.FlagSet) (any, func(), error) {
				name, err := oneArg(fs, "token name")
				if err != nil {
					return nil, nil, err
				}
				t, err := cl.CreateSCIMToken(ctx, name)
				return t, c.printSCIMSecret(t), err
			})
	case "rotate":
		return c.runAPI("sso scim tokens rotate", "sso scim tokens rotate <id> [flags]", rest, nil,
			func(ctx context.Context, cl *Client, fs *flag.FlagSet) (any, func(), error) {
				id, err := oneArg(fs, "token id")
				if err != nil {
					return nil, nil, err
				}
				t, err := cl.RotateSCIMToken(ctx, id)
				return t, c.printSCIMSecret(t), err
			})
	case "revoke":
		return c.runAPI("sso scim tokens revoke", "sso scim tokens revoke <id> [flags]", rest, nil,
			func(ctx context.Context, cl *Client, fs *flag.FlagSet) (any, func(), error) {
				id, err := oneArg(fs, "token id")
				if err != nil {
					return nil, nil, err
				}
				if err := cl.RevokeSCIMToken(ctx, id); err != nil {
					return nil, nil, err
				}
				_, _ = fmt.Fprintf(c.stdout, "scim token %s revoked\n", id)
				return nil, nil, nil
			})
	}
	_, _ = fmt.Fprintf(c.stderr, "%s: unknown sso scim tokens subcommand %q\n", c.prog, args[0])
	return exitUsage
}
