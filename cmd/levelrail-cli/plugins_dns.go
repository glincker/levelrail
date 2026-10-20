package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func runPluginsTest(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiP, profP, jsonP, outP, qP := apiFlagSet(prog, "plugins test", "print the result as JSON", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, pluginsUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiP, profP, jsonP, outP, qP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, "plugins test", "plugin name")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.TestPlugin(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("plugins test %q: %w", name, err))
	}
	code := writeScheduledTaskResult(stdout, stderr, of, res, func() {
		state := "failed"
		if res.OK {
			state = "ok"
		}
		_, _ = fmt.Fprintf(stdout, "%s: %s\n", state, res.Message)
		if len(res.Zones) > 0 {
			_, _ = fmt.Fprintf(stdout, "zones: %s\n", strings.Join(res.Zones, ", "))
		}
	})
	if code == exitOK && !res.OK {
		return exitCheckFailed
	}
	return code
}

func runPluginsDNSProvider(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiP, profP, jsonP, outP, qP := apiFlagSet(prog, "plugins dns-provider", "print the setting as JSON", stderr)
	off := fs.Bool("off", false, "return DNS writes to the built-in client")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, pluginsUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiP, profP, jsonP, outP, qP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	var (
		res apiclient.DNSProviderPlugin
		err error
	)
	switch {
	case *off && fs.NArg() == 0:
		res, err = client.SetDNSProviderPlugin(ctx, "")
	case !*off && fs.NArg() == 1:
		res, err = client.SetDNSProviderPlugin(ctx, fs.Arg(0))
	case !*off && fs.NArg() == 0:
		res, err = client.GetDNSProviderPlugin(ctx)
	default:
		_, _ = fmt.Fprintf(stderr, "%s plugins dns-provider: give a plugin name, --off, or nothing to show the setting\n", prog)
		return exitUsage
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("plugins dns-provider: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		if res.Plugin == "" {
			_, _ = fmt.Fprintln(stdout, "DNS records are written by the built-in client")
			return
		}
		_, _ = fmt.Fprintf(stdout, "DNS records are written by the %q plugin\n", res.Plugin)
	})
}
