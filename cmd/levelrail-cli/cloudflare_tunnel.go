package main

import (
	"context"
	"fmt"
	"io"
)

// cloudflareTunnelJSONUsage is every cloudflare-tunnel subcommand's
// --json flag description: identical across get/set/disconnect since
// each returns the same settings shape.
const cloudflareTunnelJSONUsage = "print the settings as JSON to stdout and nothing else"

// runCloudflareTunnel dispatches "cloudflare-tunnel <verb> [flags]" to
// one of get/set/disconnect, mirroring runDomainsCloudflareDNS's own
// get/set/clear dispatch shape for the API's other settings-scoped
// Cloudflare resource. This one is not nested under "domains": it runs
// the cloudflared container that exposes the whole control plane, not a
// per-domain DNS-01 credential.
func runCloudflareTunnel(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, cloudflareTunnelUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, cloudflareTunnelUsage(prog))
		return exitOK
	case "get":
		return runCloudflareTunnelGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runCloudflareTunnelSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "disconnect":
		return runCloudflareTunnelDisconnect(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown cloudflare-tunnel subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, cloudflareTunnelUsage(prog))
		return exitUsage
	}
}

func cloudflareTunnelUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s cloudflare-tunnel get [flags]                          show the current settings and connection status
  %[1]s cloudflare-tunnel set --cf-tunnel-token TOKEN [flags]  configure and enable
  %[1]s cloudflare-tunnel disconnect [flags]                   disable and forget the token

Runs the cloudflared container connected to a tunnel token generated in
your own Cloudflare Zero Trust dashboard, exposing this control plane
without opening an inbound port. Hostname routing stays configured on
Cloudflare's side. This is a distinct credential from
"%[1]s domains cloudflare-dns", which configures the DNS-01 ACME
challenge instead.

Run "%[1]s cloudflare-tunnel <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runCloudflareTunnelGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "cloudflare-tunnel get", cloudflareTunnelJSONUsage, stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s cloudflare-tunnel get [flags]\n\nShows the current Cloudflare Tunnel settings and connection status.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP})
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	settings, err := client.GetCloudflareTunnel(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get cloudflare tunnel settings: %w", err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, settings); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printCloudflareTunnelSettings(stdout, settings)
	return exitOK
}

func runCloudflareTunnelSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "cloudflare-tunnel set", cloudflareTunnelJSONUsage, stderr)
	var cfTunnelTokenFlag string
	var enabledFlag bool
	fs.StringVar(&cfTunnelTokenFlag, "cf-tunnel-token", "", "Cloudflare Tunnel token from your Zero Trust dashboard (required the first time the tunnel is enabled; omit to keep the currently stored token; distinct from --token, this CLI's own bearer auth flag)")
	fs.BoolVar(&enabledFlag, "enabled", true, "run the cloudflared container (pass --enabled=false to disable without clearing the stored token)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s cloudflare-tunnel set --cf-tunnel-token TOKEN [flags]\n\nConfigures Cloudflare Tunnel, enabling or disabling the cloudflared\ncontainer. A token is required the first time the tunnel is enabled.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP})
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	settings, err := client.SetCloudflareTunnel(context.Background(), updateCloudflareTunnelRequest{Enabled: enabledFlag, Token: cfTunnelTokenFlag})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set cloudflare tunnel settings: %w", err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, settings); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printCloudflareTunnelSettings(stdout, settings)
	return exitOK
}

func runCloudflareTunnelDisconnect(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "cloudflare-tunnel disconnect", cloudflareTunnelJSONUsage, stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s cloudflare-tunnel disconnect [flags]\n\nDisables Cloudflare Tunnel and forgets the stored token.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP})
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	settings, err := client.DisconnectCloudflareTunnel(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("disconnect cloudflare tunnel: %w", err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, settings); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	printCloudflareTunnelSettings(stdout, settings)
	return exitOK
}

func printCloudflareTunnelSettings(out io.Writer, s cloudflareTunnelResource) {
	_, _ = fmt.Fprintf(out, "enabled:   %v\n", s.Enabled)
	_, _ = fmt.Fprintf(out, "has_token: %v\n", s.HasToken)
	_, _ = fmt.Fprintf(out, "status:    %s\n", s.Status)
	if s.Message != "" {
		_, _ = fmt.Fprintf(out, "message:   %s\n", s.Message)
	}
}
