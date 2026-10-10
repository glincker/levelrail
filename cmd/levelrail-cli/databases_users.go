package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func databasesUsersUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases users list <database> [flags]                  list roles with attributes and connection counts
  %[1]s databases users create <database> <name> [flags]         create a user (--preset read_only|read_write|owner)
  %[1]s databases users rotate <database> <name> [flags]         set a new random password, shown once
  %[1]s databases users disable <database> <name> [flags]        block login and end sessions
  %[1]s databases users enable <database> <name> [flags]         allow login again
  %[1]s databases users delete <database> <name> [flags]         drop the user, ownership goes to the admin role

Run "%[1]s databases users <subcommand> -h" for a subcommand's own flags.
`, prog)
}

// runDatabasesUsers dispatches "databases users <verb>".
func runDatabasesUsers(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, databasesUsersUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, databasesUsersUsage(prog))
		return exitOK
	case "list":
		return runDatabasesUsersList(prog, rest, stdout, stderr, lookupEnv)
	case "create":
		return runDatabasesUsersCreate(prog, rest, stdout, stderr, lookupEnv)
	case "rotate":
		return runDatabasesUsersRotate(prog, rest, stdout, stderr, lookupEnv)
	case "disable":
		return runDatabasesUsersLogin(prog, rest, stdout, stderr, lookupEnv, false)
	case "enable":
		return runDatabasesUsersLogin(prog, rest, stdout, stderr, lookupEnv, true)
	case "delete":
		return runDatabasesUsersDelete(prog, rest, stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown databases users subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, databasesUsersUsage(prog))
		return exitUsage
	}
}

func runDatabasesUsersList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseDBAccess(prog, "databases users list", "databases users list <database> [flags]", args, 1, "a database name", stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	users, err := c.client.ListDatabaseUsers(context.Background(), c.rest[0])
	if err != nil {
		return c.fail(fmt.Errorf("list users of database %q: %w", c.rest[0], err))
	}
	return c.result(users, func() { printDatabaseUsers(stdout, users) })
}

func printDatabaseUsers(out io.Writer, users []apiclient.DatabaseUser) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tKIND\tPRESET\tLOGIN\tCONNECTIONS\tLIMIT\tVALID UNTIL")
	for _, u := range users {
		login, limit, until := "yes", "unlimited", "never"
		if !u.CanLogin {
			login = "no"
		}
		if u.ConnectionLimit > 0 {
			limit = fmt.Sprint(u.ConnectionLimit)
		}
		if u.ValidUntil != "" {
			until = u.ValidUntil
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%s\t%s\n", u.Name, u.Kind, dashIfEmpty(u.Preset), login, u.Connections, limit, until)
	}
	_ = tw.Flush()
}

func runDatabasesUsersCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var preset string
	var limit, expiresDays int
	c, code, ok := parseDBAccess(prog, "databases users create", "databases users create <database> <name> [flags]", args, 2, "a database name and a user name", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&preset, "preset", "read_only", "read_only, read_write or owner (owner of this one database)")
		fs.IntVar(&limit, "connection-limit", 0, "maximum concurrent connections (default 20)")
		fs.IntVar(&expiresDays, "expires-in-days", 0, "expire the login after this many days (default never)")
	})
	if !ok {
		return code
	}
	req := apiclient.DatabaseUserCreateRequest{Name: c.rest[1], Preset: preset, ConnectionLimit: limit}
	if expiresDays > 0 {
		req.ExpiresAt = time.Now().Add(time.Duration(expiresDays) * 24 * time.Hour).UTC().Format(time.RFC3339)
	}
	out, err := c.client.CreateDatabaseUser(context.Background(), c.rest[0], req)
	if err != nil {
		return c.fail(fmt.Errorf("create user %q on database %q: %w", c.rest[1], c.rest[0], err))
	}
	return c.result(out, func() { printCredential(stdout, out.Credential, "user created") })
}

func printCredential(out io.Writer, c apiclient.DatabaseCredential, headline string) {
	_, _ = fmt.Fprintf(out, "%s: %s\n", headline, c.Username)
	_, _ = fmt.Fprintf(out, "password (shown once, not stored): %s\n", c.Password)
	_, _ = fmt.Fprintf(out, "from apps:  %s\n", c.InternalURL)
	if c.ExternalURL != "" {
		_, _ = fmt.Fprintf(out, "from outside: %s\n  %s\n", c.ExternalURL, c.ExternalNote)
	}
	if c.ExpiresAt != "" {
		_, _ = fmt.Fprintf(out, "expires: %s\n", c.ExpiresAt)
	}
}

func runDatabasesUsersRotate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseDBAccess(prog, "databases users rotate", "databases users rotate <database> <name> [flags]", args, 2, "a database name and a user name", stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	out, err := c.client.RotateDatabaseUser(context.Background(), c.rest[0], c.rest[1])
	if err != nil {
		return c.fail(fmt.Errorf("rotate user %q on database %q: %w", c.rest[1], c.rest[0], err))
	}
	return c.result(out, func() { printCredential(stdout, out, "password rotated") })
}

func runDatabasesUsersLogin(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), login bool) int {
	verb := "disable"
	if login {
		verb = "enable"
	}
	c, code, ok := parseDBAccess(prog, "databases users "+verb, "databases users "+verb+" <database> <name> [flags]", args, 2, "a database name and a user name", stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	if err := c.client.SetDatabaseUserLogin(context.Background(), c.rest[0], c.rest[1], login); err != nil {
		return c.fail(fmt.Errorf("%s user %q on database %q: %w", verb, c.rest[1], c.rest[0], err))
	}
	return c.result(map[string]any{"user": c.rest[1], "login": login}, func() {
		_, _ = fmt.Fprintf(stdout, "user %q login %sd\n", c.rest[1], verb)
	})
}

func runDatabasesUsersDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseDBAccess(prog, "databases users delete", "databases users delete <database> <name> [flags]", args, 2, "a database name and a user name", stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	if err := c.client.DeleteDatabaseUser(context.Background(), c.rest[0], c.rest[1]); err != nil {
		return c.fail(fmt.Errorf("delete user %q on database %q: %w", c.rest[1], c.rest[0], err))
	}
	return c.result(map[string]any{"deleted": c.rest[1]}, func() {
		_, _ = fmt.Fprintf(stdout, "user %q deleted\n", c.rest[1])
	})
}
