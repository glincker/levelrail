package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func settingsBetaFeaturesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s settings beta-features list [flags]                      list every gated feature and its state
  %[1]s settings beta-features set <feature> on|off [--restart]  switch a feature, saved at once

Features wired at server start show "restart to apply" until the control
plane restarts; --restart asks it to restart right away (root, supervised installs).
`, prog)
}

func runSettingsBetaFeatures(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, settingsBetaFeaturesUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, settingsBetaFeaturesUsage(prog))
		return exitOK
	case "list":
		return runListCommand(prog, args[1:], stdout, stderr, lookupEnv, listCommandParams[[]apiclient.BetaFeature]{
			cmdLabel:  "settings beta-features list",
			jsonUsage: "print the features as JSON to stdout and nothing else",
			usageText: fmt.Sprintf("Usage:\n  %s settings beta-features list [flags]\n\nFlags:\n", prog),
			fetch: func(c *Client, ctx context.Context) ([]apiclient.BetaFeature, error) {
				return c.ListBetaFeatures(ctx)
			},
			errVerb: "list beta features",
			print:   printBetaFeatures,
		})
	case "set":
		return runSettingsBetaFeatureSwitch(prog, args[1:], stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown settings beta-features subcommand %q\n\n", prog, args[0])
	_, _ = fmt.Fprint(stderr, settingsBetaFeaturesUsage(prog))
	return exitUsage
}

func printBetaFeatures(out io.Writer, list []apiclient.BetaFeature) {
	for _, f := range list {
		state := "off"
		switch {
		case f.Enabled && f.EnabledBy == "env":
			state = "on (APP_EXPERIMENTAL)"
		case f.Enabled:
			state = "on (settings)"
		}
		how := "applies live"
		if f.PendingRestart {
			how = "restart to apply"
		} else if f.RequiresRestart {
			how = "applies at server start"
		}
		_, _ = fmt.Fprintf(out, "%-20s %-12s %-22s %s\n", f.ID, f.Maturity, state, how)
	}
}

func runSettingsBetaFeatureSwitch(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "settings beta-features set", "print the feature state as JSON to stdout and nothing else", stderr)
	restart := fs.Bool("restart", false, "restart the control plane afterwards when the change needs it")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, settingsBetaFeaturesUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if fs.NArg() != 2 || (fs.Arg(1) != "on" && fs.Arg(1) != "off") {
		_, _ = fmt.Fprint(stderr, settingsBetaFeaturesUsage(prog))
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	id, enable := fs.Arg(0), fs.Arg(1) == "on"
	res, err := client.SetBetaFeature(context.Background(), id, enable)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set beta feature %q: %w", id, err))
	}
	if *restart && res.PendingRestart {
		if _, rerr := client.RestartControlPlane(context.Background()); rerr != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("saved %s, but restarting the control plane failed: %w", id, rerr))
		}
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		switch {
		case res.Locked:
			_, _ = fmt.Fprintf(stdout, "%s is on through the server setting; remove it there to turn it off\n", res.ID)
		case res.PendingRestart && *restart:
			_, _ = fmt.Fprintf(stdout, "%s saved, control plane restarting to apply it\n", res.ID)
		case res.PendingRestart:
			_, _ = fmt.Fprintf(stdout, "%s saved, restart the control plane to apply it (or rerun with --restart)\n", res.ID)
		case res.Enabled:
			_, _ = fmt.Fprintf(stdout, "%s is on\n", res.ID)
		default:
			_, _ = fmt.Fprintf(stdout, "%s is off\n", res.ID)
		}
	})
}
