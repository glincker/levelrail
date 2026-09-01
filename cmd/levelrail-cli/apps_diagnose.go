package main

import (
	"context"
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
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps diagnose", "print the diagnosis as JSON to stdout and nothing else", stderr)
	var deployFlag string
	fs.StringVar(&deployFlag, "deploy", "", "diagnose a specific past deploy attempt ID instead of the newest one")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps diagnose <name> [--deploy ID] [flags]\n\nExplains why an app's most recent deploy attempt failed, or why it's\ncrashlooping, using a deterministic pattern match over already-collected\nsignals (deploy attempt error, reconcile conditions, crashloop state).\nNever calls an external model and never changes anything.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps diagnose", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	diagnosis, err := client.DiagnoseApp(context.Background(), name, deployFlag)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("diagnose app %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, diagnosis, func() { printDiagnosisHuman(stdout, diagnosis) })
}
