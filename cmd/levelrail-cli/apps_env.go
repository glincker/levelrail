package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
)

func runAppsEnv(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsEnvUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsEnvUsage(prog))
		return exitOK
	case "import":
		return runAppsEnvImport(prog, args[1:], stdout, stderr, lookupEnv)
	case "export":
		return runAppsEnvExport(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps env subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsEnvUsage(prog))
		return exitUsage
	}
}

func appsEnvUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps env import <name> --file .env [--dry-run] [--keep-existing] [flags]   merge a .env file into an app's plain env vars
  %[1]s apps env export <name> [--out FILE] [flags]                               write an app's env vars as .env text

Export never includes secret values: secret keys are written empty with a
comment. Import never writes secrets either: a key that is already a secret
is skipped, use "%[1]s apps secrets set" for those.

Run "%[1]s apps env <subcommand> -h" for a subcommand's own flags.
`, prog)
}

// appSecretKeys returns the union of the app's declared secret env names
// and the keys held in its secret store.
func appSecretKeys(ctx context.Context, client *Client, name string, declared []string) ([]string, error) {
	stored, err := client.ListSecrets(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("list secrets for app %q: %w", name, err)
	}
	seen := map[string]bool{}
	var out []string
	for _, k := range declared {
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	for _, s := range stored {
		if !seen[s.Key] {
			seen[s.Key] = true
			out = append(out, s.Key)
		}
	}
	return out, nil
}

type envImportResult struct {
	App     string        `json:"app"`
	DryRun  bool          `json:"dry_run"`
	Applied bool          `json:"applied"`
	Plan    envImportPlan `json:"plan"`
}

func runAppsEnvImport(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps env import", "print the import plan as JSON to stdout and nothing else", stderr)
	var file string
	var dryRun, keepExisting, apply bool
	fs.BoolVar(&apply, "apply", false, "restart the app now so the new env reaches the running container")
	fs.StringVar(&file, "file", "", "path to the .env file to import (required)")
	fs.BoolVar(&dryRun, "dry-run", false, "show what would change without saving")
	fs.BoolVar(&keepExisting, "keep-existing", false, "leave keys that already exist with a different value untouched")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps env import <name> --file .env [flags]\n\nMerges a .env file into an app's plain env vars. Keys already set as\nsecrets are skipped. Env changes take effect on the next restart,\nor now with --apply.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, "apps env import", "app name")
	if !ok {
		return exitUsage
	}
	if file == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("apps env import requires --file"))
	}
	data, err := os.ReadFile(file) //nolint:gosec // operator-supplied local path, same trust boundary as every other --file flag in this CLI
	if err != nil {
		return reportError(stdout, stderr, jsonOut, newValidationError("read env file %q: %v", file, err))
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

	plan := planEnvImport(app.Env, parseEnvFileBytes(data), secretKeys, keepExisting)
	result := envImportResult{App: name, DryRun: dryRun, Plan: plan}

	willChange := len(plan.New) > 0 || (len(plan.Changed) > 0 && !keepExisting)
	if !dryRun && willChange {
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
		afterConfigWrite(ctx, client, prog, name, apply, hintOut, stderr)
	}
	return code
}

func printEnvImportHuman(w io.Writer, name string, r envImportResult) {
	rows := []struct {
		mark string
		desc string
		keys []string
	}{
		{"+", "new", r.Plan.New},
		{"~", "changed", r.Plan.Changed},
		{"=", "unchanged", r.Plan.Unchanged},
		{"!", "skipped, key is a secret", r.Plan.Secret},
	}
	for _, row := range rows {
		for _, k := range row.keys {
			_, _ = fmt.Fprintf(w, "%s %s (%s)\n", row.mark, k, row.desc)
		}
	}
	switch {
	case r.DryRun:
		_, _ = fmt.Fprintf(w, "dry run: nothing saved for app %q\n", name)
	case r.Applied:
		_, _ = fmt.Fprintf(w, "saved env for app %q (%d new, %d changed)\n", name, len(r.Plan.New), len(r.Plan.Changed))
	default:
		_, _ = fmt.Fprintf(w, "nothing to change for app %q\n", name)
	}
}

func runAppsEnvExport(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps env export", "unused for this subcommand", stderr)
	var out string
	fs.StringVar(&out, "out", "", "write to this file instead of stdout")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps env export <name> [--out FILE] [flags]\n\nWrites an app's env vars as .env text. Secret keys are written empty\nwith a comment, their values are never exported.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, _, _, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, "apps env export", "app name")
	if !ok {
		return exitUsage
	}

	ctx := context.Background()
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	app, err := client.GetApp(ctx, name)
	if err != nil {
		return reportError(stdout, stderr, false, fmt.Errorf("get app %q: %w", name, err))
	}
	secretKeys, err := appSecretKeys(ctx, client, name, app.SecretEnv)
	if err != nil {
		return reportError(stdout, stderr, false, err)
	}
	text := renderDotenv(app.Env, secretKeys)

	if out == "" {
		_, _ = io.WriteString(stdout, text)
		return exitOK
	}
	if err := os.WriteFile(out, []byte(text), 0o600); err != nil {
		return reportError(stdout, stderr, false, newValidationError("write %q: %v", out, err))
	}
	_, _ = fmt.Fprintf(stdout, "wrote %d line(s) to %s\n", strings.Count(text, "\n"), out)
	return exitOK
}
