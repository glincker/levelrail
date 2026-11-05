package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func orgsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s orgs show <org> [--json]                   profile, counts, failing apps and recent activity
  %[1]s orgs rename <org> <name>                   change the display name (org admins)
  %[1]s orgs set-slug <org> <slug>                 3 to 40 chars: lowercase letters, digits, single hyphens; "" clears
  %[1]s orgs set-description <org> <text>          at most 500 characters; "" clears
  %[1]s orgs avatar set <org> <file>               upload a PNG or JPEG avatar
  %[1]s orgs avatar rm <org> [--yes]               remove the avatar

"teams" accepts the same verbs. Every command takes --token, --api-url, --profile and --json.
`, prog)
}

// runOrgs dispatches "orgs <verb>".
func runOrgs(prog string, args []string, stdout, stderr io.Writer, lookupEnv lookupFn) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, orgsUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, orgsUsage(prog))
		return exitOK
	case "show":
		return runOrgsShow(prog, rest, stdout, stderr, lookupEnv)
	case "rename":
		return runOrgsPatch(prog, "rename", "name", rest, stdout, stderr, lookupEnv)
	case "set-slug":
		return runOrgsPatch(prog, "set-slug", "slug", rest, stdout, stderr, lookupEnv)
	case "set-description":
		return runOrgsPatch(prog, "set-description", "description", rest, stdout, stderr, lookupEnv)
	case "avatar":
		return runOrgsAvatar(prog, rest, stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown orgs subcommand %q\n\n%s", prog, args[0], orgsUsage(prog))
	return exitUsage
}

func runOrgsShow(prog string, args []string, stdout, stderr io.Writer, lookupEnv lookupFn) int {
	c, code, ok := parseTeamsCmd(prog, "orgs show", "orgs show <org> [flags]", 1, args, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	s, err := c.client.GetOrganizationSummary(context.Background(), c.args[0])
	return c.finish(stdout, stderr, s, err, func() { printOrgSummary(stdout, s) })
}

func printOrgSummary(out io.Writer, s apiclient.OrganizationSummary) {
	o := s.Organization
	_, _ = fmt.Fprintf(out, "%s (%s)\n", o.Name, o.ID)
	if o.Slug != "" {
		_, _ = fmt.Fprintf(out, "slug:         %s\n", o.Slug)
	}
	if o.Description != "" {
		_, _ = fmt.Fprintf(out, "description:  %s\n", o.Description)
	}
	_, _ = fmt.Fprintf(out, "avatar:       %t\n\n", o.AvatarURL != "")
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	n := s.Counts
	_, _ = fmt.Fprintf(tw, "projects\t%d\napps\t%d\ndatabases\t%d\nenvironments\t%d\nmembers\t%d\npending invites\t%d\nfailing apps\t%d\n",
		n.Projects, n.Apps, n.Databases, n.Environments, n.Members, n.PendingInvites, s.FailingApps.Count)
	_ = tw.Flush()
	for _, name := range s.FailingApps.Names {
		_, _ = fmt.Fprintf(out, "  failing: %s\n", name)
	}
	if len(s.Activity) == 0 {
		return
	}
	_, _ = fmt.Fprintln(out, "\nRecent activity")
	for _, e := range s.Activity {
		_, _ = fmt.Fprintf(out, "  %s  %s  %s %s\n", e.CreatedAt, e.ActorName, e.Action, e.Subject)
	}
}

func runOrgsPatch(prog, verb, field string, args []string, stdout, stderr io.Writer, lookupEnv lookupFn) int {
	usage := fmt.Sprintf("orgs %s <org> <%s> [flags]", verb, field)
	c, code, ok := parseTeamsCmd(prog, "orgs "+verb, usage, 2, args, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	o, err := c.client.PatchOrganization(context.Background(), c.args[0], map[string]string{field: c.args[1]})
	return c.finish(stdout, stderr, o, err, func() {
		_, _ = fmt.Fprintf(stdout, "organization %s updated (%s)\n", o.ID, field)
	})
}

func runOrgsAvatar(prog string, args []string, stdout, stderr io.Writer, lookupEnv lookupFn) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, orgsUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "set":
		c, code, ok := parseTeamsCmd(prog, "orgs avatar set", "orgs avatar set <org> <file> [flags]", 2, args[1:], stderr, lookupEnv, nil)
		if !ok {
			return code
		}
		data, err := os.ReadFile(c.args[1])
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: read %s: %v\n", prog, c.args[1], err)
			return exitUsage
		}
		o, err := c.client.SetOrganizationAvatar(context.Background(), c.args[0], data)
		return c.finish(stdout, stderr, o, err, func() {
			_, _ = fmt.Fprintf(stdout, "avatar of %s updated\n", o.ID)
		})
	case "rm":
		var yes bool
		c, code, ok := parseTeamsCmd(prog, "orgs avatar rm", "orgs avatar rm <org> [--yes] [flags]", 1, args[1:], stderr, lookupEnv, func(fs *flag.FlagSet) {
			fs.BoolVar(&yes, "yes", false, "confirm removing the avatar")
		})
		if !ok {
			return code
		}
		if !yes {
			_, _ = fmt.Fprintf(stderr, "%s: orgs avatar rm removes the avatar of %s; pass --yes to confirm\n", prog, c.args[0])
			return exitUsage
		}
		err := c.client.DeleteOrganizationAvatar(context.Background(), c.args[0])
		return c.finish(stdout, stderr, map[string]bool{"removed": true}, err, func() {
			_, _ = fmt.Fprintf(stdout, "avatar of %s removed\n", c.args[0])
		})
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown orgs avatar subcommand %q\n\n%s", prog, args[0], orgsUsage(prog))
	return exitUsage
}
