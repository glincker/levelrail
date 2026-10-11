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

type lookupFn = func(string) (string, bool)

// teamsCmd is one parsed teams subcommand: flags resolved, positionals checked.
type teamsCmd struct {
	client  *apiclient.Client
	args    []string
	jsonOut bool
	of      outputFlags
	profile string
}

// parseTeamsCmd builds the flag set, parses, and checks the positional count.
// extra registers command specific flags before parsing.
func parseTeamsCmd(prog, label, usage string, nargs int, args []string, stderr io.Writer, lookupEnv lookupFn, extra func(*flag.FlagSet)) (teamsCmd, int, bool) {
	fs, tokenP, urlP, profileP, jsonP, outP, queryP := apiFlagSet(prog, label, "print the result as JSON to stdout, nothing else", stderr)
	if extra != nil {
		extra(fs)
	}
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s %s\n\nRun \"%s teams -h\" for every teams command.\n", prog, usage, prog)
	}
	token, apiURL, profile, jsonOut, of, code, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, urlP, profileP, jsonP, outP, queryP}, prog, stderr)
	if !ok {
		return teamsCmd{}, code, false
	}
	rest := fs.Args()
	if len(rest) != nargs {
		_, _ = fmt.Fprintf(stderr, "%s: %s takes %d argument(s)\n\n", prog, label, nargs)
		fs.Usage()
		return teamsCmd{}, exitUsage, false
	}
	return teamsCmd{client: apiClientFromFlags(prog, apiURL, token, profile, lookupEnv), args: rest, jsonOut: jsonOut, of: of, profile: resolveProfile(profile, lookupEnv)}, exitOK, true
}

// finish renders data or reports err, returning the exit code.
func (c teamsCmd) finish(stdout, stderr io.Writer, data any, err error, table func()) int {
	if err != nil {
		return reportError(stdout, stderr, c.jsonOut, err)
	}
	if err := renderResult(stdout, c.of.Format, c.of.Query, data, table); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func teamsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s teams list                                  teams you belong to, with your role
  %[1]s teams create <name>                         create a team (instance admins); you become its owner
  %[1]s teams show <team>                           one team and your role in it
  %[1]s teams rename <team> <name>                  rename a team (owners)
  %[1]s teams delete <team>                         delete an empty team (owners)
  %[1]s teams use [<team>] [--project ID]           save the team and project every command is scoped to; no team clears it
  %[1]s teams roles                                 the four team roles and what each grants
  %[1]s teams transfer <team> <user-id>             hand ownership to another member; you become admin
  %[1]s teams events <team> [--limit N]             the team's membership audit trail (admins)
  %[1]s teams members <verb> ...                    list, set-role, remove, leave
  %[1]s teams invites <verb> ...                    list, create, revoke, accept

Every command also takes --team ID (and, before the command name, --project ID)
to narrow what the control plane shows, overriding APP_TEAM, APP_PROJECT and the saved context.
`, prog)
}

// runTeams dispatches "teams <verb>".
func runTeams(prog string, args []string, stdout, stderr io.Writer, lookupEnv lookupFn) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, teamsUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, teamsUsage(prog))
		return exitOK
	case "list":
		return runTeamsList(prog, rest, stdout, stderr, lookupEnv)
	case "create", "show", "rename", "delete":
		return runTeamsCRUD(prog, args[0], rest, stdout, stderr, lookupEnv)
	case "use":
		return runTeamsUse(prog, rest, stdout, stderr, lookupEnv)
	case "roles":
		return runTeamsRoles(prog, rest, stdout, stderr, lookupEnv)
	case "transfer":
		return runTeamsTransfer(prog, rest, stdout, stderr, lookupEnv)
	case "events":
		return runTeamsEvents(prog, rest, stdout, stderr, lookupEnv)
	case "members":
		return runMembers(prog, rest, stdout, stderr, lookupEnv)
	case "invites":
		return runTeamInvites(prog, rest, stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown teams subcommand %q\n\n%s", prog, args[0], teamsUsage(prog))
	return exitUsage
}

func printTeams(out io.Writer, teams []apiclient.TeamResource) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tYOUR ROLE\tMEMBERS\tDEFAULT")
	for _, t := range teams {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%t\n", t.ID, t.Name, t.Role, t.MemberCount, t.Default)
	}
	_ = tw.Flush()
}

func runTeamsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv lookupFn) int {
	c, code, ok := parseTeamsCmd(prog, "teams list", "teams list [flags]", 0, args, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	teams, err := c.client.ListTeams(context.Background())
	return c.finish(stdout, stderr, teams, err, func() { printTeams(stdout, teams) })
}

func runTeamsCRUD(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv lookupFn) int {
	usage := map[string]string{"create": "teams create <name>", "show": "teams show <team>", "rename": "teams rename <team> <name>", "delete": "teams delete <team>"}[verb]
	nargs := strings.Count(usage, "<")
	c, code, ok := parseTeamsCmd(prog, "teams "+verb, usage+" [flags]", nargs, args, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	ctx := context.Background()
	var team apiclient.TeamResource
	var err error
	switch verb {
	case "create":
		team, err = c.client.CreateTeam(ctx, c.args[0])
	case "show":
		team, err = c.client.GetTeam(ctx, c.args[0])
	case "rename":
		team, err = c.client.RenameTeam(ctx, c.args[0], c.args[1])
	case "delete":
		err = c.client.DeleteTeam(ctx, c.args[0])
		return c.finish(stdout, stderr, map[string]bool{"deleted": true}, err, func() {
			_, _ = fmt.Fprintf(stdout, "team %s deleted\n", c.args[0])
		})
	}
	return c.finish(stdout, stderr, team, err, func() { printTeams(stdout, []apiclient.TeamResource{team}) })
}

func runTeamsUse(prog string, args []string, stdout, stderr io.Writer, lookupEnv lookupFn) int {
	var project *string
	nargs := 1
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		nargs = 0
	}
	c, code, ok := parseTeamsCmd(prog, "teams use", "teams use [<team>] [--project ID]", nargs, args, stderr, lookupEnv, func(fs *flag.FlagSet) {
		project = fs.String("project", "", "also scope to this project")
	})
	if !ok {
		return code
	}
	scope := apiclient.Scope{Project: *project}
	if nargs == 1 {
		scope.Team = c.args[0]
		if _, err := c.client.GetTeam(context.Background(), scope.Team); err != nil {
			return reportError(stdout, stderr, c.jsonOut, fmt.Errorf("team %q: %w", scope.Team, err))
		}
	}
	err := apiclient.WriteContext(prog, c.profile, scope)
	return c.finish(stdout, stderr, scope, err, func() {
		if scope.Team == "" {
			_, _ = fmt.Fprintln(stdout, "team context cleared")
			return
		}
		_, _ = fmt.Fprintf(stdout, "now scoped to team %s %s\n", scope.Team, scope.Project)
	})
}

func runTeamsRoles(prog string, args []string, stdout, stderr io.Writer, lookupEnv lookupFn) int {
	c, code, ok := parseTeamsCmd(prog, "teams roles", "teams roles [flags]", 0, args, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	roles, err := c.client.ListTeamRoles(context.Background())
	return c.finish(stdout, stderr, roles, err, func() {
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "ROLE\tABILITIES")
		for _, r := range roles {
			_, _ = fmt.Fprintf(tw, "%s\t%s\n", r.Name, strings.Join(r.Abilities, ","))
		}
		_ = tw.Flush()
	})
}

func runTeamsTransfer(prog string, args []string, stdout, stderr io.Writer, lookupEnv lookupFn) int {
	c, code, ok := parseTeamsCmd(prog, "teams transfer", "teams transfer <team> <user-id> [flags]", 2, args, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	err := c.client.TransferTeam(context.Background(), c.args[0], c.args[1])
	return c.finish(stdout, stderr, map[string]bool{"transferred": true}, err, func() {
		_, _ = fmt.Fprintf(stdout, "ownership of %s transferred to %s\n", c.args[0], c.args[1])
	})
}

func runTeamsEvents(prog string, args []string, stdout, stderr io.Writer, lookupEnv lookupFn) int {
	var limit *int
	c, code, ok := parseTeamsCmd(prog, "teams events", "teams events <team> [--limit N]", 1, args, stderr, lookupEnv, func(fs *flag.FlagSet) {
		limit = fs.Int("limit", 50, "how many events, newest first")
	})
	if !ok {
		return code
	}
	events, err := c.client.ListTeamEvents(context.Background(), c.args[0], *limit)
	return c.finish(stdout, stderr, events, err, func() {
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "WHEN\tWHO\tACTION\tSUBJECT\tDETAIL")
		for _, e := range events {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", e.CreatedAt, e.ActorName, e.Action, e.Subject, e.Detail)
		}
		_ = tw.Flush()
	})
}
