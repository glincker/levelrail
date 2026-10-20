package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func appsContextsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps contexts list <name> [flags]                              list per-context env and build overrides
  %[1]s apps contexts set <name> --context CTX [--env K=V] [--build-command CMD] [--publish DIR] [--build-arg K=V] [flags]   replace one context's overrides
  %[1]s apps contexts clear <name> --context CTX [flags]               remove one context's overrides
  %[1]s apps contexts show <name> [--context CTX] [--branch B] [flags] show the effective config and where each value came from

CTX is deploy-preview, branch-deploy or branch:<pattern>. Production uses the app's own settings.
Order of precedence: branch env override, then branch context, then context, then the app default.
`, prog)
}

func runAppsContexts(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsContextsUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsContextsUsage(prog))
		return exitOK
	case "list":
		return runAppsContextsList(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runAppsContextsSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "clear":
		return runAppsContextsClear(prog, args[1:], stdout, stderr, lookupEnv)
	case "show":
		return runAppsContextsShow(prog, args[1:], stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown apps contexts subcommand %q\n\n%s", prog, args[0], appsContextsUsage(prog))
	return exitUsage
}

func runAppsContextsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps contexts list", "print the contexts as JSON to stdout and nothing else", stderr)
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps contexts list", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	rows, err := client.ListAppContexts(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list contexts for app %q: %w", name, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, rows, func() {
		if len(rows) == 0 {
			_, _ = fmt.Fprintln(stdout, "no context overrides")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "CONTEXT\tENV VARS\tBUILD COMMAND\tPUBLISH\tBUILD ARGS")
		for _, r := range rows {
			_, _ = fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%d\n", r.Context, len(r.Env), r.BuildCommand, r.PublishDir, len(r.BuildArgs))
		}
		_ = tw.Flush()
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func kvPairs(items []string, flagName string) (map[string]string, error) {
	out := make(map[string]string, len(items))
	for _, it := range items {
		k, v, ok := strings.Cut(it, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("--%s %q must be KEY=VALUE", flagName, it)
		}
		out[k] = v
	}
	return out, nil
}

func runAppsContextsSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps contexts set", "print the saved context as JSON to stdout and nothing else", stderr)
	var ctxName, buildCmd, publish string
	var envs, buildArgs stringListFlag
	fs.StringVar(&ctxName, "context", "", "deploy-preview, branch-deploy or branch:<pattern> (required)")
	fs.Var(&envs, "env", "env var KEY=VALUE for this context (repeatable)")
	fs.StringVar(&buildCmd, "build-command", "", "static site build command for this context")
	fs.StringVar(&publish, "publish", "", "static site publish directory for this context")
	fs.Var(&buildArgs, "build-arg", "build arg KEY=VALUE for this context (repeatable)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps contexts set <name> --context CTX [--env K=V] [--build-command CMD] [--publish DIR] [--build-arg K=V] [flags]\n\nReplaces everything set for CTX on app <name>.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps contexts set", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	if ctxName == "" {
		_, _ = fmt.Fprintf(stderr, "%s: apps contexts set requires --context\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	env, err := kvPairs(envs, "env")
	if err == nil {
		var ba map[string]string
		if ba, err = kvPairs(buildArgs, "build-arg"); err == nil {
			cfg := apiclient.AppContextConfig{Context: ctxName, Env: env, BuildCommand: buildCmd, PublishDir: publish, BuildArgs: ba}
			var saved apiclient.AppContextConfig
			if saved, err = client.SetAppContext(context.Background(), name, cfg); err == nil {
				return writeScheduledTaskResult(stdout, stderr, of, saved, func() {
					_, _ = fmt.Fprintf(stdout, "%s: context %s saved (%d env vars)\n", name, saved.Context, len(saved.Env))
				})
			}
		}
	}
	return reportError(stdout, stderr, jsonOut, fmt.Errorf("set context %q for app %q: %w", ctxName, name, err))
}

func runAppsContextsClear(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps contexts clear", "print {\"cleared\": true} as JSON to stdout on success and nothing else", stderr)
	var ctxName string
	fs.StringVar(&ctxName, "context", "", "context to clear (required)")
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps contexts clear", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	if ctxName == "" {
		_, _ = fmt.Fprintf(stderr, "%s: apps contexts clear requires --context\n", prog)
		return exitUsage
	}
	if err := client.ClearAppContext(context.Background(), name, ctxName); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("clear context %q for app %q: %w", ctxName, name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, map[string]bool{"cleared": true}, func() {
		_, _ = fmt.Fprintf(stdout, "context %q cleared for app %q\n", ctxName, name)
	})
}

func runAppsContextsShow(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps contexts show", "print the effective config as JSON to stdout and nothing else", stderr)
	var ctxName, branch string
	fs.StringVar(&ctxName, "context", "production", "production, deploy-preview or branch-deploy")
	fs.StringVar(&branch, "branch", "", "branch name, to include branch overrides that match it")
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps contexts show", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	eff, err := client.GetEffectiveConfig(context.Background(), name, ctxName, branch)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("effective config for app %q: %w", name, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, eff, func() {
		tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "KEY\tVALUE\tSOURCE")
		for _, e := range eff.Env {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", e.Key, e.Value, e.Source)
		}
		_, _ = fmt.Fprintf(tw, "build command\t%s\t%s\n", eff.BuildCommand.Value, eff.BuildCommand.Source)
		_, _ = fmt.Fprintf(tw, "publish dir\t%s\t%s\n", eff.PublishDir.Value, eff.PublishDir.Source)
		for k, s := range eff.BuildArgs {
			_, _ = fmt.Fprintf(tw, "build arg %s\t%s\t%s\n", k, s.Value, s.Source)
		}
		_ = tw.Flush()
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}
