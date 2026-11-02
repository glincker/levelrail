package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runSettingsIngressCloudflare implements "settings ingress cloudflare show|set".
func runSettingsIngressCloudflare(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	verb := "show"
	if len(args) > 0 && (args[0] == "show" || args[0] == "set") {
		verb, args = args[0], args[1:]
	}
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "settings ingress cloudflare "+verb, "print the result as JSON to stdout and nothing else", stderr)
	preset := fs.String("preset", "", "none or cloudflare (set only)")
	confirm := fs.Bool("confirm-weakening", false, "required to switch to cloudflare, which trusts CF-Connecting-IP from Cloudflare's ranges")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %[1]s settings ingress cloudflare show [flags]\n  %[1]s settings ingress cloudflare set --preset none|cloudflare [--confirm-weakening] [flags]\n\nWith cloudflare, the visitor address comes from CF-Connecting-IP, but only when the\nconnection arrives from Cloudflare's published ranges. Any Cloudflare customer can\nreach your server through their own proxied record, so this is not proof a request\ncame through your zone. The stored value wins and applies at once; the range list is\nrefreshed in the background (see settings runtime, ingress.cloudflare_ips_*).\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if verb == "set" && *preset == "" {
		_, _ = fmt.Fprintf(stderr, "%s: --preset none|cloudflare is required\n", prog)
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	var res apiclient.IngressCloudflare
	var err error
	if verb == "show" {
		res, err = client.GetIngressCloudflare(ctx)
	} else {
		res, err = client.SetIngressCloudflare(ctx, *preset, *confirm)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("cloudflare preset %s: %w", verb, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printIngressCloudflareHuman(stdout, res) })
}

func printIngressCloudflareHuman(out io.Writer, r apiclient.IngressCloudflare) {
	_, _ = fmt.Fprintf(out, "trusted proxy preset: %s (source: %s)\n", r.Preset, r.Source)
	last := "never fetched, using the list shipped in the binary"
	if r.LastRefresh != nil {
		last = r.LastRefresh.Format("2006-01-02 15:04 MST")
	}
	_, _ = fmt.Fprintf(out, "cloudflare ranges:    %d (%s), last refresh: %s, every %s\n", r.Ranges, r.RangesSource, last, r.RefreshInterval)
	if r.LastError != "" {
		_, _ = fmt.Fprintf(out, "last refresh error:   %s\n", r.LastError)
	}
}
