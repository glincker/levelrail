package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func settingsBetaFeaturesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s settings beta-features list [flags]              list every gated feature and how to turn it on
  %[1]s settings beta-features enable <feature> [flags]  switch a beta feature on (no restart)
  %[1]s settings beta-features disable <feature> [flags] switch a beta feature off

Features that are wired at startup are not switchable here: list shows the
APP_EXPERIMENTAL line to set for each, and enable prints the same instructions.
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
	case "enable", "disable":
		return runSettingsBetaFeatureSwitch(prog, args[0] == "enable", args[1:], stdout, stderr, lookupEnv)
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
		how := "set APP_EXPERIMENTAL=" + f.ID + " and restart"
		if f.Toggleable {
			how = "switchable with enable/disable"
		}
		_, _ = fmt.Fprintf(out, "%-20s %-12s %-22s %s\n", f.ID, f.Maturity, state, how)
	}
}

func runSettingsBetaFeatureSwitch(prog string, enable bool, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	verb := map[bool]string{true: "enable", false: "disable"}[enable]
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "settings beta-features "+verb, "print the feature state as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, settingsBetaFeaturesUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if fs.NArg() != 1 {
		_, _ = fmt.Fprint(stderr, settingsBetaFeaturesUsage(prog))
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	id := fs.Arg(0)
	res, err := client.SetBetaFeature(context.Background(), id, enable)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s beta feature %q: %w", verb, id, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		switch {
		case res.Locked:
			_, _ = fmt.Fprintf(stdout, "%s is on through APP_EXPERIMENTAL; remove it there to turn it off\n", res.ID)
		case res.Enabled:
			_, _ = fmt.Fprintf(stdout, "%s is on\n", res.ID)
		default:
			_, _ = fmt.Fprintf(stdout, "%s is off\n", res.ID)
		}
	})
}
