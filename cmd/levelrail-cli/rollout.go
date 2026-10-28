package main

import (
	"fmt"
	"io"
)

// rolloutVerbs maps each rollout verb onto the existing apps subcommand it forwards to.
// pause and resume are absent on purpose: no API holds a rollout open, freeze windows are cron based.
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
		_, _ = fmt.Fprintf(stderr, "%s: rollout %s is not available: there is no API to hold a rollout open. Use \"%s apps freeze\" for scheduled deploy windows or \"%s apps stop\" to stop an app.\n", prog, args[0], prog, prog)
		return exitUsage
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
  %[1]s rollout status <app> [flags]     wait for the latest deploy to converge (same as apps wait)
  %[1]s rollout history <app> [flags]    list deploy attempts, newest first (same as apps deploys list)
  %[1]s rollout undo <app> [flags]       roll back to the previous deploy (same as apps rollback)
  %[1]s rollout restart <app> [flags]    restart the app (same as apps restart)

Each verb forwards its flags to the apps command named above, so run
"%[1]s apps wait --help" and the like for the full flag list.

rollout pause and rollout resume do not exist: no API holds a rollout open.
For scheduled deploy windows see "%[1]s apps freeze".
`, prog)
}
