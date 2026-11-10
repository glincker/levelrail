package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runDatabasesParams dispatches "databases params show|set|unset|discard|apply|rollback <database>".
func runDatabasesParams(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, databasesParamsUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, databasesParamsUsage(prog))
		return exitOK
	case "show", "set", "unset", "discard", "apply", "rollback":
		return runDatabasesParamsVerb(prog, args[0], args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown databases params subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, databasesParamsUsage(prog))
		return exitUsage
	}
}

func databasesParamsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases params show <database>                   default, current and staged value of each parameter
  %[1]s databases params set <database> NAME=VALUE...      stage a change (no restart)
  %[1]s databases params unset <database> NAME...          stage a return to the engine default
  %[1]s databases params rollback <database>               stage the set that ran before the last apply
  %[1]s databases params discard <database>                drop the staged change
  %[1]s databases params apply <database> --yes            apply the staged change, restarts the database

Only allowlisted parameters with typed values are accepted. A staged change
also goes live on the next "databases start".
`, prog)
}

func runDatabasesParamsVerb(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	cmd := "databases params " + verb
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, cmd, "print the parameters as JSON to stdout and nothing else", stderr)
	var yes *bool
	if verb == "apply" {
		yes = fs.Bool("yes", false, "confirm the restart that applying causes")
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) == 0 {
		_, _ = fmt.Fprintf(stderr, "%s: %s: missing database name\n", prog, cmd)
		return exitUsage
	}
	name, extra := rest[0], rest[1:]
	change, err := paramsChange(verb, extra)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %s: %v\n", prog, cmd, err)
		return exitUsage
	}
	if verb == "apply" && !*yes {
		_, _ = fmt.Fprintf(stderr, "%s: %s: applying restarts %q, pass --yes to confirm\n", prog, cmd, name)
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	var res apiclient.DatabaseParams
	switch verb {
	case "show":
		res, err = client.GetDatabaseParams(ctx, name)
	case "discard":
		res, err = client.DiscardDatabaseParams(ctx, name)
	case "apply":
		res, err = client.ApplyDatabaseParams(ctx, name)
	default:
		res, err = client.StageDatabaseParams(ctx, name, change)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s for database %q: %w", cmd, name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printDatabaseParams(stdout, res) })
}

// paramsChange builds the staged change from positional NAME=VALUE or NAME arguments.
func paramsChange(verb string, extra []string) (apiclient.DatabaseParamsChange, error) {
	var c apiclient.DatabaseParamsChange
	switch verb {
	case "set":
		if len(extra) == 0 {
			return c, fmt.Errorf("give at least one NAME=VALUE")
		}
		c.Set = map[string]string{}
		for _, kv := range extra {
			k, v, ok := strings.Cut(kv, "=")
			if !ok || k == "" {
				return c, fmt.Errorf("%q is not NAME=VALUE", kv)
			}
			c.Set[k] = v
		}
	case "unset":
		if len(extra) == 0 {
			return c, fmt.Errorf("give at least one parameter name")
		}
		c.Unset = extra
	case "rollback":
		c.Rollback = true
	default:
		if len(extra) > 0 {
			return c, fmt.Errorf("unexpected arguments %v", extra)
		}
	}
	return c, nil
}

func printDatabaseParams(w io.Writer, res apiclient.DatabaseParams) {
	if !res.Supported {
		_, _ = fmt.Fprintf(w, "database %q (%s) takes no tuning parameters\n", res.Database, res.Engine)
		return
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tDEFAULT\tCURRENT\tPENDING")
	for _, p := range res.Params {
		cur, pend := p.Current, ""
		if cur == "" {
			cur = "(default)"
		}
		if p.Pending != nil {
			pend = *p.Pending
			if pend == "" {
				pend = "(default)"
			}
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", p.Name, p.Default, cur, pend)
	}
	_ = tw.Flush()
	if res.RestartRequired {
		_, _ = fmt.Fprintf(w, "\nA change is staged. Run \"databases params apply %s --yes\" or start the database to apply it.\n", res.Database)
	}
}
