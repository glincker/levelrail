package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runDockerGuard implements "docker-guard status|set": the allowlisting
// proxy between the control plane and the Docker socket.
func runDockerGuard(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, dockerGuardUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, dockerGuardUsage(prog))
		return exitOK
	case "status":
		return runDockerGuardStatus(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runDockerGuardSet(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown docker-guard subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, dockerGuardUsage(prog))
		return exitUsage
	}
}

func dockerGuardUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s docker-guard status [flags]
  %[1]s docker-guard set --mode off|audit|enforce [flags]

The control plane reaches Docker through an allowlisting proxy:
  enforce  requests a rule denies get a 403 and never reach Docker
  audit    everything is allowed, would-be denials are logged and audited (default)
  off      no proxy, Docker is reached directly (takes effect after a restart)
APP_DOCKER_GUARD on the server overrides this setting.
`, prog)
}

func runDockerGuardStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runListCommand(prog, args, stdout, stderr, lookupEnv, listCommandParams[apiclient.DockerGuardResource]{
		cmdLabel:  "docker-guard status",
		jsonUsage: "print the docker guard status as JSON to stdout and nothing else",
		usageText: fmt.Sprintf("Usage:\n  %s docker-guard status [flags]\n\nShows the guard mode and what it flagged.\n\nFlags:\n", prog),
		fetch: func(c *Client, ctx context.Context) (apiclient.DockerGuardResource, error) {
			return c.GetDockerGuard(ctx)
		},
		errVerb: "get docker guard status",
		print:   func(out io.Writer, s apiclient.DockerGuardResource) { printDockerGuard(out, s) },
	})
}

func runDockerGuardSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "docker-guard set", "print the updated docker guard status as JSON to stdout and nothing else", stderr)
	var mode string
	fs.StringVar(&mode, "mode", "", "off, audit or enforce (required)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s docker-guard set --mode MODE [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	switch mode {
	case "off", "audit", "enforce":
	default:
		_, _ = fmt.Fprintf(stderr, "%s: docker-guard set requires --mode off, audit or enforce\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.UpdateDockerGuard(context.Background(), apiclient.UpdateDockerGuardRequest{Mode: mode})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set docker guard mode: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printDockerGuard(stdout, res) })
}

func printDockerGuard(out io.Writer, s apiclient.DockerGuardResource) {
	if !s.Configured {
		_, _ = fmt.Fprintln(out, "docker guard: not configured on this control plane")
		return
	}
	_, _ = fmt.Fprintf(out, "mode:        %s (from %s)\n", s.Mode, s.Source)
	_, _ = fmt.Fprintf(out, "running:     %s\n", runningLabel(s))
	if s.RestartRequired {
		_, _ = fmt.Fprintln(out, "restart:     required to apply the configured mode")
	}
	if s.ConfigError != "" {
		_, _ = fmt.Fprintf(out, "config:      %s\n", s.ConfigError)
	}
	days := s.WindowSeconds / 86400
	_, _ = fmt.Fprintf(out, "last %dd:     %d would deny, %d denied\n", days, s.WouldDenyTotal, s.DeniedTotal)
	for _, r := range s.Window {
		_, _ = fmt.Fprintf(out, "  %-24s would deny %d, denied %d, last %s %s\n", r.Rule, r.WouldDeny, r.Denied, r.LastSeen, r.LastPath)
	}
	if s.ReadyToEnforce {
		_, _ = fmt.Fprintln(out, "ready:       no would-be denials over the full window, enforce is likely safe")
	}
}

func runningLabel(s apiclient.DockerGuardResource) string {
	if !s.Running {
		return "no (Docker is reached directly)"
	}
	return s.Effective + " on " + s.Socket
}
