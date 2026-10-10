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

func databasesAccessUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases access temp <database> [flags]        issue a short-lived login (--preset, --minutes)
  %[1]s databases access list <database> [flags]        list active temporary logins
  %[1]s databases access revoke <database> <id> [flags] revoke a temporary login now
  %[1]s databases access who <database> [flags]         who and which policies can reach this database
  %[1]s databases access grant <database> [flags]       apply a database-scoped policy (--template, --user-id or --token-id)

Run "%[1]s databases access <subcommand> -h" for a subcommand's own flags.
`, prog)
}

// runDatabasesAccess dispatches "databases access <verb>".
func runDatabasesAccess(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, databasesAccessUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, databasesAccessUsage(prog))
		return exitOK
	case "temp":
		return runDatabasesAccessTemp(prog, rest, stdout, stderr, lookupEnv)
	case "list":
		return runDatabasesAccessList(prog, rest, stdout, stderr, lookupEnv)
	case "revoke":
		return runDatabasesAccessRevoke(prog, rest, stdout, stderr, lookupEnv)
	case "who":
		return runDatabasesAccessWho(prog, rest, stdout, stderr, lookupEnv)
	case "grant":
		return runDatabasesAccessGrant(prog, rest, stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown databases access subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, databasesAccessUsage(prog))
		return exitUsage
	}
}

func runDatabasesAccessTemp(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var preset string
	var minutes int
	c, code, ok := parseDBAccess(prog, "databases access temp", "databases access temp <database> [flags]", args, 1, "a database name", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&preset, "preset", "read_only", "read_only or read_write")
		fs.IntVar(&minutes, "minutes", 0, "lifetime in minutes (default and limits come from the server)")
	})
	if !ok {
		return code
	}
	out, err := c.client.IssueDatabaseTemp(context.Background(), c.rest[0], preset, minutes)
	if err != nil {
		return c.fail(fmt.Errorf("issue temporary credentials for database %q: %w", c.rest[0], err))
	}
	return c.result(out, func() {
		printCredential(stdout, out.Credential, "temporary login")
		if out.Clamped {
			_, _ = fmt.Fprintf(stdout, "note: the lifetime was moved into the allowed %d to %d minutes\n", out.Limits.MinMinutes, out.Limits.MaxMinutes)
		}
		_, _ = fmt.Fprintln(stdout, "it is revoked automatically at expiry")
	})
}

func runDatabasesAccessList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseDBAccess(prog, "databases access list", "databases access list <database> [flags]", args, 1, "a database name", stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	out, err := c.client.ListDatabaseTemp(context.Background(), c.rest[0])
	if err != nil {
		return c.fail(fmt.Errorf("list temporary credentials for database %q: %w", c.rest[0], err))
	}
	return c.result(out, func() {
		if len(out.Items) == 0 {
			_, _ = fmt.Fprintln(stdout, "no active temporary logins")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "ID\tROLE\tPRESET\tCREATED BY\tEXPIRES")
		for _, t := range out.Items {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", t.ID, t.Role, t.Preset, t.CreatedBy, t.ExpiresAt)
		}
		_ = tw.Flush()
	})
}

func runDatabasesAccessRevoke(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseDBAccess(prog, "databases access revoke", "databases access revoke <database> <id> [flags]", args, 2, "a database name and a credential id", stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	if err := c.client.RevokeDatabaseTemp(context.Background(), c.rest[0], c.rest[1]); err != nil {
		return c.fail(fmt.Errorf("revoke temporary credential %q on database %q: %w", c.rest[1], c.rest[0], err))
	}
	return c.result(map[string]any{"revoked": c.rest[1]}, func() { _, _ = fmt.Fprintf(stdout, "revoked %s\n", c.rest[1]) })
}

func runDatabasesAccessWho(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseDBAccess(prog, "databases access who", "databases access who <database> [flags]", args, 1, "a database name", stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	out, err := c.client.GetDatabaseWho(context.Background(), c.rest[0])
	if err != nil {
		return c.fail(fmt.Errorf("who can access database %q: %w", c.rest[0], err))
	}
	return c.result(out, func() { printDatabaseWho(stdout, out) })
}

func printDatabaseWho(out io.Writer, w apiclient.DatabaseWho) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "TYPE\tNAME\tABILITIES\tVIA")
	for _, p := range w.Principals {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", p.Type, p.Name, strings.Join(p.Effective, ","), strings.Join(p.Via, "; "))
	}
	_ = tw.Flush()
	if len(w.Policies) > 0 {
		_, _ = fmt.Fprintln(out, "\npolicies that touch this database:")
		for _, p := range w.Policies {
			_, _ = fmt.Fprintf(out, "  %s (%d attached)\n", p.Name, len(p.Principals))
		}
	}
}

func runDatabasesAccessGrant(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var template, user, token string
	var preview bool
	c, code, ok := parseDBAccess(prog, "databases access grant", "databases access grant <database> --template T (--user-id ID | --token-id ID) [flags]", args, 1, "a database name", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&template, "template", "", "database-read-only, database-operator or database-owner")
		fs.StringVar(&user, "user-id", "", "user id to grant")
		fs.StringVar(&token, "token-id", "", "API token id to grant")
		fs.BoolVar(&preview, "preview", false, "show the policy without applying it")
	})
	if !ok {
		return code
	}
	req := apiclient.DatabaseGrantRequest{Template: template, Preview: preview}
	switch {
	case user != "" && token == "":
		req.PrincipalType, req.PrincipalID = "user", user
	case token != "" && user == "":
		req.PrincipalType, req.PrincipalID = "token", token
	default:
		return c.fail(fmt.Errorf("pass exactly one of --user-id or --token-id"))
	}
	out, err := c.client.GrantDatabaseAccess(context.Background(), c.rest[0], req)
	if err != nil {
		return c.fail(fmt.Errorf("grant access to database %q: %w", c.rest[0], err))
	}
	return c.result(out, func() {
		verb := "would apply"
		if out.Applied {
			verb = "applied"
		}
		_, _ = fmt.Fprintf(stdout, "%s policy %s to %s\n", verb, out.PolicyName, out.Principal)
		for _, n := range out.Notes {
			_, _ = fmt.Fprintf(stdout, "  %s\n", n)
		}
	})
}
