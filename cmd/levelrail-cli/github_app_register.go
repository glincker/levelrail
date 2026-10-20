package main

import (
	"fmt"
	"io"
	"net/url"
	"strings"
)

// gitHubAppRegisterURL is the shape --json prints for github-app register-url.
type gitHubAppRegisterURL struct {
	URL string `json:"url"`
}

// runGitHubAppRegisterURL prints the dashboard URL that starts the manifest
// registration flow for the chosen owner. The flow itself must run in a
// logged-in browser, so the CLI only builds the link.
func runGitHubAppRegisterURL(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "github-app register-url", "print {\"url\": ...} as JSON to stdout and nothing else", stderr)
	ownerFlag := fs.String("owner", "", "GitHub organization login to own the App (default: your personal account)")
	publicFlag := fs.Bool("public", false, "allow other accounts and organizations to install the App")
	nameFlag := fs.String("name", "", "App name (default: derived from the brand and primary domain)")
	instanceFlag := fs.String("instance-url", "", "GitHub Enterprise Server base URL (default: https://github.com)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s github-app register-url [flags]\n\nPrints the URL that starts the GitHub App registration flow. Open it in a\nbrowser signed in to the dashboard: GitHub's manifest flow is a real browser\nredirect, so the CLI cannot drive it. With --owner the App is created under\nthat organization, otherwise under your personal account. A private App can\nonly be installed on its owner.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	_, apiURLFlag, profileFlag, _, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	owner := strings.TrimSpace(*ownerFlag)
	if owner != "" && !validGitHubOrgLogin(owner) {
		_, _ = fmt.Fprintf(stderr, "%s: github-app register-url: --owner %q is not a valid GitHub organization login\n", prog, owner)
		return exitUsage
	}

	q := url.Values{}
	if owner != "" {
		q.Set("owner", owner)
	}
	if *publicFlag {
		q.Set("public", "true")
	}
	if n := strings.TrimSpace(*nameFlag); n != "" {
		q.Set("name", n)
	}
	if i := strings.TrimSpace(*instanceFlag); i != "" {
		q.Set("instance_url", i)
	}

	base := strings.TrimRight(resolveAPIURL(apiURLFlag, lookupEnv, prog, resolveProfile(profileFlag, lookupEnv)), "/")
	target := base + "/api/v1/github-app/register/start"
	if len(q) > 0 {
		target += "?" + q.Encode()
	}

	return writeScheduledTaskResult(stdout, stderr, of, gitHubAppRegisterURL{URL: target}, func() {
		_, _ = fmt.Fprintln(stdout, target)
	})
}

// validGitHubOrgLogin mirrors the server's owner check: alphanumerics and
// hyphens, 1 to 39 characters, no leading hyphen.
func validGitHubOrgLogin(s string) bool {
	if s == "" || len(s) > 39 || s[0] == '-' {
		return false
	}
	for _, c := range s {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-':
		default:
			return false
		}
	}
	return true
}
