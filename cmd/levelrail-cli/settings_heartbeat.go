package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runSettingsHeartbeat dispatches "settings heartbeat show|set|clear" over GET/PUT /api/v1/settings/heartbeat.
func runSettingsHeartbeat(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, settingsHeartbeatUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, settingsHeartbeatUsage(prog))
		return exitOK
	case "show", "set", "clear":
		return runSettingsHeartbeatVerb(prog, args[0], args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown settings heartbeat subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, settingsHeartbeatUsage(prog))
		return exitUsage
	}
}

func settingsHeartbeatUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s settings heartbeat show [flags]
  %[1]s settings heartbeat set --url URL [--fail-url URL] [--body on|off] [flags]
  %[1]s settings heartbeat clear [flags]

An outbound dead-man ping: while the control plane is healthy it calls the
URL every interval, so an external monitor (Healthchecks, Uptime Kuma push,
Better Stack) alerts when the pings stop. The URL holds a secret key, so
it is stored encrypted and only its host is ever shown. Pass --url - to
read it from stdin and keep it out of shell history.

Flags:
  --url string         ping URL (write-only)
  --fail-url string    optional URL called instead when the control plane knows it is unhealthy
  --body string        send a small status JSON body: on or off
  --token string       API token (default: %[2]s env var, then the credentials file)
  --api-url string     control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string     named credentials profile to read
  --json               print the result as JSON to stdout, nothing else
  -h, --help           show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

func runSettingsHeartbeatVerb(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "settings heartbeat "+verb, "print the heartbeat state as JSON to stdout and nothing else", stderr)
	var pingURL, failURL, body string
	if verb == "set" {
		fs.StringVar(&pingURL, "url", "", "ping URL (required, - reads stdin)")
		fs.StringVar(&failURL, "fail-url", "", "URL called when the control plane knows it is unhealthy")
		fs.StringVar(&body, "body", "", "on or off")
	}
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, settingsHeartbeatUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	var upd apiclient.HeartbeatUpdate
	switch verb {
	case "set":
		if pingURL == "-" {
			line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
			pingURL = strings.TrimSpace(line)
		}
		if pingURL == "" && failURL == "" && body == "" {
			return reportError(stdout, stderr, jsonOut, newValidationError("--url, --fail-url or --body is required"))
		}
		if pingURL != "" {
			upd.URL = &pingURL
		}
		if failURL != "" {
			upd.FailURL = &failURL
		}
		if body != "" {
			if body != "on" && body != "off" {
				return reportError(stdout, stderr, jsonOut, newValidationError("--body must be on or off"))
			}
			on := body == "on"
			upd.Body = &on
		}
	case "clear":
		empty := ""
		upd.URL = &empty
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	var (
		res apiclient.HeartbeatResource
		err error
	)
	if verb == "show" {
		res, err = client.GetHeartbeat(context.Background())
	} else {
		res, err = client.UpdateHeartbeat(context.Background(), upd)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("heartbeat %s: %w", verb, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printHeartbeatHuman(stdout, res) })
}

func printHeartbeatHuman(out io.Writer, r apiclient.HeartbeatResource) {
	if !r.Configured {
		_, _ = fmt.Fprintln(out, "heartbeat: not configured")
		return
	}
	_, _ = fmt.Fprintf(out, "heartbeat: pinging %s every %ds (source: %s)\n", r.Host, r.IntervalSecs, r.Source)
	if r.FailHost != "" {
		_, _ = fmt.Fprintf(out, "fail url host: %s\n", r.FailHost)
	}
	if r.LastPingAt.IsZero() {
		_, _ = fmt.Fprintln(out, "last ping: none yet")
		return
	}
	line := fmt.Sprintf("last ping: %s (%s)", r.LastPingAt.Format(time.RFC3339), r.LastResult)
	if r.LastError != "" {
		line += ": " + r.LastError
	}
	_, _ = fmt.Fprintln(out, line)
}
