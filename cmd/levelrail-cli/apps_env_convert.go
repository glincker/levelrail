package main

import (
	"context"
	"fmt"
	"io"
)

type envConvertResult struct {
	App       string `json:"app"`
	Key       string `json:"key"`
	Converted bool   `json:"converted"`
}

func runAppsEnvConvertToSecret(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps env convert-to-secret", "print the result as JSON", stderr)
	var appFlag string
	fs.StringVar(&appFlag, "app", "", "app name (alternative to the positional <name>)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps env convert-to-secret <name> <KEY> [flags]\n  %s apps env convert-to-secret <KEY> --app <name> [flags]\n\nMoves one plain variable into the encrypted secret store. The value\nis handled by the server and never printed.\n\nFlags:\n", prog, prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	var name, key string
	switch {
	case appFlag != "" && len(rest) == 1:
		name, key = appFlag, rest[0]
	case appFlag == "" && len(rest) == 2:
		name, key = rest[0], rest[1]
	default:
		_, _ = fmt.Fprintf(stderr, "%s: apps env convert-to-secret needs a KEY and an app (<name> <KEY> or <KEY> --app <name>)\n", prog)
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if err := client.ConvertEnvToSecret(context.Background(), name, key); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("convert %s to a secret on app %q: %w", key, name, err))
	}
	res := envConvertResult{App: name, Key: key, Converted: true}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		_, _ = fmt.Fprintf(stdout, "%s on %s is now a secret\n", key, name)
	})
}
