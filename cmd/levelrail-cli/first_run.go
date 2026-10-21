package main

import (
	"bufio"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
)

// notSignedInFix is the 401 message for a request sent with no token, naming the exact fix.
func notSignedInFix(prog, profile, apiURL string) string {
	return fmt.Sprintf("not signed in: no saved login for profile %q. Run: %s login --api-url %s (or pass --token, or set %s)", profile, prog, apiURL, envAPIToken)
}

// isLoopbackURL reports whether rawURL points at this machine, where password login over plain http is fine.
func isLoopbackURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	switch u.Hostname() {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

// firstRunLoginArgs picks the login flow a server can actually complete: the session cookie is Secure, so plain http off-host needs the device flow.
func firstRunLoginArgs(apiURL, profile string) []string {
	args := []string{"--api-url", apiURL, "--profile", profile}
	if strings.HasPrefix(apiURL, "http://") && !isLoopbackURL(apiURL) {
		args = append(args, "--device")
	}
	return args
}

// offerFirstRunLogin asks an interactive user with no saved login for a server URL and signs in, returning the new URL and token.
func offerFirstRunLogin(prog, profile, apiURL string, lookupEnv func(string) (string, bool), in io.Reader, stdout, stderr io.Writer) (newURL, token string, ok bool) {
	reader := bufio.NewReader(in)
	_, _ = fmt.Fprintf(stderr, "No saved login for profile %q. Sign in now? [Y/n] ", profile)
	answer, _ := reader.ReadString('\n')
	if a := strings.ToLower(strings.TrimSpace(answer)); a == "n" || a == "no" {
		return "", "", false
	}
	_, _ = fmt.Fprintf(stderr, "Server URL [%s]: ", apiURL)
	line, _ := reader.ReadString('\n')
	if u := strings.TrimSpace(line); u != "" {
		apiURL = u
	}
	if code := runAuthLogin(prog, firstRunLoginArgs(apiURL, profile), stdout, stderr, lookupEnv, in); code != exitOK {
		return "", "", false
	}
	creds, err := readCredentialsFile(prog, profile)
	if err != nil || creds.Token == "" {
		return "", "", false
	}
	return apiURL, creds.Token, true
}

// firstRunInteractive is true only with a real terminal on stdin and stderr, so scripts never prompt.
var firstRunInteractive = func() bool {
	return isTerminal(os.Stdin) && isTerminal(os.Stderr)
}
