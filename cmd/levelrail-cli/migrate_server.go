package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/datamigrate"
)

// runMigrateServer implements "migrate server": inventory a source server
// read-only, show the plan, and with --apply copy the selected databases.
func runMigrateServer(prog string, args []string, stdin io.Reader, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var f dataFlags
	var src apiclient.HubSource
	var passwordStdin, list, plan, apply, wait, local bool
	var session, only string
	fs := newDataFlagSet(prog, "server", stderr)
	f.register(fs)
	fs.StringVar(&src.Engine, "engine", "postgres", "source engine: postgres, mysql, mariadb or mongodb")
	fs.StringVar(&src.Host, "host", "", "source server host")
	fs.IntVar(&src.Port, "port", 0, "source server port (default per engine)")
	fs.StringVar(&src.User, "user", "", "source server user")
	fs.StringVar(&src.Container, "container", "", "read a database container on this node over its own Docker network")
	fs.BoolVar(&local, "local", false, "list database containers on the node and exit")
	fs.StringVar(&src.NodeID, "node", "", "node that should host the managed databases")
	fs.BoolVar(&src.TLS, "tls", false, "require TLS to the source")
	fs.BoolVar(&passwordStdin, "password-stdin", false, "read the source password from the first line of stdin")
	fs.BoolVar(&list, "list", false, "inventory the source server and print its databases")
	fs.BoolVar(&plan, "plan", false, "inventory and print the preflight plan, nothing is created")
	fs.BoolVar(&apply, "apply", false, "create the managed databases and copy the selected ones")
	fs.BoolVar(&wait, "wait", true, "with --apply, wait for every copy to finish")
	fs.StringVar(&session, "session", "", "continue an existing session instead of connecting again")
	fs.StringVar(&only, "only", "", "comma separated source databases to copy, default every selectable one")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if local {
		return runMigrateLocal(prog, f, src.NodeID, stdout, stderr, lookupEnv)
	}
	if session == "" && (src.Host == "" && src.Container == "" || (!list && !plan && !apply)) {
		_, _ = fmt.Fprintf(stderr, "usage: %s migrate server --engine postgres --host H --user U --password-stdin (--list | --plan | --apply)\n", prog)
		return exitUsage
	}
	switch v, has := lookupEnv(envMigrateSourcePassword); {
	case passwordStdin:
		line, _ := bufio.NewReader(stdin).ReadString('\n')
		src.Password = strings.TrimRight(line, "\r\n")
	case has:
		src.Password = v
	}

	client := apiClientFromFlags(prog, f.apiURL, "", f.profile, lookupEnv)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	var sess apiclient.HubSession
	var err error
	if session != "" {
		sess, err = client.GetMigrationSession(ctx, session)
	} else {
		sess, err = client.CreateMigrationSession(ctx, src)
	}
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("read source server: %w", err))
	}
	if only != "" {
		sess, err = selectOnly(ctx, client, sess, only)
		if err != nil {
			return reportError(stdout, stderr, f.jsonOut, err)
		}
	}

	if apply {
		sess, err = client.ApplyMigrationSession(ctx, sess.ID, src.Password)
		if err != nil {
			return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("start copy: %w", err))
		}
		if wait {
			sess, err = waitForSession(ctx, client, sess, stderr, f.jsonOut)
			if err != nil {
				return reportError(stdout, stderr, f.jsonOut, err)
			}
		}
	}
	if f.jsonOut {
		if c := printJSONOrFail(stdout, stderr, sess); c != exitOK {
			return c
		}
	} else {
		printHubSession(stdout, sess, list && !plan && !apply)
	}
	if sess.Summary.Failed > 0 || (!apply && plan && sess.Summary.Blocked > 0) {
		return exitCheckFailed
	}
	return exitOK
}

func selectOnly(ctx context.Context, client *Client, sess apiclient.HubSession, only string) (apiclient.HubSession, error) {
	want := map[string]bool{}
	for _, n := range strings.Split(only, ",") {
		want[strings.TrimSpace(n)] = true
	}
	sel := make([]apiclient.HubSelection, 0, len(sess.Items))
	for _, it := range sess.Items {
		on := want[it.SourceDB]
		sel = append(sel, apiclient.HubSelection{SourceDB: it.SourceDB, Selected: &on})
	}
	return client.SelectMigrationItems(ctx, sess.ID, sel)
}

func waitForSession(ctx context.Context, client *Client, sess apiclient.HubSession, stderr io.Writer, quiet bool) (apiclient.HubSession, error) {
	if !quiet {
		_, _ = fmt.Fprintf(stderr, "Copying %d database(s), session %s. Ctrl-C stops waiting, the copy keeps running.\n", sess.Summary.Selected, sess.ID)
	}
	for sess.Running {
		select {
		case <-ctx.Done():
			return sess, fmt.Errorf("stopped waiting, continue with: migrate server --session %s --list", sess.ID)
		case <-time.After(copyPollInterval):
		}
		next, err := client.GetMigrationSession(ctx, sess.ID)
		if err != nil {
			return sess, fmt.Errorf("read session %s: %w", sess.ID, err)
		}
		sess = next
	}
	return sess, nil
}

func printHubSession(w io.Writer, s apiclient.HubSession, listOnly bool) {
	_, _ = fmt.Fprintf(w, "Session %s: %s %s on %s:%d (step: %s)\n\n", s.ID, s.Engine, s.ServerVersion, s.Host, s.Port, s.Step)
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "SOURCE\tSIZE\tTABLES\tEXTENSIONS\tTARGET\tVERSION\tPLAN\tSTATUS")
	for _, it := range s.Items {
		plan := "skip"
		switch {
		case it.Selected && it.Preflight.Blocked:
			plan = "BLOCKED"
		case it.Selected:
			plan = "copy"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\t%s\t%s\t%s\n", it.SourceDB, datamigrate.HumanBytes(it.SizeBytes), it.Tables,
			strings.Join(it.Extensions, ","), it.TargetName, it.TargetVersion, plan, it.Status)
	}
	_ = tw.Flush()
	if listOnly {
		return
	}
	for _, it := range s.Items {
		for _, c := range it.Preflight.Checks {
			if c.Severity == "ok" {
				continue
			}
			_, _ = fmt.Fprintf(w, "\n%s [%s] %s\n", it.SourceDB, strings.ToUpper(c.Severity), c.Message)
			if c.NextAction != "" {
				_, _ = fmt.Fprintf(w, "  next: %s\n", c.NextAction)
			}
		}
		if it.Reason != "" {
			_, _ = fmt.Fprintf(w, "\n%s: %s\n", it.SourceDB, it.Reason)
		}
	}
	sum := s.Summary
	_, _ = fmt.Fprintf(w, "\n%d selected, %d verified, %d failed, %d blocked. Source untouched: only read-only statements run against it.\n", sum.Selected, sum.Verified, sum.Failed, sum.Blocked)
}

func runMigrateLocal(prog string, f dataFlags, nodeID string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	client := apiClientFromFlags(prog, f.apiURL, "", f.profile, lookupEnv)
	list, err := client.ListMigrationLocalSources(context.Background(), nodeID)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, err)
	}
	if f.jsonOut {
		return printJSONOrFail(stdout, stderr, list)
	}
	tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "CONTAINER\tENGINE\tNETWORK\tADDRESS\tNOTE")
	for _, s := range list {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s:%d\t%s\n", s.Container, s.Engine, s.Network, s.Host, s.Port, s.Problem)
	}
	_ = tw.Flush()
	return exitOK
}
