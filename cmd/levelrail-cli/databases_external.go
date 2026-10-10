package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

type externalFlags struct {
	dataFlags
	req           apiclient.ExternalDatabaseRequest
	passwordStdin bool
	testOnly      bool
}

func (e *externalFlags) register(fs *flag.FlagSet) {
	e.dataFlags.register(fs)
	fs.StringVar(&e.req.Name, "name", "", "record name (default: derived from the container or host)")
	fs.StringVar(&e.req.Engine, "engine", "", "postgres, mysql, mariadb, mongodb or redis")
	fs.StringVar(&e.req.Host, "host", "", "database host or container name")
	fs.IntVar(&e.req.Port, "port", 0, "database port (default per engine)")
	fs.StringVar(&e.req.Username, "user", "", "database user")
	fs.StringVar(&e.req.Database, "database", "", "database name")
	fs.StringVar(&e.req.AuthDatabase, "auth-database", "", "MongoDB authentication database (default admin)")
	fs.StringVar(&e.req.TLSMode, "tls-mode", "", "disable, prefer or require (default per engine)")
	fs.StringVar(&e.req.Network, "network", "", "Docker network the database is reachable on")
	fs.StringVar(&e.req.NodeID, "node", "", "node that runs the connection helper (default: local)")
	fs.StringVar(&e.req.ProjectID, "project", "", "project to place the record in")
	fs.BoolVar(&e.passwordStdin, "password-stdin", false, "read the password from the first line of stdin")
}

func (e *externalFlags) readPassword(stdin io.Reader, lookupEnv func(string) (string, bool)) {
	switch v, has := lookupEnv("APP_EXTERNAL_DB_PASSWORD"); {
	case e.passwordStdin:
		line, _ := bufio.NewReader(stdin).ReadString('\n')
		e.req.Password = strings.TrimRight(line, "\r\n")
	case has:
		e.req.Password = v
	}
}

func printExternalDatabase(w io.Writer, d apiclient.ExternalDatabase) {
	_, _ = fmt.Fprintf(w, "%s  %s  %s:%d  user=%s db=%s tls=%s", d.Name, d.Engine, d.Host, d.Port, d.Username, d.Database, d.TLSMode)
	if d.Network != "" {
		_, _ = fmt.Fprintf(w, " network=%s", d.Network)
	}
	if d.Health != nil {
		_, _ = fmt.Fprintf(w, "  health=%s", d.Health.Status)
	}
	_, _ = fmt.Fprintln(w)
}

func printProbe(w io.Writer, p apiclient.ExternalDatabaseProbe) {
	_, _ = fmt.Fprintf(w, "status: %s (%d ms)\n", p.Status, p.LatencyMs)
	if p.Reason != "" {
		_, _ = fmt.Fprintf(w, "detail: %s\n", p.Reason)
	}
	if p.Version != "" {
		_, _ = fmt.Fprintf(w, "server: %s\n", p.Version)
	}
	if len(p.Databases) > 0 {
		_, _ = fmt.Fprintf(w, "databases: %s\n", strings.Join(p.Databases, ", "))
	}
}

// runDatabasesConnect implements "databases connect": record a database that
// already exists elsewhere, with no data movement.
func runDatabasesConnect(prog string, args []string, stdin io.Reader, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var f externalFlags
	fs := flag.NewFlagSet(prog+" databases connect", flag.ContinueOnError)
	fs.SetOutput(stderr)
	f.register(fs)
	fs.BoolVar(&f.testOnly, "test", false, "only test the connection, save nothing")
	if _, code, ok := parseInterspersed(fs, args); !ok {
		return code
	}
	if f.req.Engine == "" || f.req.Host == "" || (f.req.Name == "" && !f.testOnly) {
		_, _ = fmt.Fprintf(stderr, "usage: %s databases connect --name NAME --engine ENGINE --host HOST [--port N] [--user U] [--database DB] [--tls-mode M] [--network N] [--password-stdin] [--test] [--json]\n", prog)
		return exitUsage
	}
	f.readPassword(stdin, lookupEnv)
	client := apiClientFromFlags(prog, f.apiURL, "", f.profile, lookupEnv)
	ctx := context.Background()
	if f.testOnly {
		p, err := client.TestExternalDatabase(ctx, f.req)
		if err != nil {
			return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("test connection: %w", err))
		}
		return renderExternal(stdout, stderr, f.jsonOut, p, func() { printProbe(stdout, p) }, p.Status != "reachable" && p.Status != "slow")
	}
	d, err := client.ConnectExternalDatabase(ctx, f.req)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("connect database: %w", err))
	}
	return renderExternal(stdout, stderr, f.jsonOut, d, func() {
		printExternalDatabase(stdout, d)
		_, _ = fmt.Fprintln(stdout, "Connected. Nothing was moved or changed on the database itself.")
	}, false)
}

// runDatabasesAdopt implements "databases adopt": point a record at a
// database container that is already running on a node. The container is
// never stopped, restarted or changed.
func runDatabasesAdopt(prog string, args []string, stdin io.Reader, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var f externalFlags
	fs := flag.NewFlagSet(prog+" databases adopt", flag.ContinueOnError)
	fs.SetOutput(stderr)
	f.register(fs)
	container := fs.String("container", "", "name of the running database container to adopt")
	list := fs.Bool("list", false, "list adoptable containers on the node and exit")
	if _, code, ok := parseInterspersed(fs, args); !ok {
		return code
	}
	client := apiClientFromFlags(prog, f.apiURL, "", f.profile, lookupEnv)
	ctx := context.Background()
	if *list {
		cands, err := client.ListExternalDatabaseCandidates(ctx, f.req.NodeID)
		if err != nil {
			return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("list adoptable containers: %w", err))
		}
		return renderExternal(stdout, stderr, f.jsonOut, cands, func() {
			for _, c := range cands {
				_, _ = fmt.Fprintf(stdout, "%s  %s  %s  host=%s network=%s\n", c.Container, c.Engine, c.Image, c.SuggestedHost, c.Network)
			}
		}, false)
	}
	if *container == "" {
		_, _ = fmt.Fprintf(stderr, "usage: %s databases adopt --container C [--node N] [--name NAME] [--user U] [--database DB] [--password-stdin] [--json]   (or --list)\n", prog)
		return exitUsage
	}
	f.req.Container = *container
	f.readPassword(stdin, lookupEnv)
	d, err := client.AdoptExternalDatabase(ctx, f.req)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("adopt container %q: %w", *container, err))
	}
	return renderExternal(stdout, stderr, f.jsonOut, d, func() {
		printExternalDatabase(stdout, d)
		_, _ = fmt.Fprintln(stdout, "Adopted. The container was not stopped, restarted or modified.")
	}, false)
}

// runDatabasesProbe implements "databases probe <name>".
func runDatabasesProbe(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var f dataFlags
	fs := flag.NewFlagSet(prog+" databases probe", flag.ContinueOnError)
	fs.SetOutput(stderr)
	f.register(fs)
	pos, code, ok := parseInterspersed(fs, args)
	if !ok {
		return code
	}
	if len(pos) != 1 {
		_, _ = fmt.Fprintf(stderr, "usage: %s databases probe NAME [--json]\n", prog)
		return exitUsage
	}
	client := apiClientFromFlags(prog, f.apiURL, "", f.profile, lookupEnv)
	p, err := client.ProbeExternalDatabase(context.Background(), pos[0])
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("probe %q: %w", pos[0], err))
	}
	return renderExternal(stdout, stderr, f.jsonOut, p, func() { printProbe(stdout, p) }, p.Status != "reachable" && p.Status != "slow")
}

func renderExternal(stdout, stderr io.Writer, jsonOut bool, v any, text func(), failed bool) int {
	if jsonOut {
		if c := printJSONOrFail(stdout, stderr, v); c != exitOK {
			return c
		}
	} else {
		text()
	}
	if failed {
		return exitCheckFailed
	}
	return exitOK
}
