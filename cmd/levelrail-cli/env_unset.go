package main

import (
	"context"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
)

type envUnsetResult struct {
	App     string   `json:"app"`
	Removed []string `json:"removed"`
	Missing []string `json:"missing"`
	Applied bool     `json:"applied"`
}

// runEnvUnset implements "env unset <app> KEY...": the inverse of "env set".
func runEnvUnset(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonOutP, outputP, queryP := apiFlagSet(prog, "env unset", "print the result as JSON to stdout and nothing else", stderr)
	var apply bool
	fs.BoolVar(&apply, "apply", false, "restart the app now so the running container loses the removed keys")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, envUnsetUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonOutP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) < 2 {
		_, _ = fmt.Fprintf(stderr, "%s: env unset requires an app name and at least one KEY\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name, keys := rest[0], rest[1:]
	for _, k := range keys {
		if k == "" || strings.Contains(k, "=") {
			return reportError(stdout, stderr, jsonOut, newValidationError("%q is not a key; pass names only, like KEY", k))
		}
	}

	ctx := context.Background()
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	app, err := client.GetApp(ctx, name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get app %q: %w", name, err))
	}
	if secret := slices.DeleteFunc(slices.Clone(keys), func(k string) bool { return !slices.Contains(app.SecretEnv, k) }); len(secret) > 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("%s is stored as a secret; remove it with \"%s apps secrets delete %s KEY\"", strings.Join(secret, ", "), prog, name))
	}
	if bound, berr := client.ListBoundEnv(ctx, name); berr == nil {
		for _, b := range bound {
			if slices.Contains(keys, b.Key) {
				return reportError(stdout, stderr, jsonOut, newValidationError("%s comes from a linked database or bucket; unlink it instead", b.Key))
			}
		}
	}

	result := envUnsetResult{App: name, Removed: []string{}, Missing: []string{}}
	env := maps.Clone(app.Env)
	for _, k := range keys {
		if _, set := env[k]; !set {
			result.Missing = append(result.Missing, k)
			continue
		}
		delete(env, k)
		result.Removed = append(result.Removed, k)
	}
	if len(result.Removed) > 0 {
		// A nil map still clears: UpdateApp always sends env, so the last key goes too.
		app.Env = env
		if _, err := client.UpdateApp(ctx, name, app); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("update env for app %q: %w", name, err))
		}
		result.Applied = true
	}
	code := writeScheduledTaskResult(stdout, stderr, of, result, func() { printEnvUnsetHuman(stdout, result) })
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

func printEnvUnsetHuman(w io.Writer, r envUnsetResult) {
	for _, k := range r.Removed {
		_, _ = fmt.Fprintf(w, "- %s (removed)\n", k)
	}
	for _, k := range r.Missing {
		_, _ = fmt.Fprintf(w, "= %s (was not set)\n", k)
	}
	if r.Applied {
		_, _ = fmt.Fprintf(w, "saved env for app %q (%d removed)\n", r.App, len(r.Removed))
		return
	}
	_, _ = fmt.Fprintf(w, "nothing to change for app %q\n", r.App)
}

func envUnsetUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s env unset <app> KEY [KEY...] [flags]

Removes plain environment variables from an app. Keys that are not set are
reported and skipped. Secrets are refused, use "%[1]s apps secrets delete"
for those; keys injected by a linked database or bucket go away when you
unlink it. The running container keeps the old values until the next
restart, or now with --apply.

Flags:
  --apply             restart the app now so the running container loses the removed keys
  --token string      API token (default: %[2]s env var, then the credentials file)
  --api-url string    control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string    named credentials profile to read (overrides APP_PROFILE, default "default")
  --json              print the result as JSON to stdout, nothing else
  --output string     output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string      JMESPath expression to filter the result before printing
  -h, --help          show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
