package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

func runRotationReplace(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, _, _, _ := apiFlagSet(prog, "apps secrets rotation replace", "unused for this subcommand", stderr)
	var (
		yes       bool
		valueFile string
	)
	fs.BoolVar(&yes, "yes", false, "confirm: replace the stored value now")
	fs.StringVar(&valueFile, "value-file", "", `file holding the new value, or "-" for stdin. Omit to generate one. Never pass a value as an argument.`)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps secrets rotation replace <name> <key> [--value-file PATH|-] --yes [flags]\n\nWrites a new value and keeps the old one as <key>_PREVIOUS for the grace\nwindow. The app is not restarted: redeploy it to pick the value up.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	rest, ok := requireArgs(fs, stderr, prog, "apps secrets rotation replace", "an app name and a key", 2)
	if !ok {
		return exitUsage
	}
	if !yes {
		_, _ = fmt.Fprintln(stderr, "replacing the value changes what the app reads on its next start, pass --yes to confirm")
		return exitUsage
	}
	value := ""
	if valueFile != "" {
		v, err := readSecretFileOrStdin(valueFile)
		if err != nil {
			return reportError(stdout, stderr, false, fmt.Errorf("read new value: %w", err))
		}
		if value = strings.TrimRight(v, "\r\n"); value == "" {
			_, _ = fmt.Fprintln(stderr, "the new value is empty")
			return exitUsage
		}
	}
	client := apiClientFromFlags(prog, *apiURLFlagP, *tokenFlagP, *profileFlagP, lookupEnv)
	p, err := client.RotateSecretValue(context.Background(), rest[0], rest[1], value)
	if err != nil {
		return reportError(stdout, stderr, false, fmt.Errorf("rotate %q on app %q: %w", rest[1], rest[0], err))
	}
	_, _ = fmt.Fprintf(stdout, "secret %q on app %q rotated at %s, redeploy the app to apply it\n", p.Key, rest[0], p.LastRotatedAt)
	return exitOK
}

func runVaultRotateCredential(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "vault rotate-credential", vaultJSONUsage, stderr)
	var credentialFile string
	fs.StringVar(&credentialFile, "credential-file", "", `file holding the new Vault token or AppRole secret ID, or "-" for stdin (required)`)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s vault rotate-credential --credential-file PATH|- [flags]\n\nReplaces the stored Vault credential and keeps the old one for the grace\nwindow. Never shows either value.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if credentialFile == "" {
		_, _ = fmt.Fprintln(stderr, "--credential-file is required")
		return exitUsage
	}
	raw, err := readSecretFileOrStdin(credentialFile)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("read new credential: %w", err))
	}
	credential := strings.TrimRight(raw, "\r\n")
	if credential == "" {
		_, _ = fmt.Fprintln(stderr, "the new credential is empty")
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	settings, err := client.RotateVaultCredential(context.Background(), credential)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("rotate vault credential: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, settings, func() { printVaultSettings(stdout, settings) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}
