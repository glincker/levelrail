package main

import (
	"fmt"
	"io"
)

// rolloutVerbs maps each rollout verb onto the existing apps subcommand it forwards to.
// pause and resume hold a rolling deploy open between waves; abort rolls back through apps rollback.
var rolloutVerbs = map[string][]string{
	"status":  {"wait"},
	"history": {"deploys", "list"},
	"undo":    {"rollback"},
	"restart": {"restart"},
}

func runRollout(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, rolloutUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, rolloutUsage(prog))
		return exitOK
	case "pause", "resume":
		return runRolloutHold(prog, args[0], args[1:], stdout, stderr, lookupEnv)
	case "abort":
		return runRolloutAbort(prog, args[1:], stdout, stderr, lookupEnv)
	case "waves":
		return runRolloutWaves(prog, args[1:], stdout, stderr, lookupEnv)
	case "state":
		return runRolloutState(prog, args[1:], stdout, stderr, lookupEnv)
	case "status":
		if len(args) > 1 && args[1] != "-h" && args[1] != "--help" {
			printRolloutSummary(prog, args[1:], stdout, lookupEnv)
		}
	}
	target, ok := rolloutVerbs[args[0]]
	if !ok {
		_, _ = fmt.Fprintf(stderr, "%s: unknown rollout subcommand %q\n\n%s", prog, args[0], rolloutUsage(prog))
		return exitUsage
	}
	forwarded := append(append([]string{}, target...), args[1:]...)
	return runApps(prog, forwarded, stdout, stderr, lookupEnv)
}

func rolloutUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s rollout status <app> [flags]     show rolling waves and replica counts, then wait for the
                                       latest deploy to converge (same as apps wait)
  %[1]s rollout waves <app> [--json]     show the rollout policy and the last rolling deploy's
                                       wave, updated, ready, unavailable and surge counts
  %[1]s rollout state <app> [--json]     show the health gate, traffic flip and hold countdown of the
                                       newest release (thresholds are the app's deploy-safety settings)
  %[1]s rollout pause <app>              hold a rolling deploy before its next wave
  %[1]s rollout resume <app>             let a paused rolling deploy continue
  %[1]s rollout abort <app> [flags]      roll the app back to its previous deploy (same as apps
                                       rollback), which also releases a paused rollout
  %[1]s rollout history <app> [flags]    list deploy attempts, newest first (same as apps deploys list)
  %[1]s rollout undo <app> [flags]       roll back to the previous deploy (same as apps rollback)
  %[1]s rollout restart <app> [flags]    restart the app (same as apps restart)

Each verb forwards its flags to the apps command named above, so run
"%[1]s apps wait --help" and the like for the full flag list.

A pause takes effect at the next wave boundary of a rolling deploy, so the
waves already started finish first. For scheduled deploy windows see "%[1]s apps freeze".
`, prog)
}
