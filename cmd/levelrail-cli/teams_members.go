package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func membersUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s members list <team>                        members and their roles
  %[1]s members set-role <team> <user-id> <role>   owner, admin, developer or viewer
  %[1]s members remove <team> <user-id>            remove a member (the last owner cannot be removed)
  %[1]s members leave <team>                       leave a team yourself (signed in sessions only)
`, prog)
}

// runMembers dispatches "members <verb>", also reachable as "teams members".
func runMembers(prog string, args []string, stdout, stderr io.Writer, lookupEnv lookupFn) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, membersUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, membersUsage(prog))
		return exitOK
	case "list":
		c, code, ok := parseTeamsCmd(prog, "members list", "members list <team>", 1, rest, stderr, lookupEnv, nil)
		if !ok {
			return code
		}
		members, err := c.client.ListTeamMembers(context.Background(), c.args[0])
		return c.finish(stdout, stderr, members, err, func() {
			tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "USER ID\tEMAIL\tNAME\tROLE")
			for _, m := range members {
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", m.UserID, m.Email, m.DisplayName, m.Role)
			}
			_ = tw.Flush()
		})
	case "set-role":
		c, code, ok := parseTeamsCmd(prog, "members set-role", "members set-role <team> <user-id> <role>", 3, rest, stderr, lookupEnv, nil)
		if !ok {
			return code
		}
		err := c.client.SetTeamMemberRole(context.Background(), c.args[0], c.args[1], c.args[2])
		return c.finish(stdout, stderr, map[string]string{"role": c.args[2]}, err, func() {
			_, _ = fmt.Fprintf(stdout, "%s is now %s\n", c.args[1], c.args[2])
		})
	case "remove", "leave":
		usage, n := "members remove <team> <user-id>", 2
		if args[0] == "leave" {
			usage, n = "members leave <team>", 1
		}
		c, code, ok := parseTeamsCmd(prog, "members "+args[0], usage, n, rest, stderr, lookupEnv, nil)
		if !ok {
			return code
		}
		user := "me"
		if n == 2 {
			user = c.args[1]
		}
		err := c.client.RemoveTeamMember(context.Background(), c.args[0], user)
		return c.finish(stdout, stderr, map[string]bool{"removed": true}, err, func() {
			_, _ = fmt.Fprintf(stdout, "%s removed from %s\n", user, c.args[0])
		})
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown members subcommand %q\n\n%s", prog, args[0], membersUsage(prog))
	return exitUsage
}

func teamInvitesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s teams invites list <team>
  %[1]s teams invites create <team> --role ROLE [--email ADDR] [--expires-in-hours N]
        no --email makes a single use link anyone holding it can redeem
  %[1]s teams invites revoke <team> <invite-id>
  %[1]s teams invites accept <token>               join with the signed in profile's account
`, prog)
}

// runTeamInvites dispatches "teams invites <verb>".
func runTeamInvites(prog string, args []string, stdout, stderr io.Writer, lookupEnv lookupFn) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, teamInvitesUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, teamInvitesUsage(prog))
		return exitOK
	case "list":
		c, code, ok := parseTeamsCmd(prog, "teams invites list", "teams invites list <team>", 1, rest, stderr, lookupEnv, nil)
		if !ok {
			return code
		}
		invites, err := c.client.ListTeamInvites(context.Background(), c.args[0])
		return c.finish(stdout, stderr, invites, err, func() { printTeamInvites(stdout, invites) })
	case "create":
		var req apiclient.CreateTeamInviteRequest
		c, code, ok := parseTeamsCmd(prog, "teams invites create", "teams invites create <team> --role ROLE [--email ADDR]", 1, rest, stderr, lookupEnv, func(fs *flag.FlagSet) {
			fs.StringVar(&req.Role, "role", "developer", "admin, developer or viewer")
			fs.StringVar(&req.Email, "email", "", "bind the invite to this email")
			fs.IntVar(&req.ExpiresInHours, "expires-in-hours", 0, "shorter expiry than the instance default")
		})
		if !ok {
			return code
		}
		inv, err := c.client.CreateTeamInvite(context.Background(), c.args[0], req)
		return c.finish(stdout, stderr, inv, err, func() {
			printTeamInvites(stdout, []apiclient.TeamInviteResource{inv})
			_, _ = fmt.Fprintf(stdout, "\nShare once, it is not shown again:\n  %s\n", inviteShareText(inv))
		})
	case "revoke":
		c, code, ok := parseTeamsCmd(prog, "teams invites revoke", "teams invites revoke <team> <invite-id>", 2, rest, stderr, lookupEnv, nil)
		if !ok {
			return code
		}
		err := c.client.RevokeTeamInvite(context.Background(), c.args[0], c.args[1])
		return c.finish(stdout, stderr, map[string]bool{"revoked": true}, err, func() {
			_, _ = fmt.Fprintf(stdout, "invite %s revoked\n", c.args[1])
		})
	case "accept":
		c, code, ok := parseTeamsCmd(prog, "teams invites accept", "teams invites accept <token>", 1, rest, stderr, lookupEnv, nil)
		if !ok {
			return code
		}
		err := c.client.AcceptTeamInvite(context.Background(), apiclient.AcceptTeamInviteRequest{Token: c.args[0]})
		return c.finish(stdout, stderr, map[string]bool{"accepted": true}, err, func() {
			_, _ = fmt.Fprintln(stdout, "invite accepted")
		})
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown teams invites subcommand %q\n\n%s", prog, args[0], teamInvitesUsage(prog))
	return exitUsage
}

func inviteShareText(inv apiclient.TeamInviteResource) string {
	if inv.Link != "" && inv.Link[0] != '/' {
		return inv.Link
	}
	return "token " + inv.Token
}

func printTeamInvites(out io.Writer, invites []apiclient.TeamInviteResource) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tEMAIL\tROLE\tSTATUS\tEXPIRES")
	for _, i := range invites {
		email := i.Email
		if email == "" {
			email = "(link)"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", i.ID, email, i.Role, i.Status, i.ExpiresAt)
	}
	_ = tw.Flush()
}
