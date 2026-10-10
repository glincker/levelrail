package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

type authCodeOutput struct {
	Codes     []authCodeEntry            `json:"codes"`
	Approvals []apiclient.SignInApproval `json:"approvals"`
}

type authCodeEntry struct {
	apiclient.SignInCode
	Code string `json:"code,omitempty"`
}

// runAuthCode implements "auth code": the caller's own waiting sign-in codes,
// revealed, plus new-device sign-ins waiting for approval.
func runAuthCode(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "auth code", "print the waiting requests as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, authCodeUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	ctx := context.Background()
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	reqs, err := client.ListSignInRequests(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list sign-in requests: %w", err))
	}
	out := authCodeOutput{Codes: []authCodeEntry{}, Approvals: reqs.Approvals}
	for _, c := range reqs.Codes {
		e := authCodeEntry{SignInCode: c}
		if c.Revealable {
			revealed, err := client.RevealLoginCode(ctx, c.ID)
			if err != nil {
				return reportError(stdout, stderr, jsonOut, fmt.Errorf("reveal sign-in code: %w", err))
			}
			e.Code = revealed.Code
		}
		out.Codes = append(out.Codes, e)
	}
	if out.Approvals == nil {
		out.Approvals = []apiclient.SignInApproval{}
	}
	if err := renderResult(stdout, of.Format, of.Query, out, func() { printAuthCode(stdout, prog, out) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printAuthCode(w io.Writer, prog string, out authCodeOutput) {
	if len(out.Codes) == 0 && len(out.Approvals) == 0 {
		_, _ = fmt.Fprintln(w, "No sign-in is waiting for you.")
		return
	}
	for _, c := range out.Codes {
		code := c.Code
		if code == "" {
			code = "(not available, ask for a new code)"
		}
		_, _ = fmt.Fprintf(w, "code %s  from %s  %s  requested %s  expires %s\n", code, c.RequesterIP, c.UserAgent,
			c.CreatedAt.Local().Format(time.Kitchen), c.ExpiresAt.Local().Format(time.Kitchen))
	}
	for _, a := range out.Approvals {
		_, _ = fmt.Fprintf(w, "new browser %s  from %s  %s  requested %s  (approve: %s auth approve %s, deny: %s auth deny %s)\n",
			a.ID, a.RequesterIP, a.UserAgent, a.CreatedAt.Local().Format(time.Kitchen), prog, a.ID, prog, a.ID)
	}
	_, _ = fmt.Fprintln(w, "Only use a code or approve a browser if you started that sign-in yourself.")
}

func authCodeUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s auth code [flags]

Prints the sign-in codes waiting for your account, with the IP address,
browser and time of each request, plus password sign-ins from new browsers
that wait for your approval. The token must belong to your user and hold
write:sensitive (a device login token does).

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read
  --json                    print the result as JSON to stdout, nothing else
  --output string          output format: json, table, or text
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

// runAuthDecide implements "auth approve <id>" and "auth deny <id>".
func runAuthDecide(prog string, args []string, approve bool, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	verb := "deny"
	if approve {
		verb = "approve"
	}
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "auth "+verb, "print the result as JSON", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s auth %s <approval-id> [flags]\n\n%s a password sign-in from a new browser. Find the id with \"%s auth code\".\n\nFlags:\n", prog, verb, verb, prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, _, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if fs.NArg() != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: auth %s takes exactly one approval id\n", prog, verb)
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if err := client.DecideLoginApproval(context.Background(), fs.Arg(0), approve); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s sign-in: %w", verb, err))
	}
	done := "denied"
	if approve {
		done = "approved"
	}
	_, _ = fmt.Fprintf(stdout, "%s sign-in %s\n", done, fs.Arg(0))
	return exitOK
}

// runAuthDevices implements "auth devices [revoke <id>]".
func runAuthDevices(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "auth devices", "print the devices as JSON", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %[1]s auth devices [flags]\n  %[1]s auth devices revoke <device-id> [flags]\n\nLists the browsers trusted to sign in to your account with a password\nwithout approval, or revokes one so its next sign-in needs approval again.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	ctx := context.Background()
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if fs.NArg() > 0 {
		if fs.Arg(0) != "revoke" || fs.NArg() != 2 {
			fs.Usage()
			return exitUsage
		}
		if err := client.RevokeTrustedDevice(ctx, fs.Arg(1)); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("revoke trusted device: %w", err))
		}
		_, _ = fmt.Fprintf(stdout, "revoked trusted device %s\n", fs.Arg(1))
		return exitOK
	}
	devices, err := client.ListTrustedDevices(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list trusted devices: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, devices, func() {
		if len(devices) == 0 {
			_, _ = fmt.Fprintln(stdout, "No trusted browsers.")
		}
		for _, d := range devices {
			_, _ = fmt.Fprintf(stdout, "%s  %s  %s  last used %s\n", d.ID, d.IP, d.Label, d.LastUsedAt.Local().Format(time.RFC822))
		}
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// runAuthCodeLogin implements "auth code-login": show or set auth.code_login.
func runAuthCodeLogin(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "auth code-login", "print the setting as JSON", stderr)
	admins := fs.String("admins", "", "allow sign in with a code for admin (root) accounts: true or false")
	others := fs.String("others", "", "allow sign in with a code for every other account: true or false")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s auth code-login [--admins true|false] [--others true|false] [flags]\n\nShows or changes the auth.code_login setting. Changing it needs a root token.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	var upd apiclient.CodeLoginSettingsUpdate
	for _, f := range []struct {
		raw string
		dst **bool
	}{{*admins, &upd.Admins}, {*others, &upd.Others}} {
		if f.raw == "" {
			continue
		}
		v, err := strconv.ParseBool(f.raw)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: --admins and --others take true or false\n", prog)
			return exitValidation
		}
		*f.dst = &v
	}
	ctx := context.Background()
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	var s apiclient.CodeLoginSettings
	var err error
	if upd.Admins != nil || upd.Others != nil {
		s, err = client.UpdateCodeLoginSettings(ctx, upd)
	} else {
		s, err = client.GetCodeLoginSettings(ctx)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("code login setting: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, s, func() {
		_, _ = fmt.Fprintf(stdout, "admins:               %t\n", s.Admins)
		_, _ = fmt.Fprintf(stdout, "others:               %t\n", s.Others)
		_, _ = fmt.Fprintf(stdout, "new device approval:  %t\n", s.NewDeviceApproval)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}
