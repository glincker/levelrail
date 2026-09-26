package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runAppsDiagnose implements "apps diagnose <name> [--deploy <id>]
// [--apply-fix N]": the read-only, deterministic explanation from GET
// /api/v1/apps/{name}/diagnose, plus optionally applying one of its numbered
// fixes through the ordinary app update path.
func runAppsDiagnose(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps diagnose", "print the diagnosis as JSON to stdout and nothing else", stderr)
	var deployFlag string
	var applyFix int
	var redeploy bool
	inputs := stringMapFlag{}
	fs.StringVar(&deployFlag, "deploy", "", "diagnose a specific past deploy attempt ID instead of the newest one")
	fs.IntVar(&applyFix, "apply-fix", 0, "apply fix number N from the diagnosis by updating the app")
	fs.BoolVar(&redeploy, "redeploy", false, "with --apply-fix, redeploy after the change when the fix calls for it")
	fs.Var(inputs, "input", "with --apply-fix, a value the fix needs, as FIELD=VALUE (e.g. env.DATABASE_URL=...), repeatable")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps diagnose <name> [--deploy ID] [--apply-fix N [--input FIELD=VALUE] [--redeploy]] [flags]\n\nExplains why an app's most recent deploy attempt failed, or why it's\ncrashlooping, using deterministic rules over already-collected signals.\nNever calls an external model. Without --apply-fix nothing is changed.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps diagnose", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	ctx := context.Background()
	diagnosis, err := client.DiagnoseApp(ctx, name, deployFlag)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("diagnose app %q: %w", name, err))
	}

	if applyFix > 0 {
		return applyDiagnosisFix(ctx, client, name, diagnosis, applyFix, inputs, redeploy, stdout, stderr, jsonOut)
	}
	return writeScheduledTaskResult(stdout, stderr, of, diagnosis, func() { printDiagnosisHuman(stdout, diagnosis) })
}

func applyDiagnosisFix(ctx context.Context, client *Client, name string, d diagnosisResource, n int, inputs map[string]string, redeploy bool, stdout, stderr io.Writer, jsonOut bool) int {
	fix, found := apiclient.FindDiagnosisFix(d, n)
	if !found {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("no fix number %d in this diagnosis", n))
	}
	fields, err := client.ApplyDiagnosisFix(ctx, name, fix, inputs, redeploy)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("apply fix %d to %q: %w", n, name, err))
	}
	if jsonOut {
		return writeScheduledTaskResult(stdout, stderr, outputFlags{Format: "json"}, map[string]any{"applied": n, "fields": fields, "redeployed": redeploy && fix.Redeploy}, func() {})
	}
	_, _ = fmt.Fprintf(stdout, "applied fix %d to %s: %v\n", n, name, fields)
	if fix.Redeploy && !redeploy {
		_, _ = fmt.Fprintf(stdout, "run a deploy for the change to take effect, or rerun with --redeploy\n")
	}
	return exitOK
}
