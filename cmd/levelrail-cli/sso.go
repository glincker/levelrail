package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// cliCtx carries what every "sso" leaf command needs.
type cliCtx struct {
	prog           string
	stdout, stderr io.Writer
	lookupEnv      func(string) (string, bool)
}

// runAPI is the parse, call, render sequence shared by the sso commands.
// exec returns the result to print and a table renderer for it.
func (c cliCtx) runAPI(label, usage string, args []string, setup func(*flag.FlagSet), exec func(context.Context, *Client, *flag.FlagSet) (any, func(), error)) int {
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(c.prog, label, "print the result as JSON to stdout and nothing else", c.stderr)
	if setup != nil {
		setup(fs)
	}
	fs.Usage = func() {
		_, _ = fmt.Fprintf(c.stderr, "Usage:\n  %[1]s "+usage+"\n"+rolesFlagsHelp, c.prog, envAPIToken, envAPIURL, defaultAPIURL)
		fs.PrintDefaults()
	}
	token, apiURL, profile, jsonOut, of, code, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, c.prog, c.stderr)
	if !ok {
		return code
	}
	client := apiClientFromFlags(c.prog, apiURL, token, profile, c.lookupEnv)
	out, table, err := exec(context.Background(), client, fs)
	if err != nil {
		return reportError(c.stdout, c.stderr, jsonOut, err)
	}
	if out == nil {
		return exitOK
	}
	if err := renderResult(c.stdout, of.Format, of.Query, out, table); err != nil {
		_, _ = fmt.Fprintln(c.stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// runSSO dispatches "sso <area> <verb>".
func runSSO(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c := cliCtx{prog: prog, stdout: stdout, stderr: stderr, lookupEnv: lookupEnv}
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, ssoUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, ssoUsage(prog))
		return exitOK
	case "saml":
		return c.runSAML(args[1:])
	case "mappings":
		return c.runSSOMappings(args[1:])
	case "settings":
		return c.runSSOSettings(args[1:])
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown sso subcommand %q\n\n%s", prog, args[0], ssoUsage(prog))
	return exitUsage
}

func ssoUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s sso saml list|get|create|update|delete|import-metadata|metadata   manage SAML identity providers
  %[1]s sso mappings list                                     show IDP group to role mappings
  %[1]s sso mappings set --map GROUP=ROLE[:PRIORITY] ...      replace every mapping
  %[1]s sso settings get                                      show SSO settings and the SP certificate
  %[1]s sso settings set --enforce=true|false                 require SSO for non-root sign-in

Every command needs a root token. Run "%[1]s sso <area> <verb> -h" for its flags.
`, prog)
}

func (c cliCtx) runSSOMappings(args []string) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(c.stderr, ssoUsage(c.prog))
		return exitUsage
	}
	switch args[0] {
	case "list":
		return c.runAPI("sso mappings list", "sso mappings list [flags]", args[1:], nil,
			func(ctx context.Context, cl *Client, _ *flag.FlagSet) (any, func(), error) {
				m, err := cl.ListSSOGroupMappings(ctx)
				return m, func() { c.printMappings(m) }, err
			})
	case "set":
		var maps stringListFlag
		return c.runAPI("sso mappings set", "sso mappings set --map GROUP=ROLE[:PRIORITY] ... [flags]", args[1:],
			func(fs *flag.FlagSet) {
				fs.Var(&maps, "map", "GROUP=ROLE_ID[:PRIORITY], repeatable; no --map clears every mapping")
			},
			func(ctx context.Context, cl *Client, _ *flag.FlagSet) (any, func(), error) {
				req := apiclient.SSOGroupMappings{Mappings: []apiclient.SSOGroupMapping{}}
				for _, raw := range maps {
					m, err := parseSSOMapping(raw)
					if err != nil {
						return nil, nil, err
					}
					req.Mappings = append(req.Mappings, m)
				}
				m, err := cl.ReplaceSSOGroupMappings(ctx, req)
				return m, func() { c.printMappings(m) }, err
			})
	}
	_, _ = fmt.Fprintf(c.stderr, "%s: unknown sso mappings subcommand %q\n", c.prog, args[0])
	return exitUsage
}

func parseSSOMapping(raw string) (apiclient.SSOGroupMapping, error) {
	group, rest, ok := strings.Cut(raw, "=")
	rest = strings.TrimSpace(rest)
	if !ok || strings.TrimSpace(group) == "" || rest == "" {
		return apiclient.SSOGroupMapping{}, newValidationError("--map %q: want GROUP=ROLE_ID[:PRIORITY]", raw)
	}
	m := apiclient.SSOGroupMapping{Group: strings.TrimSpace(group), RoleID: rest}
	if role, prio, has := strings.Cut(rest, ":"); has {
		n, err := strconv.Atoi(strings.TrimSpace(prio))
		if err != nil {
			return apiclient.SSOGroupMapping{}, newValidationError("--map %q: priority must be a number", raw)
		}
		m.RoleID, m.Priority = strings.TrimSpace(role), n
	}
	return m, nil
}

func (c cliCtx) printMappings(m apiclient.SSOGroupMappings) {
	tw := tabwriter.NewWriter(c.stdout, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "GROUP\tROLE\tPRIORITY")
	for _, x := range m.Mappings {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\n", x.Group, x.RoleID, x.Priority)
	}
	_ = tw.Flush()
}

func (c cliCtx) runSSOSettings(args []string) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(c.stderr, ssoUsage(c.prog))
		return exitUsage
	}
	show := func(s apiclient.SSOSettings) func() {
		return func() {
			_, _ = fmt.Fprintf(c.stdout, "enforce sso:  %t\nbase url:     %s%s\n", s.EnforceSSO, s.BaseURL, s.BaseURLError)
		}
	}
	switch args[0] {
	case "get":
		return c.runAPI("sso settings get", "sso settings get [flags]", args[1:], nil,
			func(ctx context.Context, cl *Client, _ *flag.FlagSet) (any, func(), error) {
				s, err := cl.GetSSOSettings(ctx)
				return s, show(s), err
			})
	case "set":
		var enforce bool
		return c.runAPI("sso settings set", "sso settings set --enforce=true|false [flags]", args[1:],
			func(fs *flag.FlagSet) {
				fs.BoolVar(&enforce, "enforce", false, "require SSO for every non-root sign-in")
			},
			func(ctx context.Context, cl *Client, fs *flag.FlagSet) (any, func(), error) {
				if !flagWasSet(fs, "enforce") {
					return nil, nil, newValidationError("--enforce is required")
				}
				s, err := cl.SetSSOEnforce(ctx, enforce)
				return s, show(s), err
			})
	}
	_, _ = fmt.Fprintf(c.stderr, "%s: unknown sso settings subcommand %q\n", c.prog, args[0])
	return exitUsage
}

func flagWasSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}
