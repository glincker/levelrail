package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

type instanceInfo = apiclient.InstanceInfo

const envTrustServer = "APP_TRUST_SERVER"

func describeServer(info instanceInfo) string {
	return fmt.Sprintf("%s (%s), fingerprint %s", info.Name, apiclient.ShortInstanceID(info.InstanceID), info.Fingerprint)
}

// checkLoginServer reads the identity of the server a login is about to talk to and refuses
// to replace a profile pinned to a different one unless reauth is set.
func checkLoginServer(prog, profile, apiURL string, reauth bool, stderr io.Writer) (*instanceInfo, error) {
	existing, rerr := readCredentialsFile(prog, profile)
	pinned := rerr == nil && existing.InstanceID != "" && !reauth
	info, err := apiclient.FetchInstanceInfo(context.Background(), nil, apiURL)
	if err != nil {
		if pinned {
			return nil, err
		}
		// Unpinned: the login itself reports an unreachable server, and the first
		// authenticated call asks for trust.
		return nil, nil
	}
	if pinned {
		if info == nil || info.InstanceID != existing.InstanceID || info.Fingerprint != existing.Fingerprint {
			got := "no server identity"
			if info != nil {
				got = describeServer(*info)
			}
			return nil, fmt.Errorf("profile %q is pinned to %q (%s, fingerprint %s) but %s is %s; run \"%s auth login --reauth --profile %s\" to replace the pin",
				profile, existing.ServerName, apiclient.ShortInstanceID(existing.InstanceID), existing.Fingerprint, apiURL, got, prog, profile)
		}
	}
	if info != nil {
		_, _ = fmt.Fprintf(stderr, "Server: %s\n", describeServer(*info))
	}
	return info, nil
}

func withServerPin(c credentials, info *instanceInfo) credentials {
	if info != nil {
		c.InstanceID, c.Fingerprint, c.ServerName = info.InstanceID, info.Fingerprint, info.Name
	}
	return c
}

func argsConfirmTrust(args []string, lookupEnv func(string) (string, bool)) bool {
	if v, ok := lookupEnv(envTrustServer); ok && (v == "1" || strings.EqualFold(v, "yes") || strings.EqualFold(v, "true")) {
		return true
	}
	for _, a := range args {
		if a == "--" {
			break
		}
		switch a {
		case "--yes", "-yes", "--yes=true", "-yes=true", "--trust-server":
			return true
		}
	}
	return false
}

func stdinIsTerminal() bool {
	st, err := os.Stdin.Stat()
	return !ciMode && err == nil && st.Mode()&os.ModeCharDevice != 0
}

// installServerTrust wires trust-on-first-use for profiles that have no pinned server yet.
func installServerTrust(prog string, args []string, stderr io.Writer, lookupEnv func(string) (string, bool)) {
	apiclient.SetWarnWriter(stderr)
	apiclient.SetTrustHook(func(req apiclient.TrustRequest) error {
		return confirmTrust(prog, req, argsConfirmTrust(args, lookupEnv), stdinIsTerminal(), os.Stdin, stderr)
	})
}

func confirmTrust(prog string, req apiclient.TrustRequest, assumeYes, interactive bool, in io.Reader, stderr io.Writer) error {
	_, _ = fmt.Fprintf(stderr, "First connection to %s under profile %q.\nServer: %s\n", req.URL, req.Profile, describeServer(req.Info))
	if !assumeYes {
		if !interactive {
			return fmt.Errorf("confirm the fingerprint above and re-run with --yes (or set %s=yes), or run \"%s auth trust-server --profile %s\"", envTrustServer, prog, req.Profile)
		}
		_, _ = fmt.Fprint(stderr, "Does this fingerprint match the one shown by the server? Trust it? [y/N] ")
		line, _ := bufio.NewReader(in).ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
			return errors.New("declined")
		}
	}
	creds, err := readCredentialsFile(prog, req.Profile)
	if err != nil {
		return fmt.Errorf("read credentials: %w", err)
	}
	return apiclient.PinProfile(prog, req.Profile, creds, req.Info)
}

type serverStatus struct {
	Profile     string `json:"profile"`
	APIURL      string `json:"api_url"`
	Pinned      bool   `json:"pinned"`
	ServerName  string `json:"server_name,omitempty"`
	InstanceID  string `json:"instance_id,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Reachable   bool   `json:"reachable"`
	Matches     bool   `json:"matches"`
	LiveName    string `json:"live_server_name,omitempty"`
	LiveID      string `json:"live_instance_id,omitempty"`
	LiveFinger  string `json:"live_fingerprint,omitempty"`
}

func buildServerStatus(prog, profile, apiURL string) serverStatus {
	st := serverStatus{Profile: profile, APIURL: apiURL}
	if c, err := readCredentialsFile(prog, profile); err == nil && c.InstanceID != "" {
		st.Pinned, st.ServerName, st.InstanceID, st.Fingerprint = true, c.ServerName, c.InstanceID, c.Fingerprint
	}
	info, err := apiclient.FetchInstanceInfo(context.Background(), nil, apiURL)
	if err == nil && info != nil {
		st.Reachable, st.LiveName, st.LiveID, st.LiveFinger = true, info.Name, info.InstanceID, info.Fingerprint
		st.Matches = st.Pinned && info.InstanceID == st.InstanceID && info.Fingerprint == st.Fingerprint
	}
	return st
}

func printServerStatus(out io.Writer, st serverStatus) {
	_, _ = fmt.Fprintf(out, "profile:      %s\napi url:      %s\n", st.Profile, st.APIURL)
	if !st.Pinned {
		_, _ = fmt.Fprintln(out, "server:       not pinned yet (the first authenticated call asks you to confirm the fingerprint)")
	} else {
		_, _ = fmt.Fprintf(out, "server:       %s (%s)\nfingerprint:  %s\n", st.ServerName, apiclient.ShortInstanceID(st.InstanceID), st.Fingerprint)
	}
	switch {
	case !st.Reachable:
		_, _ = fmt.Fprintln(out, "live check:   server did not report an identity")
	case st.Pinned && st.Matches:
		_, _ = fmt.Fprintln(out, "live check:   matches the pinned server")
	case st.Pinned:
		_, _ = fmt.Fprintf(out, "live check:   MISMATCH, this URL is %s (%s), fingerprint %s\n", st.LiveName, apiclient.ShortInstanceID(st.LiveID), st.LiveFinger)
	default:
		_, _ = fmt.Fprintf(out, "live check:   %s (%s), fingerprint %s\n", st.LiveName, apiclient.ShortInstanceID(st.LiveID), st.LiveFinger)
	}
}

func runAuthStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "auth status", "print the pinned server as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, authStatusUsage(prog)) }
	_, apiURLFlag, profileFlag, _, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	profile := resolveProfile(profileFlag, lookupEnv)
	st := buildServerStatus(prog, profile, resolveAPIURL(apiURLFlag, lookupEnv, prog, profile))
	if err := renderResult(stdout, of.Format, of.Query, st, func() { printServerStatus(stdout, st) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func authStatusUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s auth status [flags]

Shows the server this profile is pinned to (name, id, fingerprint, URL) and checks the
URL still answers as that server. No token is sent.

Flags:
  --api-url string   control plane base URL (default: %[2]s env var, then %[3]s)
  --profile string   named credentials profile to read (overrides APP_PROFILE, default "default")
  --json             print the status as JSON to stdout, nothing else
  --output string    output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string     JMESPath expression to filter the result before printing
  -h, --help         show this help
`, prog, envAPIURL, defaultAPIURL)
}

func runAuthTrustServer(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "auth trust-server", "print the pinned server as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, authTrustServerUsage(prog)) }
	yes := fs.Bool("yes", false, "pin without asking, for scripts")
	_, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	profile := resolveProfile(profileFlag, lookupEnv)
	apiURL := resolveAPIURL(apiURLFlag, lookupEnv, prog, profile)
	creds, err := readCredentialsFile(prog, profile)
	if err != nil || creds.Token == "" {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("profile %q has no saved login; run \"%s auth login\" first", profile, prog))
	}
	info, err := apiclient.FetchInstanceInfo(context.Background(), nil, apiURL)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	if info == nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s did not report a server identity, nothing to pin", apiURL))
	}
	if terr := confirmPin(creds, *info, *yes, stdin, stderr, prog, profile, apiURL); terr != nil {
		return reportError(stdout, stderr, jsonOut, terr)
	}
	creds.APIURL = apiURL
	if err := apiclient.PinProfile(prog, profile, creds, *info); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("save pin: %w", err))
	}
	st := buildServerStatus(prog, profile, apiURL)
	if err := renderResult(stdout, of.Format, of.Query, st, func() { printServerStatus(stdout, st) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func confirmPin(creds credentials, info instanceInfo, yes bool, stdin io.Reader, stderr io.Writer, prog, profile, apiURL string) error {
	_, _ = fmt.Fprintf(stderr, "Server at %s: %s\n", apiURL, describeServer(info))
	if creds.InstanceID != "" && creds.InstanceID != info.InstanceID {
		_, _ = fmt.Fprintf(stderr, "This replaces the current pin: %s (%s).\nThe saved token was issued by that server and will be rejected here; use \"%s auth login --reauth --profile %s\" to get a token for this one.\n",
			creds.ServerName, apiclient.ShortInstanceID(creds.InstanceID), prog, profile)
	}
	if yes {
		return nil
	}
	if !stdinIsTerminal() {
		return errors.New("not a terminal: pass --yes to pin without asking")
	}
	_, _ = fmt.Fprint(stderr, "Pin this server for the profile? [y/N] ")
	line, _ := bufio.NewReader(stdin).ReadString('\n')
	if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
		return errors.New("declined")
	}
	return nil
}

func authTrustServerUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s auth trust-server [flags]

Pins the server currently answering at the profile's API URL: prints its name and
fingerprint, asks you to confirm they match what the server shows, and saves them. Later
requests refuse to send the token to any other server.

Flags:
  --api-url string   control plane base URL (default: %[2]s env var, then %[3]s)
  --profile string   named credentials profile to pin (overrides APP_PROFILE, default "default")
  --yes              pin without asking, for scripts
  --json             print the pinned server as JSON to stdout, nothing else
  --output string    output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string     JMESPath expression to filter the result before printing
  -h, --help         show this help
`, prog, envAPIURL, defaultAPIURL)
}
