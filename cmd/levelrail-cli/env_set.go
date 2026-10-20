package main

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// runEnvSet implements "env set <app> KEY=VALUE...": the plain-env counterpart of "env push" for a few values.
func runEnvSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonOutP, outputP, queryP := apiFlagSet(prog, "env set", "print the plan as JSON to stdout and nothing else", stderr)
	var apply bool
	fs.BoolVar(&apply, "apply", false, "restart the app now so the new env reaches the running container")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, envSetUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonOutP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) < 2 {
		_, _ = fmt.Fprintf(stderr, "%s: env set requires an app name and at least one KEY=VALUE\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name := rest[0]
	entries := make([]envFileEntry, 0, len(rest)-1)
	for _, kv := range rest[1:] {
		k, v, found := strings.Cut(kv, "=")
		if !found || k == "" {
			return reportError(stdout, stderr, jsonOut, newValidationError("%q is not KEY=VALUE", kv))
		}
		entries = append(entries, envFileEntry{Key: k, Value: v})
	}

	ctx := context.Background()
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	app, err := client.GetApp(ctx, name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get app %q: %w", name, err))
	}
	secretKeys, err := appSecretKeys(ctx, client, name, app.SecretEnv)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	if bound, berr := client.ListBoundEnv(ctx, name); berr == nil {
		for _, b := range bound {
			secretKeys = append(secretKeys, b.Key)
		}
	}
	plan := planEnvImport(app.Env, entries, secretKeys, false)
	result := envImportResult{App: name, Plan: plan}
	warnEnvSecrets(ctx, client, stderr, name, plan)
	if len(plan.Secret) > 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("%s is stored as a secret; change it with \"%s apps secrets set %s KEY VALUE\"", strings.Join(plan.Secret, ", "), prog, name))
	}
	if len(plan.New) > 0 || len(plan.Changed) > 0 {
		app.Env = plan.Merged
		if _, err := client.UpdateApp(ctx, name, app); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("update env for app %q: %w", name, err))
		}
		result.Applied = true
	}
	code := writeScheduledTaskResult(stdout, stderr, of, result, func() { printEnvImportHuman(stdout, name, result) })
	if result.Applied {
		hintOut := stdout
		if of.Format == outputJSON || of.Query != "" {
			hintOut = io.Discard
		}
		if applyCode := afterConfigWrite(ctx, client, prog, name, apply, hintOut, stderr); code == exitOK {
			code = applyCode
		}
	}
	return code
}

func envSetUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s env set <app> KEY=VALUE [KEY=VALUE...] [flags]

Sets plain environment variables on an app, creating or changing them.
Keys stored as secrets are refused, use "%[1]s apps secrets set" for those.
Changes reach the running container at the next restart, or now with --apply.

Flags:
  --apply             restart the app now so the new env reaches the running container
  --token string      API token (default: %[2]s env var, then the credentials file)
  --api-url string    control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string    named credentials profile to read (overrides APP_PROFILE, default "default")
  --json              print the plan as JSON to stdout, nothing else
  --output string     output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string      JMESPath expression to filter the result before printing
  -h, --help          show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
