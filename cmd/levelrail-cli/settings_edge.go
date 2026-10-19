package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/spendcap"
)

const edgeDefaultWord = "default"

func settingsEdgeUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s settings edge get [flags]
  %[1]s settings edge set [--trusted-proxies LIST] [--max-body SIZE] [--max-header SIZE] [--read-header-timeout DUR] [--idle-timeout DUR] [flags]
  %[1]s settings edge fix [flags]

Instance request limits and the trusted proxy list. An APP_INGRESS_* environment
variable on the control plane still wins over a saved value. Pass "default" to a
limit flag to restore the built-in value.

fix finds the proxy in front of this instance (Cloudflare, Traefik, Coolify or
another) from the request itself and saves it as the trusted proxy list, so
visitor addresses and forwarded headers are honoured.
`, prog)
}

func runSettingsEdge(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, settingsEdgeUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, settingsEdgeUsage(prog))
		return exitOK
	case "get":
		return runListCommand(prog, args[1:], stdout, stderr, lookupEnv, listCommandParams[apiclient.EdgeSettings]{
			cmdLabel:  "settings edge get",
			jsonUsage: "print the edge settings as JSON to stdout and nothing else",
			usageText: fmt.Sprintf("Usage:\n  %s settings edge get [flags]\n\nShows request limits, the trusted proxy list and the proxy detected in front of this instance.\n\nFlags:\n", prog),
			fetch: func(c *Client, ctx context.Context) (apiclient.EdgeSettings, error) {
				return c.GetEdgeSettings(ctx)
			},
			errVerb: "get edge settings",
			print:   func(out io.Writer, s apiclient.EdgeSettings) { printEdgeSettings(out, s) },
		})
	case "set":
		return runSettingsEdgeSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "fix":
		return runSettingsEdgeFix(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown settings edge subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, settingsEdgeUsage(prog))
		return exitUsage
	}
}

func parseEdgeSize(s string) (*int64, error) {
	if strings.EqualFold(strings.TrimSpace(s), edgeDefaultWord) {
		return nil, nil
	}
	if strings.TrimSpace(s) == "0" {
		zero := int64(0)
		return &zero, nil
	}
	n, err := spendcap.ParseBytes(s)
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func parseEdgeDuration(s string) (*int64, error) {
	if strings.EqualFold(strings.TrimSpace(s), edgeDefaultWord) {
		return nil, nil
	}
	d, err := time.ParseDuration(strings.TrimSpace(s))
	if err != nil || d < 0 {
		return nil, fmt.Errorf("invalid duration %q, want something like 10s or 2m", s)
	}
	secs := int64(d / time.Second)
	return &secs, nil
}

func runSettingsEdgeSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiP, profileP, jsonP, outP, queryP := apiFlagSet(prog, "settings edge set", "print the updated settings as JSON to stdout and nothing else", stderr)
	trusted := fs.String("trusted-proxies", "", "comma separated IPs, CIDRs or private_ranges (empty clears it)")
	maxBody := fs.String("max-body", "", "request body cap such as 100MiB, 0 for none, or default")
	maxHeader := fs.String("max-header", "", "request header cap such as 128KiB, or default")
	readHeader := fs.String("read-header-timeout", "", "time a client has to send its headers such as 10s, or default")
	idle := fs.String("idle-timeout", "", "idle keep-alive timeout such as 2m, or default")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s settings edge set [--trusted-proxies LIST] [--max-body SIZE] [--max-header SIZE] [--read-header-timeout DUR] [--idle-timeout DUR] [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiP, profileP, jsonP, outP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	cur, err := client.GetEdgeSettings(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("read edge settings: %w", err))
	}
	upd := apiclient.EdgeSettingsUpdate{
		TrustedProxies: cur.TrustedProxies, MaxHeaderBytes: cur.MaxHeaderBytes, MaxBodyBytes: cur.MaxBodyBytes,
		ReadHeaderTimeout: cur.ReadHeaderTimeout, IdleTimeout: cur.IdleTimeout,
	}
	var perr error
	changed := false
	fs.Visit(func(f *flag.Flag) {
		if perr != nil {
			return
		}
		changed = true
		switch f.Name {
		case "trusted-proxies":
			upd.TrustedProxies = *trusted
		case "max-body":
			upd.MaxBodyBytes, perr = parseEdgeSize(*maxBody)
		case "max-header":
			upd.MaxHeaderBytes, perr = parseEdgeSize(*maxHeader)
		case "read-header-timeout":
			upd.ReadHeaderTimeout, perr = parseEdgeDuration(*readHeader)
		case "idle-timeout":
			upd.IdleTimeout, perr = parseEdgeDuration(*idle)
		default:
			changed = false
		}
	})
	if perr != nil {
		_, _ = fmt.Fprintln(stderr, perr)
		return exitUsage
	}
	if !changed {
		_, _ = fmt.Fprintf(stderr, "usage: %s settings edge set needs at least one of --trusted-proxies, --max-body, --max-header, --read-header-timeout, --idle-timeout\n", prog)
		return exitUsage
	}
	res, err := client.UpdateEdgeSettings(ctx, upd)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set edge settings: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printEdgeSettings(stdout, res) })
}

func runSettingsEdgeFix(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiP, profileP, jsonP, outP, queryP := apiFlagSet(prog, "settings edge fix", "print the result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s settings edge fix [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiP, profileP, jsonP, outP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.ApplyDetectedProxy(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("fix trusted proxies: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		_, _ = fmt.Fprintf(stdout, "trusted proxies set to %s (detected %s)\n", res.TrustedProxies, res.Detected.Proxy)
	})
}

func printEdgeSettings(out io.Writer, s apiclient.EdgeSettings) {
	tp := s.TrustedProxies
	if tp == "" {
		tp = "(not set)"
	}
	_, _ = fmt.Fprintf(out, "trusted proxies  %s [%s]\n", tp, s.TrustedProxiesSource)
	_, _ = fmt.Fprintf(out, "detected proxy   %s\n", s.Detected.Proxy)
	if s.NeedsTrustedProxies {
		_, _ = fmt.Fprintln(out, "                 a proxy is in front and no trusted proxies are set; run: settings edge fix")
	}
	e := s.Effective
	_, _ = fmt.Fprintf(out, "max body         %s [%s]\n", formatEdgeBytes(e.MaxBodyBytes.Value), e.MaxBodyBytes.Source)
	_, _ = fmt.Fprintf(out, "max header       %s [%s]\n", formatEdgeBytes(e.MaxHeaderBytes.Value), e.MaxHeaderBytes.Source)
	_, _ = fmt.Fprintf(out, "read header      %ds [%s]\n", e.ReadHeaderTimeout.Value, e.ReadHeaderTimeout.Source)
	_, _ = fmt.Fprintf(out, "idle timeout     %ds [%s]\n", e.IdleTimeout.Value, e.IdleTimeout.Source)
}

func formatEdgeBytes(n int64) string {
	if n == 0 {
		return "unlimited"
	}
	return formatByteSize(n)
}

func runIngressLimits(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || (args[0] != "show" && args[0] != "set" && args[0] != "clear") {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %[1]s ingress-limits show APP [flags]\n  %[1]s ingress-limits set APP --max-body SIZE [flags]\n  %[1]s ingress-limits clear APP [flags]\n\nPer-app request body cap, replacing the instance value for that app's domains.\nHeader size and timeouts are instance wide: see \"%[1]s settings edge\".\n", prog)
		return exitUsage
	}
	verb := args[0]
	fs, tokenP, apiP, profileP, jsonP, outP, queryP := apiFlagSet(prog, "ingress-limits "+verb, "print the result as JSON to stdout and nothing else", stderr)
	maxBody := fs.String("max-body", "", "request body cap such as 100MiB (set)")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args[1:], apiFlagPtrs{tokenP, apiP, profileP, jsonP, outP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) != 1 {
		_, _ = fmt.Fprintf(stderr, "usage: %s ingress-limits %s APP\n", prog, verb)
		return exitUsage
	}
	app := rest[0]
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	var res apiclient.AppIngressLimits
	var err error
	switch verb {
	case "set":
		if *maxBody == "" {
			_, _ = fmt.Fprintf(stderr, "usage: %s ingress-limits set APP --max-body SIZE\n", prog)
			return exitUsage
		}
		n, perr := spendcap.ParseBytes(*maxBody)
		if perr != nil || n <= 0 {
			_, _ = fmt.Fprintf(stderr, "invalid --max-body %q, want a positive size such as 100MiB\n", *maxBody)
			return exitUsage
		}
		res, err = client.SetAppIngressLimits(ctx, app, &n)
	case "clear":
		res, err = client.SetAppIngressLimits(ctx, app, nil)
	default:
		res, err = client.GetAppIngressLimits(ctx, app)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("ingress limits %s: %w", verb, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		if res.MaxBodyBytes == nil {
			_, _ = fmt.Fprintf(stdout, "%s: no override, using the instance body cap of %s\n", app, formatEdgeBytes(res.InstanceMaxBodyBytes))
			return
		}
		_, _ = fmt.Fprintf(stdout, "%s: body cap %s (instance value %s)\n", app, formatByteSize(*res.MaxBodyBytes), formatEdgeBytes(res.InstanceMaxBodyBytes))
	})
}
