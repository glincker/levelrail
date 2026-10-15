package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runAppsExposure implements "apps exposure|ports <name> [show|check]" and
// the set form with --mode/--host-port/--port (add --preview for a dry run).
func runAppsExposure(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps exposure", "print the result as JSON to stdout and nothing else", stderr)
	var mode string
	var hostPort, port int
	var preview bool
	fs.StringVar(&mode, "mode", "", "proxied (domain or automatic hostname only) or direct (also publish a host port and open the firewall)")
	fs.IntVar(&hostPort, "host-port", 0, "host port for direct mode (default: auto-picked free port)")
	fs.IntVar(&port, "port", 0, "container port the app listens on")
	fs.BoolVar(&preview, "preview", false, "show what would change without applying it")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %[1]s apps exposure <name> [flags]\n  %[1]s apps exposure <name> check\n\nWith no flags, shows the current exposure, URLs and host ports in use. With\n--mode, --host-port or --port, applies the change (direct mode is plain HTTP\nand opens a host firewall rule). \"check\" probes reachability from the\ncontrol plane host.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) == 0 || len(rest) > 2 {
		fs.Usage()
		return exitUsage
	}
	name := rest[0]
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	render := func(data any, human func()) int {
		if err := renderResult(stdout, of.Format, of.Query, data, human); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitCodeForError(err)
		}
		return exitOK
	}

	if len(rest) == 2 {
		if rest[1] != "check" {
			return reportError(stdout, stderr, jsonOut, newValidationError("unknown subcommand %q (want check)", rest[1]))
		}
		res, err := client.CheckAppExposure(ctx, name)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("check exposure for app %q: %w", name, err))
		}
		return render(res, func() { printAppExposureCheck(stdout, res) })
	}

	req := apiclient.AppExposureRequest{Mode: mode}
	if hostPort > 0 {
		req.HostPort = &hostPort
	}
	if port > 0 {
		req.ContainerPort = &port
	}
	if mode == "" && req.HostPort == nil && req.ContainerPort == nil {
		res, err := client.GetAppExposure(ctx, name)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("get exposure for app %q: %w", name, err))
		}
		return render(res, func() { printAppExposure(stdout, res.AppExposurePlan, res.PortsInUse) })
	}
	if preview {
		plan, err := client.PreviewAppExposure(ctx, name, req)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("preview exposure for app %q: %w", name, err))
		}
		return render(plan, func() { printAppExposure(stdout, plan, nil) })
	}
	res, err := client.SetAppExposure(ctx, name, req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set exposure for app %q: %w", name, err))
	}
	return render(res, func() {
		_, _ = fmt.Fprintf(stderr, "exposure updated; reconcile is asynchronous, check \"%s apps status %s\"\n", prog, name)
		printAppExposure(stdout, res.AppExposurePlan, nil)
	})
}

func printAppExposure(out io.Writer, p apiclient.AppExposurePlan, inUse []apiclient.AppPortHolder) {
	_, _ = fmt.Fprintf(out, "mode:            %s\ncontainer port:  %d\n", p.Mode, p.ContainerPort)
	if p.HostPort > 0 {
		_, _ = fmt.Fprintf(out, "host port:       %d (bind %s)\n", p.HostPort, p.BindAddress)
	}
	for _, u := range p.URLs {
		_, _ = fmt.Fprintf(out, "url (%s):%s%s\n", u.Kind, strings.Repeat(" ", max(1, 9-len(u.Kind))), u.URL)
	}
	for _, c := range p.Changes {
		_, _ = fmt.Fprintf(out, "change:          %s %s -> %s\n", c.Kind, c.From, c.To)
	}
	if p.Recreates {
		_, _ = fmt.Fprintln(out, "note:            the container is recreated (the image is not rebuilt)")
	}
	for _, w := range p.Warnings {
		if w == "plain_http" {
			_, _ = fmt.Fprintln(out, "warning:         direct access is plain HTTP and bypasses TLS and auth gates")
		}
	}
	for _, h := range inUse {
		_, _ = fmt.Fprintf(out, "in use:          %d  %s %s\n", h.Port, h.Kind, h.Holder)
	}
}

func printAppExposureCheck(out io.Writer, c apiclient.AppExposureCheck) {
	_, _ = fmt.Fprintf(out, "probed from: %s (cannot prove reachability from the internet)\n", c.Scope)
	for _, t := range c.Targets {
		state := "ok"
		if !t.OK {
			state = "FAILED " + t.Failure
		}
		_, _ = fmt.Fprintf(out, "%-10s %s  %s  status=%d  %dms  %s\n", t.Kind, state, t.URL, t.Status, t.LatencyMS, t.Detail)
	}
}
