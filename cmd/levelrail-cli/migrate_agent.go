package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/migrationplan"
)

func migrateAgentUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s migrate agent start [--label L] [--control-plane-addr HOST:PORT]   create a single-use token and print the install command
  %[1]s migrate agent status [SESSION]                                     sessions, or one session with what the agent found
  %[1]s migrate agent verify SESSION APP [--target NAME=FILES:BYTES]       compare the app's volumes with the source; zero diffs complete data_verified
  %[1]s migrate agent reopen APP                                           undo the agent's data_verified (also reopens the steps after it)
  %[1]s migrate agent remove SESSION                                       revoke the session now; works even if the source host is unreachable

The agent is read only and never moves data in this release: it lists what runs on the source and
compares file counts and sizes. Databases are not dumped or counted by it.
`, prog)
}

func runMigrateAgent(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, migrateAgentUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, migrateAgentUsage(prog))
		return exitOK
	case "start":
		return runMigrateAgentStart(prog, args[1:], stdout, stderr, lookupEnv)
	case "status":
		return runMigrateAgentStatus(prog, args[1:], stdout, stderr, lookupEnv)
	case "verify":
		return runMigrateAgentVerify(prog, args[1:], stdout, stderr, lookupEnv)
	case "reopen":
		return runMigrateAgentReopen(prog, args[1:], stdout, stderr, lookupEnv)
	case "remove":
		return runMigrateAgentRemove(prog, args[1:], stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown migrate agent command %q\n\n%s", prog, args[0], migrateAgentUsage(prog))
	return exitUsage
}

func runMigrateAgentStart(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var f dataFlags
	var label, addr string
	fs := newDataFlagSet(prog, "agent start", stderr)
	f.register(fs)
	fs.StringVar(&label, "label", "", "a name for the source server")
	fs.StringVar(&addr, "control-plane-addr", "", "host:port the source dials, default is the advertised agent address")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	client := apiClientFromFlags(prog, f.apiURL, "", f.profile, lookupEnv)
	created, err := client.CreateMigrationAgentSession(context.Background(), label, addr)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, err)
	}
	if f.jsonOut {
		return printJSONOrFail(stdout, stderr, created)
	}
	_, _ = fmt.Fprintf(stdout, "Session %s created. The token below is shown once and works once, for %s.\n\n", created.ID, created.ExpiresAt.Format("2006-01-02 15:04 MST"))
	if created.Command == "" {
		_, _ = fmt.Fprintf(stdout, "No agent address is advertised. Re-run with --control-plane-addr HOST:PORT.\ntoken: %s\nCA fingerprint: %s\n", created.Token, created.CAFingerprint)
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "On the source server, as root, with the %s binary downloaded there:\n\n  %s\n\n", created.Binary, created.Command)
	_, _ = fmt.Fprintf(stdout, "The session ends on its own after %d hours. Remove it sooner with: %s migrate agent remove %s\n", created.SessionMaxHours, prog, created.ID)
	return exitOK
}

func runMigrateAgentStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var f dataFlags
	fs := newDataFlagSet(prog, "agent status", stderr)
	f.register(fs)
	positional, code, ok := parseInterspersed(fs, args)
	if !ok {
		return code
	}
	client := apiClientFromFlags(prog, f.apiURL, "", f.profile, lookupEnv)
	ctx := context.Background()
	if len(positional) == 0 {
		list, err := client.ListMigrationAgentSessions(ctx)
		if err != nil {
			return reportError(stdout, stderr, f.jsonOut, err)
		}
		if f.jsonOut {
			return printJSONOrFail(stdout, stderr, list)
		}
		if len(list) == 0 {
			_, _ = fmt.Fprintln(stdout, "No migration agent sessions.")
		}
		for _, s := range list {
			_, _ = fmt.Fprintf(stdout, "%s  %-10s %s\n", s.ID, s.State, s.Label)
		}
		return exitOK
	}
	d, err := client.GetMigrationAgentSession(ctx, positional[0])
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, err)
	}
	if f.jsonOut {
		return printJSONOrFail(stdout, stderr, d)
	}
	printAgentDetail(stdout, d)
	return exitOK
}

func printAgentDetail(w io.Writer, d apiclient.MigrationAgentDetail) {
	_, _ = fmt.Fprintf(w, "%s  %s  %s\n", d.Session.ID, d.Session.State, d.Session.Label)
	if d.Session.SessionExpiresAt != nil {
		_, _ = fmt.Fprintf(w, "ends by %s\n", d.Session.SessionExpiresAt.Format("2006-01-02 15:04 MST"))
	}
	inv := d.Inventory
	_, _ = fmt.Fprintf(w, "docker %s, %d containers, %d volumes\n", inv.DockerVersion, len(inv.Containers), len(inv.Volumes))
	for _, c := range inv.Containers {
		tag := ""
		if c.Unlisted {
			tag += " [not matched to an imported app]"
		}
		if c.DatabaseEngine != "" {
			tag += " [database: " + c.DatabaseEngine + " " + c.DatabaseVersion + ", copy by hand]"
		}
		_, _ = fmt.Fprintf(w, "  %s  %s  %s%s\n", c.Name, c.Image, c.State, tag)
	}
}

// parseTargetStat reads NAME=FILES:BYTES.
func parseTargetStat(v string) (apiclient.MigrationAgentVerifyTarget, error) {
	name, rest, ok := strings.Cut(v, "=")
	files, bytes, ok2 := strings.Cut(rest, ":")
	nf, err1 := strconv.ParseInt(files, 10, 64)
	nb, err2 := strconv.ParseInt(bytes, 10, 64)
	if !ok || !ok2 || name == "" || err1 != nil || err2 != nil || nf < 0 || nb < 0 {
		return apiclient.MigrationAgentVerifyTarget{}, fmt.Errorf("--target %q must look like NAME=FILES:BYTES", v)
	}
	return apiclient.MigrationAgentVerifyTarget{Name: name, Files: nf, Bytes: nb}, nil
}

func runMigrateAgentVerify(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var f dataFlags
	var targets listFlag
	var ackEmpty bool
	fs := newDataFlagSet(prog, "agent verify", stderr)
	f.register(fs)
	fs.Var(&targets, "target", "NAME=FILES:BYTES measured on this side for a volume stored on another node, repeatable. Typed numbers are not verified by the agent, so using this flag prevents data_verified from completing automatically")
	fs.BoolVar(&ackEmpty, "ack-empty", false, "accept volumes that are empty on the source (0 files, 0 bytes), otherwise they block automatic completion")
	positional, code, ok := parseInterspersed(fs, args)
	if !ok {
		return code
	}
	if len(positional) != 2 {
		_, _ = fmt.Fprintf(stderr, "usage: %s migrate agent verify SESSION APP [--target NAME=FILES:BYTES] [--ack-empty]\n", prog)
		return exitUsage
	}
	var stats []apiclient.MigrationAgentVerifyTarget
	for _, t := range targets {
		st, err := parseTargetStat(t)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: %v\n", prog, err)
			return exitUsage
		}
		stats = append(stats, st)
	}
	client := apiClientFromFlags(prog, f.apiURL, "", f.profile, lookupEnv)
	res, err := client.VerifyMigrationAgent(context.Background(), positional[0], positional[1], stats, ackEmpty)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, err)
	}
	if f.jsonOut {
		return printJSONOrFail(stdout, stderr, res)
	}
	for _, c := range res.Comparisons {
		from := ""
		if c.TargetFrom == "operator-supplied" {
			from = " (target numbers supplied by you, not measured)"
		}
		_, _ = fmt.Fprintf(stdout, "%-24s %-20s source %d files %d bytes, here %d files %d bytes%s %s\n", c.Name, c.Status, c.SourceFiles, c.SourceBytes, c.TargetFiles, c.TargetBytes, from, c.Detail)
	}
	if res.Advance.Advanced {
		_, _ = fmt.Fprintf(stdout, "\ndata_verified completed by the migration agent. Undo with: %s migrate agent reopen %s\n", prog, positional[1])
	} else {
		_, _ = fmt.Fprintf(stdout, "\ndata_verified not completed: %s\n", res.Advance.Reason)
	}
	if res.Databases > 0 {
		_, _ = fmt.Fprintf(stdout, "%d database(s) on the source are not checked by the agent: copy and verify them with migrate db-copy.\n", res.Databases)
	}
	return exitOK
}

func runMigrateAgentReopen(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var f dataFlags
	fs := newDataFlagSet(prog, "agent reopen", stderr)
	f.register(fs)
	positional, code, ok := parseInterspersed(fs, args)
	if !ok {
		return code
	}
	if len(positional) != 1 {
		_, _ = fmt.Fprintf(stderr, "usage: %s migrate agent reopen APP\n", prog)
		return exitUsage
	}
	client := apiClientFromFlags(prog, f.apiURL, "", f.profile, lookupEnv)
	c, err := client.StepMigrationChecklist(context.Background(), positional[0], migrationplan.StepDataVerified, apiclient.MigrationStepRequest{Action: "reset"})
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, err)
	}
	if f.jsonOut {
		return printJSONOrFail(stdout, stderr, c)
	}
	_, _ = fmt.Fprintf(stdout, "%s: data_verified reopened, next step %s\n", c.App, c.Current)
	return exitOK
}

func runMigrateAgentRemove(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var f dataFlags
	fs := newDataFlagSet(prog, "agent remove", stderr)
	f.register(fs)
	positional, code, ok := parseInterspersed(fs, args)
	if !ok {
		return code
	}
	if len(positional) != 1 {
		_, _ = fmt.Fprintf(stderr, "usage: %s migrate agent remove SESSION\n", prog)
		return exitUsage
	}
	client := apiClientFromFlags(prog, f.apiURL, "", f.profile, lookupEnv)
	if err := client.RevokeMigrationAgentSession(context.Background(), positional[0]); err != nil {
		return reportError(stdout, stderr, f.jsonOut, err)
	}
	if f.jsonOut {
		return printJSONOrFail(stdout, stderr, map[string]string{"revoked": positional[0]})
	}
	_, _ = fmt.Fprintf(stdout, "Session %s revoked. Its certificate and token no longer work, and a connected agent was told to stop.\nOn the source, run \"remove\" on the migrate binary to delete its unit, identity and binary.\n", positional[0])
	return exitOK
}
