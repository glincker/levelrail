package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"
)

// runFlags dispatches "flags <verb> [flags]" to one of
// create/list/get/set/delete: managing feature flags
// (internal/api/feature_flags.go), the boolean-plus-rollout-percentage
// value a running app's own code reads live via GET
// /api/v1/flags/evaluate/{key}, never baked into a container at create
// time. CRUD is scoped to an owning app the same way "apps
// scheduled-tasks" is, so every verb but "create" takes <app> as its
// first positional argument.
func runFlags(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, flagsUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, flagsUsage(prog))
		return exitOK
	case "create":
		return runFlagsCreate(prog, args[1:], stdout, stderr, lookupEnv)
	case "list":
		return runFlagsList(prog, args[1:], stdout, stderr, lookupEnv)
	case "get":
		return runFlagsGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runFlagsSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "delete":
		return runFlagsDelete(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown flags subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, flagsUsage(prog))
		return exitUsage
	}
}

func flagsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s flags create <app> --key KEY --name NAME [--description DESC] [--disabled] [--rollout PERCENT] [flags]
  %[1]s flags list <app> [flags]
  %[1]s flags get <app> <id> [flags]
  %[1]s flags set <app> <id> --name NAME [--description DESC] [--disabled] [--rollout PERCENT] [flags]
  %[1]s flags delete <app> <id> [flags]

Manage feature flags: a boolean (plus an optional gradual rollout
percentage) an app's own running code reads live via GET
/api/v1/flags/evaluate/{key}, using a read-scoped API token injected as a
secret env var. Changes take effect immediately, no redeploy or restart
needed, since a flag's value is never baked into a container. Key is
globally unique across the whole control plane and cannot be changed
after create. AbilityWrite-gated on create/set/delete, AbilityRead on
list/get.

Run "%[1]s flags <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func featureFlagFlags(fs *flag.FlagSet, name, description *string, disabled *bool, rollout *int) {
	fs.StringVar(name, "name", "", "human-readable name (required)")
	fs.StringVar(description, "description", "", "optional description")
	fs.BoolVar(disabled, "disabled", false, "create/save the flag disabled (default: enabled)")
	fs.IntVar(rollout, "rollout", 100, "rollout percentage, 0-100 (default: 100, fully on when enabled)")
}

func runFlagsCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "flags create", "print the created flag as JSON to stdout and nothing else", stderr)
	var key, name, description string
	var disabled bool
	var rollout int
	fs.StringVar(&key, "key", "", "the string an app looks up this flag by, globally unique (required)")
	featureFlagFlags(fs, &name, &description, &disabled, &rollout)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, flagsCreateUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	appName, ok := requireOneArg(fs, stderr, prog, "flags create", "app name")
	if !ok {
		return exitUsage
	}
	if key == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--key is required"))
	}
	if name == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--name is required"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	created, err := client.CreateFeatureFlag(context.Background(), appName, featureFlagRequest{
		Key: key, Name: name, Description: description, Enabled: !disabled, RolloutPercentage: rollout,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create feature flag for app %q: %w", appName, err))
	}

	return writeFeatureFlagResult(stdout, stderr, of, created, func() {
		_, _ = fmt.Fprintf(stdout, "feature flag %q (%s) created for app %q\n", created.Key, created.ID, appName)
	})
}

func flagsCreateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s flags create <app> --key KEY --name NAME [--description DESC] [--disabled] [--rollout PERCENT] [flags]

Creates a new feature flag owned by <app>. --key must be globally unique
across the whole control plane: it's the same string the evaluate route
(GET /api/v1/flags/evaluate/{key}) looks up by, and it cannot be changed
after create.

Flags:
  --key string            globally unique lookup key (required)
  --name string           human-readable name (required)
  --description string    optional description
  --disabled                 create the flag disabled (default: enabled)
  --rollout int              rollout percentage, 0-100 (default: 100)
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print the created flag as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runFlagsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "flags list", "print flags as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, flagsListUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	appName, ok := requireOneArg(fs, stderr, prog, "flags list", "app name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	flags, err := client.ListFeatureFlags(context.Background(), appName)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list feature flags for app %q: %w", appName, err))
	}

	return writeFeatureFlagResult(stdout, stderr, of, flags, func() { printFeatureFlagsTable(stdout, flags) })
}

func printFeatureFlagsTable(out io.Writer, flags []featureFlagResource) {
	if len(flags) == 0 {
		_, _ = fmt.Fprintln(out, "no feature flags")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tKEY\tNAME\tENABLED\tROLLOUT")
	for _, f := range flags {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%t\t%d%%\n", f.ID, f.Key, f.Name, f.Enabled, f.RolloutPercentage)
	}
	_ = tw.Flush()
}

func flagsListUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s flags list <app> [flags]

Lists the feature flags owned by <app>.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print flags as a JSON array to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

// appAndFlagIDLabel is requireArgs' argsLabel for "flags get/set/delete
// <app> <id>", the same "an app name and a resource id" shape
// appAndTaskIDLabel establishes for scheduled tasks.
const appAndFlagIDLabel = "an app name and a flag id"

func parseAppAndFlagID(fs *flag.FlagSet, args []string, stderr io.Writer, prog, cmdLabel string) (appName, id string, exitCode int, ok bool) {
	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return "", "", exitOK, false
		}
		return "", "", exitUsage, false
	}
	rest, argsOK := requireArgs(fs, stderr, prog, cmdLabel, appAndFlagIDLabel, 2)
	if !argsOK {
		return "", "", exitUsage, false
	}
	return rest[0], rest[1], exitOK, true
}

func runFlagsGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "flags get", "print the flag as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, flagsGetUsage(prog)) }

	appName, id, code, ok := parseAppAndFlagID(fs, args, stderr, prog, "flags get")
	if !ok {
		return code
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP
	format, ferr := resolveOutputFormat(jsonOut, *outputFlagP)
	if ferr != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %s\n", prog, ferr)
		return exitValidation
	}
	of := outputFlags{format, *queryFlagP}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	f, err := client.GetFeatureFlag(context.Background(), appName, id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get feature flag %q: %w", id, err))
	}

	return writeFeatureFlagResult(stdout, stderr, of, f, func() { printFeatureFlagHuman(stdout, f) })
}

func printFeatureFlagHuman(out io.Writer, f featureFlagResource) {
	_, _ = fmt.Fprintf(out, "id:          %s\n", f.ID)
	_, _ = fmt.Fprintf(out, "key:         %s\n", f.Key)
	_, _ = fmt.Fprintf(out, "app:         %s\n", f.ServiceName)
	_, _ = fmt.Fprintf(out, "name:        %s\n", f.Name)
	if f.Description != "" {
		_, _ = fmt.Fprintf(out, "description: %s\n", f.Description)
	}
	_, _ = fmt.Fprintf(out, "enabled:     %t\n", f.Enabled)
	_, _ = fmt.Fprintf(out, "rollout:     %d%%\n", f.RolloutPercentage)
}

func flagsGetUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s flags get <app> <id> [flags]

Shows one feature flag's full metadata.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the flag as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runFlagsSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "flags set", "print the updated flag as JSON to stdout and nothing else", stderr)
	var name, description string
	var disabled bool
	var rollout int
	featureFlagFlags(fs, &name, &description, &disabled, &rollout)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, flagsSetUsage(prog)) }

	appName, id, code, ok := parseAppAndFlagID(fs, args, stderr, prog, "flags set")
	if !ok {
		return code
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP
	format, ferr := resolveOutputFormat(jsonOut, *outputFlagP)
	if ferr != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %s\n", prog, ferr)
		return exitValidation
	}
	of := outputFlags{format, *queryFlagP}

	if name == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--name is required"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	updated, err := client.UpdateFeatureFlag(context.Background(), appName, id, featureFlagRequest{
		Name: name, Description: description, Enabled: !disabled, RolloutPercentage: rollout,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set feature flag %q: %w", id, err))
	}

	return writeFeatureFlagResult(stdout, stderr, of, updated, func() {
		_, _ = fmt.Fprintf(stdout, "feature flag %q updated (enabled=%t, rollout=%d%%), takes effect immediately\n", updated.Key, updated.Enabled, updated.RolloutPercentage)
	})
}

func flagsSetUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s flags set <app> <id> --name NAME [--description DESC] [--disabled] [--rollout PERCENT] [flags]

Replaces every editable field of an existing feature flag (a full
replace, not a partial patch): --name must be supplied on every call,
the same as the value already saved. Key cannot be changed. Takes effect
immediately for every caller of GET /api/v1/flags/evaluate/{key}, no
redeploy or restart needed.

Flags:
  --name string           human-readable name (required)
  --description string    optional description
  --disabled                 save the flag disabled (default: enabled)
  --rollout int              rollout percentage, 0-100 (default: 100)
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print the updated flag as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runFlagsDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "flags delete", "print {} to stdout on success instead of a plain confirmation", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, flagsDeleteUsage(prog)) }

	appName, id, code, ok := parseAppAndFlagID(fs, args, stderr, prog, "flags delete")
	if !ok {
		return code
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP
	format, ferr := resolveOutputFormat(jsonOut, *outputFlagP)
	if ferr != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %s\n", prog, ferr)
		return exitValidation
	}
	of := outputFlags{format, *queryFlagP}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if err := client.DeleteFeatureFlag(context.Background(), appName, id); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete feature flag %q: %w", id, err))
	}

	return writeFeatureFlagResult(stdout, stderr, of, map[string]any{}, func() {
		_, _ = fmt.Fprintf(stdout, "feature flag %q deleted\n", id)
	})
}

func flagsDeleteUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s flags delete <app> <id> [flags]

Deletes a feature flag. Any app still calling GET
/api/v1/flags/evaluate/{key} for it afterward gets a 404.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print {} to stdout on success instead of a plain confirmation
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

// writeFeatureFlagResult prints value according to of's resolved
// format/query, falling back to plainOut for the plain default path,
// the same success-output shape writeScheduledTaskResult establishes
// for a sibling command group.
func writeFeatureFlagResult(stdout, stderr io.Writer, of outputFlags, value any, plainOut func()) int {
	if err := renderResult(stdout, of.Format, of.Query, value, func() { plainOut() }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}
