package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runSettingsIngressDNS01 implements "settings ingress dns01 show|enable|disable".
func runSettingsIngressDNS01(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	verb := "show"
	if len(args) > 0 && (args[0] == "show" || args[0] == "enable" || args[0] == "disable") {
		verb, args = args[0], args[1:]
	}
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "settings ingress dns01 "+verb, "print the result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %[1]s settings ingress dns01 show [flags]\n  %[1]s settings ingress dns01 enable|disable [flags]\n\nIssues certificates for single hosts over DNS-01 too, for servers with port 80\nclosed. Needs a connected DNS provider and is not yet verified against a real CA.\nThe stored value wins and applies at once, no restart; APP_INGRESS_DNS01_ALL_HOSTS\nonly seeds the default until something is stored.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	var res apiclient.IngressDNS01
	var err error
	if verb == "show" {
		res, err = client.GetIngressDNS01(ctx)
	} else {
		res, err = client.SetIngressDNS01(ctx, verb == "enable")
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("dns01 %s: %w", verb, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printIngressDNS01Human(stdout, res) })
}

func printIngressDNS01Human(out io.Writer, r apiclient.IngressDNS01) {
	state := "off"
	if r.Enabled {
		state = "on"
	}
	provider := "none connected"
	if r.ProviderConnected {
		provider = "connected"
	}
	_, _ = fmt.Fprintf(out, "dns01 for every host: %s (source: %s, dns provider: %s)\n", state, r.Source, provider)
}
