package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runSettingsVacation dispatches "settings vacation show|on|off", over
// /api/v1/settings/vacation-freeze (internal/api/vacation_freeze.go).
func runSettingsVacation(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, settingsVacationUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, settingsVacationUsage(prog))
		return exitOK
	case "show", "on", "off":
		return runSettingsVacationVerb(prog, args[0], args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown settings vacation subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, settingsVacationUsage(prog))
		return exitUsage
	}
}

func settingsVacationUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s settings vacation show [flags]
  %[1]s settings vacation on --days N [--reason TEXT] [flags]
  %[1]s settings vacation off [flags]

One switch that holds automatic deploys, scheduled jobs and held-deploy
replay for N days, then lifts itself. "off" lifts it early.
`, prog)
}

func runSettingsVacationVerb(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	label := "settings vacation " + verb
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, label, "print the vacation state as JSON to stdout and nothing else", stderr)
	var days int
	var reason string
	if verb == "on" {
		fs.IntVar(&days, "days", 0, "how many days to hold (required)")
		fs.StringVar(&reason, "reason", "", "shown on held deploys")
	}
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, settingsVacationUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if verb == "on" && days < 1 {
		return reportError(stdout, stderr, jsonOut, newValidationError("--days is required and must be at least 1"))
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	var (
		res apiclient.VacationFreezeResource
		err error
	)
	switch verb {
	case "show":
		res, err = client.GetVacationFreeze(ctx)
	case "on":
		res, err = client.SetVacationFreeze(ctx, days, reason)
	default:
		res, err = client.ClearVacationFreeze(ctx)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s: %w", label, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		if res.Active && res.Until != nil {
			_, _ = fmt.Fprintf(stdout, "vacation: ON until %s (%d days left)\n", res.Until.Format(time.RFC3339), res.DaysLeft)
			return
		}
		_, _ = fmt.Fprintln(stdout, "vacation: off")
	})
}
