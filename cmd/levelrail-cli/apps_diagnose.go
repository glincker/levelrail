package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

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

	if redeploy && applyFix == 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("--redeploy needs --apply-fix N"))
	}
	ctx := context.Background()
	diagnosis, err := client.DiagnoseApp(ctx, name, deployFlag)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("diagnose app %q: %w", name, err))
	}

	if applyFix > 0 {
		return applyDiagnosisFix(ctx, client, prog, name, diagnosis, applyFix, inputs, redeploy, stdout, stderr, jsonOut)
	}
	return writeScheduledTaskResult(stdout, stderr, of, diagnosis, func() { printDiagnosisHuman(stdout, diagnosis) })
}

func applyDiagnosisFix(ctx context.Context, client *Client, prog, name string, d diagnosisResource, n int, inputs map[string]string, redeploy bool, stdout, stderr io.Writer, jsonOut bool) int {
	fix, found := apiclient.FindDiagnosisFix(d, n)
	if !found {
		return reportError(stdout, stderr, jsonOut, newValidationError("no fix number %d in this diagnosis", n))
	}
	if err := checkFixInputs(prog, name, n, fix, inputs); err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	fields, err := client.ApplyDiagnosisFix(ctx, name, fix, inputs, false)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("apply fix %d to %q: %w", n, name, err))
	}
	deployID := ""
	redeployed := redeploy && fix.Redeploy
	if redeployed {
		id, err := redeployAfterFix(ctx, client, name)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("fix %d was applied to %q but the redeploy failed: %w", n, name, err))
		}
		deployID = id
	}
	if jsonOut {
		return writeScheduledTaskResult(stdout, stderr, outputFlags{Format: "json"}, map[string]any{"applied": n, "fields": fields, "redeployed": redeployed, "deploy_id": deployID}, func() {})
	}
	_, _ = fmt.Fprintf(stdout, "applied fix %d to %s: %v\n", n, name, fields)
	switch {
	case redeployed:
		_, _ = fmt.Fprintf(stdout, "redeployed %s, deploy %s (watch: %s deploys wait %s)\n", name, deployID, prog, name)
	case fix.Redeploy:
		_, _ = fmt.Fprintf(stdout, "run a deploy for the change to take effect, or rerun with --redeploy\n")
	case redeploy:
		_, _ = fmt.Fprintf(stdout, "this fix needs no redeploy\n")
	}
	return exitOK
}

// checkFixInputs refuses before any write when a value the fix needs is
// missing, and prints the command that supplies it.
func checkFixInputs(prog, name string, n int, fix apiclient.DiagnosisFix, inputs map[string]string) error {
	var missing []string
	for _, ch := range fix.Changes {
		if ch.NeedsInput && inputs[ch.Field] == "" {
			missing = append(missing, ch.Field)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	var pairs []string
	for _, f := range missing {
		pairs = append(pairs, strings.TrimPrefix(f, "env.")+"=VALUE")
	}
	return newValidationError("fix %d needs a value for %s, so it cannot be applied automatically and nothing was changed.\nSet it with: %s env set %s %s --apply\nor rerun with: --input %s=VALUE",
		n, strings.Join(missing, ", "), prog, name, strings.Join(pairs, " "), missing[0])
}

// redeployAfterFix deploys the app's current image so the saved change
// reaches a fresh container, and returns the new deploy id.
func redeployAfterFix(ctx context.Context, client *Client, name string) (string, error) {
	app, err := client.GetApp(ctx, name)
	if err != nil {
		return "", err
	}
	res, err := client.DeployApp(ctx, name, app.Image, false)
	if err != nil {
		return "", err
	}
	if res.PendingApproval != nil {
		return "", fmt.Errorf("the deploy needs approval (%s) before it starts", res.PendingApproval.ID)
	}
	latest, err := client.GetDeploy(ctx, name, "latest")
	if err != nil {
		return "", err
	}
	return latest.ID, nil
}
