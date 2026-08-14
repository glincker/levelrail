package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runAppsStatus implements "apps status <name>": GET
// /api/v1/apps/{name}/deploys (internal/api/deploys.go's own
// handleDeployHistory), the application controller's current stored
// reconcile conditions, the same data the web frontend's ConditionsPanel
// reads. Deliberately a separate command from "apps get" rather than
// folded into it: the two endpoints return genuinely different resource
// shapes (desired app state vs. observed reconcile status), and "apps
// get --json" already has a stable appResource contract a script might
// depend on, so changing its shape to also carry conditions would be a
// breaking change to that contract for no reason a new command doesn't
// solve just as well.
func runAppsStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs := flag.NewFlagSet(prog+" apps status", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var tokenFlag, apiURLFlag string
	var jsonOut bool
	fs.StringVar(&tokenFlag, "token", "", "API token (overrides "+envAPIToken+" and the credentials file)")
	fs.StringVar(&apiURLFlag, "api-url", "", "control plane API base URL (overrides "+envAPIURL+" and the credentials file, default "+defaultAPIURL+")")
	fs.BoolVar(&jsonOut, "json", false, "print conditions as a JSON array to stdout and nothing else")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps status <name> [flags]\n\nShows the application controller's current stored reconcile conditions\nfor one app (current status, not a deploy history log).\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}

	rest := fs.Args()
	if len(rest) != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: apps status requires exactly one app name\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name := rest[0]

	client := NewClient(resolveAPIURL(apiURLFlag, lookupEnv, prog), resolveToken(tokenFlag, lookupEnv, prog))

	conditions, err := client.GetDeployStatus(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get status for app %q: %w", name, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, conditions); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printConditionsHuman(stdout, conditions)
	return exitOK
}
