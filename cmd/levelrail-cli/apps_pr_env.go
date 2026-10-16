package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runAppsPREnv dispatches "apps pr-env <verb>": per-pull-request env overrides
// for one app's preview (internal/api/preview_pr_env.go).
func runAppsPREnv(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsPREnvUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsPREnvUsage(prog))
		return exitOK
	case "list":
		return runAppsPREnvList(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runAppsPREnvSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "clear":
		return runAppsPREnvClear(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps pr-env subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsPREnvUsage(prog))
		return exitUsage
	}
}

func appsPREnvUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps pr-env list <name> --pr N [flags]                                 list one pull request's env overrides
  %[1]s apps pr-env set <name> <key> --pr N --value VALUE [--secret] [flags]   declare (or replace) an override for that pull request
  %[1]s apps pr-env clear <name> <id> --pr N [flags]                           remove one override by its id

Overrides win over the app's own env, "apps preview-env" and "apps branch-env"
for that one pull request's next preview deploy (push a commit to apply). They
are removed with the preview. --secret stores the value encrypted and never
shows it again, including in the pull request comment.
`, prog)
}

func prFlag(fs interface {
	IntVar(*int, string, int, string)
}, pr *int) {
	fs.IntVar(pr, "pr", 0, "pull request number (required)")
}

func runAppsPREnvList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps pr-env list", "print the overrides as JSON to stdout and nothing else", stderr)
	var pr int
	prFlag(fs, &pr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps pr-env list <name> --pr N [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps pr-env list", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	if pr <= 0 {
		_, _ = fmt.Fprintf(stderr, "%s: apps pr-env list requires --pr\n", prog)
		return exitUsage
	}
	overrides, err := client.ListPRPreviewEnv(context.Background(), name, pr)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list pr env overrides for app %q pr %d: %w", name, pr, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, overrides, func() { printPREnvHuman(stdout, overrides) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printPREnvHuman(stdout io.Writer, overrides []apiclient.PRPreviewEnvOverride) {
	if len(overrides) == 0 {
		_, _ = fmt.Fprintln(stdout, "no pull request env overrides")
		return
	}
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tKEY\tVALUE\tSECRET")
	for _, o := range overrides {
		value := o.Value
		if o.Secret {
			value = "(hidden)"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%v\n", o.ID, o.Key, value, o.Secret)
	}
	_ = tw.Flush()
}

func runAppsPREnvSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps pr-env set", "print the resulting override as JSON to stdout and nothing else", stderr)
	var pr int
	var value string
	var secret bool
	prFlag(fs, &pr)
	fs.StringVar(&value, "value", "", "the value for this pull request (required)")
	fs.BoolVar(&secret, "secret", false, "store this value encrypted; it is never returned again")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps pr-env set <name> <key> --pr N --value VALUE [--secret] [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	cmd := twoArgCmd{prog: prog, cmdLabel: "apps pr-env set", argsLabel: "an app name and an env var key"}
	client, name, key, jsonOut, of, exitCode, ok := parseTwoArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, cmd, lookupEnv)
	if !ok {
		return exitCode
	}
	if pr <= 0 || value == "" {
		_, _ = fmt.Fprintf(stderr, "%s: apps pr-env set requires --pr and --value\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	result, err := client.SetPRPreviewEnv(context.Background(), name, pr, key, value, secret)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set pr env override %q for app %q pr %d: %w", key, name, pr, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, result, func() {
		if result.Secret {
			_, _ = fmt.Fprintf(stdout, "%s pr %d: id=%s key=%s secret=true\n", name, pr, result.ID, result.Key)
			return
		}
		_, _ = fmt.Fprintf(stdout, "%s pr %d: id=%s key=%s value=%s\n", name, pr, result.ID, result.Key, result.Value)
	})
}

func runAppsPREnvClear(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps pr-env clear", "print {\"cleared\": true} as JSON to stdout on success and nothing else", stderr)
	var pr int
	prFlag(fs, &pr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps pr-env clear <name> <id> --pr N [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	cmd := twoArgCmd{prog: prog, cmdLabel: "apps pr-env clear", argsLabel: "an app name and an override id"}
	client, name, id, jsonOut, of, exitCode, ok := parseTwoArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, cmd, lookupEnv)
	if !ok {
		return exitCode
	}
	if pr <= 0 {
		_, _ = fmt.Fprintf(stderr, "%s: apps pr-env clear requires --pr\n", prog)
		return exitUsage
	}
	if err := client.DeletePRPreviewEnv(context.Background(), name, pr, id); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("clear pr env override %q for app %q pr %d: %w", id, name, pr, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, map[string]bool{"cleared": true}, func() {
		_, _ = fmt.Fprintf(stdout, "pull request env override %q cleared for app %q pr %d\n", id, name, pr)
	})
}
