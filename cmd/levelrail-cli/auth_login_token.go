package main

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// Reads one token line from stdin so CI and password managers never put it on argv or in shell history.
func runAuthLoginWithToken(prog, apiURLFlag, profileFlag string, stdin io.Reader, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), of outputFlags, jsonOut, reauth bool) int {
	line, rerr := bufio.NewReader(stdin).ReadString('\n')
	token := strings.TrimSpace(line)
	if token == "" {
		if rerr != nil && rerr != io.EOF {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("read token from stdin: %w", rerr))
		}
		return reportError(stdout, stderr, jsonOut, newValidationError("the token on stdin is empty: pipe it in, for example: %s login --with-token < token.txt", prog))
	}
	profile := resolveProfile(profileFlag, lookupEnv)
	apiURL := resolveAPIURL(apiURLFlag, lookupEnv, prog, profile)
	serverInfo, err := checkLoginServer(prog, profile, apiURL, reauth, stderr)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	info, err := verifyCredential(apiURL, token, durationEnv(lookupEnv, envVerifyTimeout, defaultVerifyTimeout))
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("the server did not accept the token from stdin: %w", err))
	}
	if err := writeCredentialsFile(prog, profile, withServerPin(credentials{APIURL: apiURL, Token: token, TokenExpiresAt: info.ExpiresAt}, serverInfo)); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("save credentials: %w", err))
	}
	result := map[string]any{"profile": profile, "api_url": apiURL, "identity": info.Name, "kind": info.Kind, "scope": info.Abilities, "saved": true}
	if err := renderResult(stdout, of.Format, of.Query, result, func() {
		dir, _ := configDir(prog)
		_, _ = fmt.Fprintf(stdout, "logged in as %q with the token from stdin, saved to %s/%s under profile %q\n", info.Name, dir, credentialsFileName, profile)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}
