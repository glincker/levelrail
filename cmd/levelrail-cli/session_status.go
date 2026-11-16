package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const (
	envExpiryWarnWindow     = "APP_EXPIRY_WARN_WINDOW"
	envVerifyTimeout        = "APP_VERIFY_TIMEOUT"
	defaultExpiryWarnWindow = 7 * 24 * time.Hour
	defaultVerifyTimeout    = 8 * time.Second
)

// sessionStatus is what "whoami" and "profile list --verify" report: where the
// credential came from, whether the server accepted it, and the exact fix.
type sessionStatus struct {
	Profile          string   `json:"profile"`
	ProfileSource    string   `json:"profile_source"`
	APIURL           string   `json:"api_url"`
	RepoPointer      string   `json:"repo_pointer,omitempty"`
	CredentialSource string   `json:"credential_source"`
	HasCredential    bool     `json:"has_credential"`
	Verified         bool     `json:"verified"`
	Usable           bool     `json:"usable"`
	Identity         string   `json:"identity,omitempty"`
	Kind             string   `json:"kind,omitempty"`
	Scope            []string `json:"scope,omitempty"`
	ExpiresAt        string   `json:"expires_at,omitempty"`
	ExpiringSoon     bool     `json:"expiring_soon,omitempty"`
	TrustedDevice    bool     `json:"trusted_device"`
	Problem          string   `json:"problem,omitempty"`
	NextCommand      string   `json:"next_command,omitempty"`
	RunBy            string   `json:"run_by,omitempty"`
}

// durationEnv reads a duration from env, falling back to def on absence or junk.
func durationEnv(lookupEnv func(string) (string, bool), key string, def time.Duration) time.Duration {
	if raw, ok := lookupEnv(key); ok {
		if d, err := time.ParseDuration(strings.TrimSpace(raw)); err == nil && d > 0 {
			return d
		}
	}
	return def
}

// humanLeft shows a remaining lifetime in days, or hours under two days, never as 2160h0m0s.
func humanLeft(d time.Duration) string {
	switch {
	case d >= 48*time.Hour:
		return fmt.Sprintf("%d days", int(d.Hours()/24))
	case d >= time.Hour:
		return fmt.Sprintf("%d hours", int(d.Hours()))
	case d > 0:
		return "under an hour"
	}
	return "expired"
}

// expiryNote is the human form of a stored expiry: the date and how long is left.
func expiryNote(raw string) string {
	exp, ok := parseExpiry(raw)
	if !ok {
		return raw
	}
	left := time.Until(exp)
	if left <= 0 {
		return raw + " (expired)"
	}
	return raw + " (in " + humanLeft(left) + ")"
}

// nextStepLine ends a rejected-credential error with the commands whoami reports.
func nextStepLine(prog, profile, apiURL string) string {
	return fmt.Sprintf("next: %s whoami --json shows the state; a person at a terminal runs: %s", prog, loginFix(prog, profile, apiURL))
}

func parseExpiry(raw string) (time.Time, bool) {
	if raw == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, raw)
	return t, err == nil
}

// loginFix is the one command a person runs to create a credential; agents never run it.
func loginFix(prog, profile, apiURL string) string {
	return fmt.Sprintf("%s login --api-url %s --profile %s", prog, apiURL, profile)
}

// verifyCredential asks the server who the token is with one cheap call.
func verifyCredential(apiURL, token string, timeout time.Duration) (apiclient.WhoamiResource, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return NewClient(apiURL, token).Whoami(ctx)
}

// collectStatus resolves profile, URL and credential the way every command does,
// then verifies live when verify is set. It never prints and never logs in.
func collectStatus(prog, profileFlag, apiURLFlag, tokenFlag string, verify bool, lookupEnv func(string) (string, bool)) sessionStatus {
	profile, profileSource := apiclient.ResolveProfileSource(profileFlag, lookupEnv)
	st := sessionStatus{
		Profile:       profile,
		ProfileSource: profileSource,
		APIURL:        resolveAPIURL(apiURLFlag, lookupEnv, prog, profile),
	}
	if p, ok := apiclient.FindRepoPointerFromCwd(); ok {
		st.RepoPointer = p.Path
	}
	token := resolveToken(tokenFlag, lookupEnv, prog, profile)
	st.CredentialSource = credentialSource(tokenFlag, prog, profile)
	st.HasCredential = token != ""
	creds, _ := readCredentialsFile(prog, profile)
	st.TrustedDevice = creds.TrustedDevice != ""
	if m, bad := savedLoginMismatch(apiURLFlag, lookupEnv, prog, profile); bad {
		st.Problem = m.Error()
		st.NextCommand = m.NextCommand()
		st.RunBy = "a person at a terminal, only if the pointer address is the one meant"
		return st
	}
	if exp, ok := parseExpiry(creds.TokenExpiresAt); ok && strings.HasPrefix(st.CredentialSource, "profile") {
		st.ExpiresAt = creds.TokenExpiresAt
		st.ExpiringSoon = time.Until(exp) < durationEnv(lookupEnv, envExpiryWarnWindow, defaultExpiryWarnWindow)
	}
	if !st.HasCredential {
		st.Problem = fmt.Sprintf("no credential for profile %q at %s", profile, st.APIURL)
		st.NextCommand = loginFix(prog, profile, st.APIURL)
		st.RunBy = "a person at a terminal (login can need a password and a new-device approval, an agent must not do either)"
		return st
	}
	if !verify {
		st.Usable = true
		return st
	}
	info, err := verifyCredential(st.APIURL, token, durationEnv(lookupEnv, envVerifyTimeout, defaultVerifyTimeout))
	if err != nil {
		st.Problem, st.NextCommand, st.RunBy = classifyVerifyError(prog, st, err)
		return st
	}
	st.Verified, st.Usable = true, true
	st.Identity, st.Kind, st.Scope = info.Name, info.Kind, info.Abilities
	if info.ExpiresAt != "" {
		st.ExpiresAt = info.ExpiresAt
		if exp, ok := parseExpiry(info.ExpiresAt); ok {
			st.ExpiringSoon = time.Until(exp) < durationEnv(lookupEnv, envExpiryWarnWindow, defaultExpiryWarnWindow)
		}
	}
	if st.ExpiringSoon {
		st.NextCommand = loginFix(prog, profile, st.APIURL)
		st.RunBy = "a person at a terminal, before the expiry"
	}
	return st
}

// classifyVerifyError separates a rejected credential (a person must sign in again) from an unreachable server (nothing to fix on the login).
func classifyVerifyError(prog string, st sessionStatus, err error) (problem, next, runBy string) {
	var apiErr *apiError
	if errors.As(err, &apiErr) && (apiErr.StatusCode == 401 || apiErr.StatusCode == 403) {
		return fmt.Sprintf("the server at %s rejected the %s (expired, revoked or for another server)", st.APIURL, st.CredentialSource),
			loginFix(prog, st.Profile, st.APIURL), "a person at a terminal"
	}
	if errors.As(err, &apiErr) {
		return fmt.Sprintf("the server at %s answered with an error: %v", st.APIURL, err), prog + " doctor", "anyone"
	}
	return fmt.Sprintf("could not reach %s: %v", st.APIURL, err), "check the API URL and that the server is up, then run: " + prog + " whoami --json", "anyone"
}

var expiryWarned = map[string]bool{}

// noteExpiry warns once per profile on stderr when the saved token expires inside the window.
func noteExpiry(prog, profile, token string, lookupEnv func(string) (string, bool), stderr io.Writer) {
	creds, err := readCredentialsFile(prog, profile)
	if err != nil || creds.Token == "" || creds.Token != token || expiryWarned[profile] {
		return
	}
	exp, ok := parseExpiry(creds.TokenExpiresAt)
	if !ok {
		return
	}
	left := time.Until(exp)
	if left >= durationEnv(lookupEnv, envExpiryWarnWindow, defaultExpiryWarnWindow) {
		return
	}
	expiryWarned[profile] = true
	if left <= 0 {
		_, _ = fmt.Fprintf(stderr, "warning: the saved login for profile %q expired on %s. A person must renew it: %s\n", profile, exp.Local().Format("2006-01-02"), loginFix(prog, profile, creds.APIURL))
		return
	}
	_, _ = fmt.Fprintf(stderr, "warning: the saved login for profile %q expires in %s. A person should renew it before then: %s\n", profile, humanLeft(left), loginFix(prog, profile, creds.APIURL))
}

// reusableLogin is the check "login" runs first: a valid saved or env credential means no new login, no new session, no approval.
func reusableLogin(prog, profileFlag, apiURLFlag string, lookupEnv func(string) (string, bool)) (sessionStatus, bool) {
	if !savedTokenMayBeSent(prog, profileFlag, apiURLFlag, lookupEnv) {
		return sessionStatus{}, false
	}
	st := collectStatus(prog, profileFlag, apiURLFlag, "", true, lookupEnv)
	return st, st.Verified
}

func printStatus(w io.Writer, st sessionStatus) {
	_, _ = fmt.Fprintf(w, "profile:     %s (from %s)\n", st.Profile, st.ProfileSource)
	_, _ = fmt.Fprintf(w, "api url:     %s\n", st.APIURL)
	_, _ = fmt.Fprintf(w, "credential:  %s\n", st.CredentialSource)
	if st.Identity != "" {
		_, _ = fmt.Fprintf(w, "identity:    %s (%s)\n", st.Identity, st.Kind)
		_, _ = fmt.Fprintf(w, "scope:       %s\n", strings.Join(st.Scope, ", "))
	}
	if st.ExpiresAt != "" {
		_, _ = fmt.Fprintf(w, "expires at:  %s\n", expiryNote(st.ExpiresAt))
	}
	_, _ = fmt.Fprintf(w, "verified:    %t\n", st.Verified)
	_, _ = fmt.Fprintf(w, "trusted device on this machine: %t\n", st.TrustedDevice)
	if st.Problem != "" {
		_, _ = fmt.Fprintf(w, "problem:     %s\n", st.Problem)
	}
	if st.NextCommand != "" {
		_, _ = fmt.Fprintf(w, "next:        %s\n", st.NextCommand)
		_, _ = fmt.Fprintf(w, "run by:      %s\n", st.RunBy)
	}
}

// runWhoami is the top-level "whoami": the first command an agent runs. Exit 0 only when the credential works.
func runWhoami(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "whoami", "print the status as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, whoamiUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	st := collectStatus(prog, profileFlag, apiURLFlag, tokenFlag, true, lookupEnv)
	if err := renderResult(stdout, of.Format, of.Query, st, func() { printStatus(stdout, st) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	if st.Verified {
		return exitOK
	}
	if !jsonOut && of.Format != outputJSON && st.Problem != "" {
		_, _ = fmt.Fprintf(stderr, "%s: %s\n", prog, st.Problem)
	}
	if st.HasCredential && !strings.Contains(st.Problem, "rejected") {
		return exitNetwork
	}
	return exitAPIError
}

func whoamiUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s whoami [flags]

Shows which saved login this directory would use and checks it live with one
cheap call: profile and where its name came from, API URL, identity, scope,
expiry, and whether the server accepted it. When nothing is usable it prints
the exact next command and who must run it. Exit 0 only when the credential
works; non-zero otherwise. Never signs in and never prompts.

Profile precedence: --profile, then %[2]s, then .levelrail/profile (found by
walking up from the current directory), then "%[3]s".

Flags:
  --token string        API token (default: %[4]s env var, then the credentials file)
  --api-url string      control plane base URL
  --profile string      named credentials profile
  --json                print the status as JSON to stdout, nothing else
  --output string       output format: json, table, or text
  --query string        JMESPath expression to filter the result before printing
  -h, --help            show this help
`, prog, envProfile, defaultProfile, apiTokenEnvHelp)
}

// profileRow is one line of "profile list": local facts always, live facts with --verify.
// Name and APIURL keep their original JSON keys so existing scripts still parse.
type profileRow struct {
	Name          string   `json:"Name"`
	APIURL        string   `json:"APIURL"`
	Active        bool     `json:"active"`
	HasToken      bool     `json:"has_token"`
	ExpiresAt     string   `json:"expires_at,omitempty"`
	TrustedDevice bool     `json:"trusted_device"`
	Verified      *bool    `json:"verified,omitempty"`
	Identity      string   `json:"identity,omitempty"`
	Scope         []string `json:"scope,omitempty"`
	Problem       string   `json:"problem,omitempty"`
	NextCommand   string   `json:"next_command,omitempty"`
}

func buildProfileRows(prog string, profiles []profileSummary, verify bool, lookupEnv func(string) (string, bool)) []profileRow {
	active := resolveProfile("", lookupEnv)
	rows := make([]profileRow, 0, len(profiles))
	for _, p := range profiles {
		c, _ := readCredentialsFile(prog, p.Name)
		row := profileRow{Name: p.Name, APIURL: p.APIURL, Active: p.Name == active, HasToken: c.Token != "", ExpiresAt: c.TokenExpiresAt, TrustedDevice: c.TrustedDevice != ""}
		if !row.HasToken {
			row.NextCommand = loginFix(prog, p.Name, p.APIURL)
		}
		if verify && row.HasToken {
			info, err := verifyCredential(p.APIURL, c.Token, durationEnv(lookupEnv, envVerifyTimeout, defaultVerifyTimeout))
			ok := err == nil
			row.Verified = &ok
			if ok {
				row.Identity, row.Scope = info.Name, info.Abilities
			} else {
				row.Problem, row.NextCommand, _ = classifyVerifyError(prog, sessionStatus{Profile: p.Name, APIURL: p.APIURL, CredentialSource: "saved token"}, err)
			}
		}
		rows = append(rows, row)
	}
	return rows
}

func printProfileRows(out io.Writer, rows []profileRow) {
	if len(rows) == 0 {
		_, _ = fmt.Fprintln(out, "no profiles configured")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tAPI URL\tACTIVE\tTOKEN\tEXPIRES\tVERIFIED\tIDENTITY")
	for _, r := range rows {
		verified := "not checked"
		if r.Verified != nil {
			verified = fmt.Sprintf("%t", *r.Verified)
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%t\t%t\t%s\t%s\t%s\n", r.Name, r.APIURL, r.Active, r.HasToken, expiryNote(r.ExpiresAt), verified, r.Identity)
	}
	_ = tw.Flush()
	for _, r := range rows {
		if r.NextCommand != "" {
			_, _ = fmt.Fprintf(out, "%s: a person runs: %s\n", r.Name, r.NextCommand)
		}
	}
}

// savedLoginMismatch reports a repo pointer address no saved profile uses. Every token is held
// back from it, whether saved, passed with --token or set in the environment.
func savedLoginMismatch(apiURLFlag string, lookupEnv func(string) (string, bool), prog, profile string) (apiclient.PointerMismatch, bool) {
	m, bad := apiclient.FindPointerMismatch(apiURLFlag, lookupEnv, prog, profile)
	m.Prog = prog
	return m, bad
}

// savedTokenMayBeSent is false when the saved token would go to an address other than the one it was saved for,
// or when the profile has no server pin to check the answering server against.
func savedTokenMayBeSent(prog, profileFlag, apiURLFlag string, lookupEnv func(string) (string, bool)) bool {
	if v, ok := lookupEnv(apiclient.EnvAPIToken); ok && v != "" {
		return true
	}
	profile := resolveProfile(profileFlag, lookupEnv)
	creds, err := readCredentialsFile(prog, profile)
	if err != nil || creds.Token == "" {
		return true
	}
	if creds.InstanceID == "" {
		return false
	}
	target := resolveAPIURL(apiURLFlag, lookupEnv, prog, profile)
	return creds.APIURL == "" || strings.TrimRight(target, "/") == strings.TrimRight(creds.APIURL, "/")
}
