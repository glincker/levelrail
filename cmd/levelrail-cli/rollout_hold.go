package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runRolloutHold handles "rollout pause <app>" and "rollout resume <app>".
func runRolloutHold(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	label := "rollout " + verb
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, label, "print the rollout progress as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, rolloutUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, label, "app name")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	call := client.PauseRollout
	if verb == "resume" {
		call = client.ResumeRollout
	}
	p, err := call(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s for app %q: %w", label, name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, p, func() { printRolloutHold(stdout, name, verb, p) })
}

func printRolloutHold(w io.Writer, name, verb string, p apiclient.RolloutProgress) {
	if verb == "pause" {
		_, _ = fmt.Fprintf(w, "app %s: rollout will pause before its next wave (now at wave %d of %d); resume with \"rollout resume %s\"\n", name, p.Wave, p.Waves, name)
		return
	}
	_, _ = fmt.Fprintf(w, "app %s: rollout resumed at wave %d of %d\n", name, p.Wave, p.Waves)
}

// runRolloutAbort rolls the app back to its previous deploy. The pause hold is
// tied to the release being rolled out, so the rollback is never held by it.
func runRolloutAbort(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runApps(prog, append([]string{"rollback"}, args...), stdout, stderr, lookupEnv)
}
