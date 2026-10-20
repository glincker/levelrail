package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"golang.org/x/term"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// promptHidden reads one line without echo from the controlling terminal.
// It is a variable so tests can answer prompts; it never reads flags or env.
var promptHidden = func(stderr io.Writer, label string) (string, error) {
	fd := int(os.Stdin.Fd()) //nolint:gosec // a terminal file descriptor always fits an int
	if !term.IsTerminal(fd) {
		return "", errors.New("--reveal asks for your password interactively and needs a terminal; it never reads one from flags or the environment")
	}
	_, _ = fmt.Fprint(stderr, label)
	b, err := term.ReadPassword(fd)
	_, _ = fmt.Fprintln(stderr)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", strings.TrimSuffix(label, ": "), err)
	}
	return string(b), nil
}

// withStepUp runs call with the token owner's password, asking for the
// authenticator code too when the server says the account needs one.
func withStepUp[T any](stderr io.Writer, call func(*apiclient.StepUpProof) (T, error)) (T, error) {
	var zero T
	password, err := promptHidden(stderr, "Password: ")
	if err != nil {
		return zero, err
	}
	proof := &apiclient.StepUpProof{Password: password}
	out, err := call(proof)
	var apiErr *apiclient.APIError
	if errors.As(err, &apiErr) && apiErr.Code == "step_up_code_required" {
		code, perr := promptHidden(stderr, "Authenticator code: ")
		if perr != nil {
			return zero, perr
		}
		proof.Code = strings.TrimSpace(code)
		return call(proof)
	}
	return out, err
}

func runAppsEnvGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps env get", "print the result as JSON", stderr)
	var reveal bool
	fs.BoolVar(&reveal, "reveal", false, "show a sensitive or secret value; asks for your password (and authenticator code) interactively")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps env get <name> <KEY> [--reveal] [flags]\n\nPrints one variable. Sensitive and secret values stay masked unless\n--reveal is given; reveal needs a token with read:sensitive and\nsecrets:reveal, APP_SECRET_REVEAL_TOKENS=on on the server, and your\npassword typed at the prompt. Every reveal is audited.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "apps env get", "an app name and a KEY", 2)
	if !ok {
		return exitUsage
	}
	name, key := rest[0], rest[1]
	ctx := context.Background()
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	app, err := client.GetApp(ctx, name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get app %q: %w", name, err))
	}
	isSecret := slices.Contains(app.SecretEnv, key)
	value, inEnv := app.Env[key]
	if !inEnv && !isSecret {
		return reportError(stdout, stderr, jsonOut, newValidationError("app %q has no variable %q", name, key))
	}
	result := apiclient.RevealResult{Key: key, Value: value, Type: "plain"}
	if isSecret {
		result = apiclient.RevealResult{Key: key, Value: apiclient.MaskedEnvValue, Type: "secret"}
	}
	masked := result.Value == apiclient.MaskedEnvValue
	if reveal && masked {
		result, err = withStepUp(stderr, func(p *apiclient.StepUpProof) (apiclient.RevealResult, error) {
			if isSecret {
				return client.RevealSecret(ctx, name, key, p)
			}
			return client.RevealEnv(ctx, name, key, p)
		})
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("reveal %s on app %q: %w", key, name, err))
		}
	}
	return writeScheduledTaskResult(stdout, stderr, of, result, func() {
		_, _ = fmt.Fprintln(stdout, result.Value)
		if result.Value == apiclient.MaskedEnvValue {
			_, _ = fmt.Fprintf(stderr, "%s is sensitive; add --reveal to show it\n", key)
		}
	})
}

// exportRevealed fetches the full export after a step-up.
func exportRevealed(ctx context.Context, client *Client, name, format string, stderr io.Writer) (string, error) {
	data, err := withStepUp(stderr, func(p *apiclient.StepUpProof) ([]byte, error) {
		return client.ExportEnvRevealed(ctx, name, format, p)
	})
	if err != nil {
		return "", fmt.Errorf("export env for app %q: %w", name, err)
	}
	return string(data), nil
}
