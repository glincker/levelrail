package main

import (
	"context"
	"fmt"
	"io"
	"os"
)

// validateSpecMaxBytes matches the server's own body limit so an oversize file fails before any request.
const validateSpecMaxBytes = 1 << 20

func appsValidateSpecUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps validate-spec <app> --file PATH|- [flags]

Checks an app.yaml with the server's deploy-time rules, without deploying or
saving anything. Prints one line per issue and exits non-zero when invalid.
--file - reads standard input. Files over 1 MiB are rejected.
%[2]s`, prog, commonFlagsHelp)
}

func readSpecInput(path string, stdin io.Reader) ([]byte, error) {
	r := stdin
	if path != "-" {
		st, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if st.IsDir() {
			return nil, fmt.Errorf("%s is a directory", path)
		}
		f, err := os.Open(path) //nolint:gosec // operator-supplied CLI flag
		if err != nil {
			return nil, err
		}
		defer func() { _ = f.Close() }()
		r = f
	}
	data, err := io.ReadAll(io.LimitReader(r, validateSpecMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > validateSpecMaxBytes {
		return nil, fmt.Errorf("spec is larger than %d bytes", validateSpecMaxBytes)
	}
	return data, nil
}

func runAppsValidateSpec(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps validate-spec", "print the raw validation response as JSON to stdout and nothing else", stderr)
	file := fs.String("file", "", "path to an app.yaml, or - for standard input (required)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsValidateSpecUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "apps validate-spec", "an app name", 1)
	if !ok {
		return exitUsage
	}
	if *file == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--file is required"))
	}
	data, err := readSpecInput(*file, stdin)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, newValidationError("read spec: %v", err))
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.ValidateAppSpec(context.Background(), rest[0], string(data))
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("validate spec for %s: %w", rest[0], err))
	}
	if rerr := renderResult(stdout, of.Format, of.Query, res, func() {
		if res.Valid {
			_, _ = fmt.Fprintf(stdout, "valid (%d services)\n", res.Services)
			return
		}
		for _, is := range res.Issues {
			_, _ = fmt.Fprintf(stdout, "error %s: %s\n", is.Path, is.Message)
		}
	}); rerr != nil {
		_, _ = fmt.Fprintln(stderr, rerr)
		return exitCodeForError(rerr)
	}
	if !res.Valid {
		return exitValidation
	}
	return exitOK
}
