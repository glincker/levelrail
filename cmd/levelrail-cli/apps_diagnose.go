package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runAppsDiagnose implements "apps diagnose <name> [--deploy <id>]": GET
// /api/v1/apps/{name}/diagnose (internal/api/diagnose.go's own
// handleDiagnoseApp), a read-only, deterministic explanation of the
// app's newest deploy failure or crashloop state, or a specific past
// attempt if --deploy is given. Never writes anything; the same
// read-and-suggest layer get_app_status/apps_status.go's own conditions
// view already establishes, just synthesized into a human explanation.
func runAppsDiagnose(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs := flag.NewFlagSet(prog+" apps diagnose", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var tokenFlag, apiURLFlag, deployFlag string
	var jsonOut bool
	fs.StringVar(&tokenFlag, "token", "", "API token (overrides "+envAPIToken+" and the credentials file)")
	fs.StringVar(&apiURLFlag, "api-url", "", "control plane API base URL (overrides "+envAPIURL+" and the credentials file, default "+defaultAPIURL+")")
	fs.StringVar(&deployFlag, "deploy", "", "diagnose a specific past deploy attempt ID instead of the newest one")
	fs.BoolVar(&jsonOut, "json", false, "print the diagnosis as JSON to stdout and nothing else")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps diagnose <name> [--deploy ID] [flags]\n\nExplains why an app's most recent deploy attempt failed, or why it's\ncrashlooping, using a deterministic pattern match over already-collected\nsignals (deploy attempt error, reconcile conditions, crashloop state).\nNever calls an external model and never changes anything.\n\nFlags:\n", prog)
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
		_, _ = fmt.Fprintf(stderr, "%s: apps diagnose requires exactly one app name\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name := rest[0]

	client := NewClient(resolveAPIURL(apiURLFlag, lookupEnv, prog), resolveToken(tokenFlag, lookupEnv, prog))

	diagnosis, err := client.DiagnoseApp(context.Background(), name, deployFlag)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("diagnose app %q: %w", name, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, diagnosis); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printDiagnosisHuman(stdout, diagnosis)
	return exitOK
}
