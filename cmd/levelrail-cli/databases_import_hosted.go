package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const hostedFrozenRefusal = "writes to the source are not frozen: put the old app in maintenance mode and pass --writes-frozen before moving"

// runDatabasesImportHosted implements "databases import-hosted <database>".
// The source URL carries a password, so it comes from an environment variable
// or stdin and never from argv, which other users can read in a process list.
func runDatabasesImportHosted(prog string, args []string, stdin io.Reader, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var f dataFlags
	var fromEnv string
	var fromStdin, frozen, large, dryRun, wait bool
	fs := flag.NewFlagSet(prog+" databases import-hosted", flag.ContinueOnError)
	fs.SetOutput(stderr)
	f.register(fs)
	fs.StringVar(&fromEnv, "from-env", "", "name of the environment variable holding the source postgres:// URL")
	fs.BoolVar(&fromStdin, "from-stdin", false, "read the source postgres:// URL from the first line of stdin")
	fs.BoolVar(&frozen, "writes-frozen", false, "confirm the old app is in maintenance mode and nothing writes to the source")
	fs.BoolVar(&large, "allow-large", false, "accept the downtime of a dump and restore above the size ceiling")
	fs.BoolVar(&dryRun, "dry-run", false, "run the preflight only and change nothing")
	fs.BoolVar(&wait, "wait", true, "wait for the move and its verification to finish")
	positional, code, ok := parseInterspersed(fs, args)
	if !ok {
		return code
	}
	if len(positional) != 1 || (fromEnv == "") == !fromStdin {
		_, _ = fmt.Fprint(stderr, importHostedUsage(prog))
		return exitUsage
	}
	name := positional[0]

	raw := ""
	if fromStdin {
		line, _ := bufio.NewReader(stdin).ReadString('\n')
		raw = strings.TrimSpace(line)
	} else if v, has := lookupEnv(fromEnv); has {
		raw = strings.TrimSpace(v)
	}
	if raw == "" {
		return reportError(stdout, stderr, f.jsonOut, newValidationError("the source URL is empty: set %s or pipe it on stdin", sourceHint(fromEnv)))
	}
	if !dryRun && !frozen {
		return reportError(stdout, stderr, f.jsonOut, newValidationError("%s\nfor Heroku: heroku maintenance:on -a APP", hostedFrozenRefusal))
	}

	req := apiclient.HostedImportRequest{URL: raw, WritesFrozen: frozen, AllowLarge: large}
	client := apiClientFromFlags(prog, f.apiURL, "", f.profile, lookupEnv)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	pre, err := client.PreflightHostedImport(ctx, name, req)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("preflight %s: %w", name, err))
	}
	if dryRun {
		if f.jsonOut {
			if c := printJSONOrFail(stdout, stderr, pre); c != exitOK {
				return c
			}
		} else {
			printHostedPreflight(stdout, pre)
		}
		if pre.Blocked {
			return exitCheckFailed
		}
		return exitOK
	}
	if pre.Blocked {
		if f.jsonOut {
			_ = printJSONOrFail(stdout, stderr, pre)
		} else {
			printHostedPreflight(stdout, pre)
			_, _ = fmt.Fprintln(stderr, "not moving: fix the blockers above and run again")
		}
		return exitCheckFailed
	}

	st, err := client.StartHostedImport(ctx, name, req)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("import into %s: %w", name, err))
	}
	if wait {
		if st, err = waitForCopy(ctx, client, name, st, stderr, f.jsonOut); err != nil {
			return reportError(stdout, stderr, f.jsonOut, err)
		}
	}
	if f.jsonOut {
		if c := printJSONOrFail(stdout, stderr, st); c != exitOK {
			return c
		}
	} else {
		printDataCopy(stdout, []apiclient.DataCopyStatus{st}, true, f.verbose)
	}
	if st.Status == "failed" {
		return exitCheckFailed
	}
	return exitOK
}

func sourceHint(fromEnv string) string {
	if fromEnv == "" {
		return "--from-env VAR"
	}
	return fromEnv
}

func printHostedPreflight(w io.Writer, pre apiclient.HostedPreflight) {
	_, _ = fmt.Fprintf(w, "Source: %s\nTarget: %s (postgres %s)\n", pre.Source, pre.Database, pre.TargetVersion)
	for _, g := range []struct{ severity, label string }{{"block", "BLOCKERS"}, {"warn", "WARNINGS"}, {"ok", "INFO"}} {
		first := true
		for _, c := range pre.Checks {
			if c.Severity != g.severity {
				continue
			}
			if first {
				_, _ = fmt.Fprintf(w, "\n%s\n", g.label)
				first = false
			}
			_, _ = fmt.Fprintf(w, "  [%s] %s\n", c.ID, c.Message)
			if c.NextAction != "" {
				_, _ = fmt.Fprintf(w, "      fix: %s\n", c.NextAction)
			}
		}
	}
	if pre.Blocked {
		_, _ = fmt.Fprintln(w, "\nBlocked: resolve the blockers before moving.")
	} else {
		_, _ = fmt.Fprintln(w, "\nReady to move.")
	}
}

func importHostedUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases import-hosted <database> (--from-env VAR | --from-stdin) [--writes-frozen] [--allow-large] [--dry-run] [--json] [flags]

Copies a hosted Postgres (for example Heroku) into an existing managed postgres database.
Create the target first with "%[1]s databases create", then import. The source URL is never
accepted as an argument: export it, then pass the variable name, or pipe it on stdin.

  export SOURCE_DATABASE_URL="$(heroku config:get DATABASE_URL -a APP)"
  %[1]s databases import-hosted mydb --from-env SOURCE_DATABASE_URL --writes-frozen --dry-run
  %[1]s databases import-hosted mydb --from-env SOURCE_DATABASE_URL --writes-frozen
`, prog)
}
