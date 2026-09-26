package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func appsSBOMUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps sbom <app> [deploy-id] [--download] [--file PATH]

Shows the software bill of materials of a deploy: package count, package types,
licenses and the scan gate decision. Without a deploy id the newest deploy that
produced an SBOM is used. --download prints the raw SPDX or CycloneDX document
(or writes it to --file).

SBOMs exist only for Dockerfile builds on a server started with APP_BUILD_ATTEST=true.
%[2]s`, prog, commonFlagsHelp)
}

func appsScanUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps scan enable <app>                 turn vulnerability scanning on (pulls the scanner image on first scan)
  %[1]s apps scan disable <app>                turn it off (also sets the gate to off)
  %[1]s apps scan status <app> [deploy-id]     show settings and the latest scan result
  %[1]s apps scan run <app> [deploy-id]        scan a deploy's SBOM now
  %[1]s apps scan gate <app> off|warn|block_on_critical
                                               what a scan may do to a release; block_on_critical
                                               keeps the previous release serving
  %[1]s apps scan override <app> --reason TEXT let the next blocked release through once

Scanning is off by default and needs APP_BUILD_ATTEST=true on the server so builds produce an SBOM.
Servers can refuse every scan with APP_SCAN_ENABLED=false.
%[2]s`, prog, commonFlagsHelp)
}

func runAppsSBOM(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var download bool
	var file string
	c, code, ok := parseCLICall(prog, "apps sbom", appsSBOMUsage(prog), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.BoolVar(&download, "download", false, "print the raw SBOM document")
		fs.StringVar(&file, "file", "", "write the raw SBOM document to this file (implies --download)")
	})
	if !ok {
		return code
	}
	rest := c.fs.Args()
	if len(rest) < 1 || len(rest) > 2 {
		return c.invalid("expected an app name and an optional deploy id")
	}
	ctx := context.Background()
	id, err := resolveSupplyChainDeploy(ctx, c.client, rest, true)
	if err != nil {
		return c.fail(err)
	}
	if download || file != "" {
		data, err := c.client.DownloadSBOM(ctx, rest[0], id)
		if err != nil {
			return c.fail(fmt.Errorf("download sbom of %q: %w", id, err))
		}
		if file != "" {
			if err := os.WriteFile(file, data, 0o600); err != nil {
				return c.fail(fmt.Errorf("write %q: %w", file, err))
			}
			_, _ = fmt.Fprintf(stdout, "wrote %d bytes to %s\n", len(data), file)
			return exitOK
		}
		_, _ = stdout.Write(data)
		return exitOK
	}
	sum, err := c.client.GetSBOM(ctx, rest[0], id)
	if err != nil {
		return c.fail(fmt.Errorf("get sbom of %q: %w", id, err))
	}
	return c.render(sum, func() { printSBOMHuman(stdout, sum) })
}

// resolveSupplyChainDeploy returns the explicit deploy id, or the newest deploy
// with an SBOM (needSBOM) or scan result.
func resolveSupplyChainDeploy(ctx context.Context, client *apiclient.Client, args []string, needSBOM bool) (string, error) {
	if len(args) == 2 {
		return args[1], nil
	}
	attempts, err := client.ListDeployAttempts(ctx, args[0])
	if err != nil {
		return "", fmt.Errorf("list deploys of %q: %w", args[0], err)
	}
	for _, a := range attempts {
		if a.SBOMPackages != nil && (needSBOM || a.VulnCounts != nil) {
			return a.ID, nil
		}
	}
	return "", newValidationError("no deploy of %q has an SBOM yet (needs APP_BUILD_ATTEST=true on the server)", args[0])
}

func printSBOMHuman(out io.Writer, s apiclient.SBOMSummary) {
	_, _ = fmt.Fprintf(out, "deploy:     %s\n", s.DeploymentID)
	_, _ = fmt.Fprintf(out, "format:     %s, %d packages", s.Format, s.PackageCount)
	if !s.Available {
		_, _ = fmt.Fprint(out, " (document removed by retention)")
	}
	_, _ = fmt.Fprintln(out)
	if s.Provenance {
		_, _ = fmt.Fprintln(out, "provenance: recorded (SLSA, min)")
	}
	if len(s.Types) > 0 {
		parts := make([]string, 0, len(s.Types))
		for _, t := range s.Types {
			parts = append(parts, fmt.Sprintf("%s %d", t.Type, t.Count))
		}
		_, _ = fmt.Fprintf(out, "types:      %s\n", strings.Join(parts, ", "))
	}
	if len(s.Licenses) > 0 {
		parts := make([]string, 0, len(s.Licenses))
		for _, l := range s.Licenses {
			parts = append(parts, fmt.Sprintf("%s %d", l.License, l.Count))
		}
		_, _ = fmt.Fprintf(out, "licenses:   %s (%d without a license)\n", strings.Join(parts, ", "), s.Unlicensed)
	}
}

func runAppsScan(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsScanUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsScanUsage(prog))
		return exitOK
	case "enable":
		return runScanSetEnabled(prog, rest, stdout, stderr, lookupEnv, true)
	case "disable":
		return runScanSetEnabled(prog, rest, stdout, stderr, lookupEnv, false)
	case "status":
		return runScanStatus(prog, rest, stdout, stderr, lookupEnv)
	case "run":
		return runScanRun(prog, rest, stdout, stderr, lookupEnv)
	case "gate":
		return runScanGate(prog, rest, stdout, stderr, lookupEnv)
	case "override":
		return runScanOverride(prog, rest, stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps scan subcommand %q\n\n%s", prog, args[0], appsScanUsage(prog))
		return exitUsage
	}
}

func runScanSetEnabled(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), enabled bool) int {
	verb := "disable"
	if enabled {
		verb = "enable"
	}
	c, code, ok := parseCLICall(prog, "apps scan "+verb, appsScanUsage(prog), args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	if len(c.fs.Args()) != 1 {
		return c.invalid("expected exactly one app name")
	}
	req := apiclient.SupplyChainSettingsRequest{ScanEnabled: &enabled}
	if !enabled {
		off := "off"
		req.ScanGate = &off
	}
	res, err := c.client.SetSupplyChain(context.Background(), c.fs.Arg(0), req)
	if err != nil {
		return c.fail(fmt.Errorf("apps scan %s for %q: %w", verb, c.fs.Arg(0), err))
	}
	return c.render(res, func() { printScanSettings(stdout, res) })
}

func runScanGate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCLICall(prog, "apps scan gate", appsScanUsage(prog), args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	if len(c.fs.Args()) != 2 {
		return c.invalid("expected an app name and one of off, warn, block_on_critical")
	}
	mode := c.fs.Arg(1)
	res, err := c.client.SetSupplyChain(context.Background(), c.fs.Arg(0), apiclient.SupplyChainSettingsRequest{ScanGate: &mode})
	if err != nil {
		return c.fail(fmt.Errorf("apps scan gate for %q: %w", c.fs.Arg(0), err))
	}
	return c.render(res, func() { printScanSettings(stdout, res) })
}

func runScanOverride(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var reason string
	c, code, ok := parseCLICall(prog, "apps scan override", appsScanUsage(prog), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&reason, "reason", "", "why this release may go live despite critical vulnerabilities (required)")
	})
	if !ok {
		return code
	}
	if len(c.fs.Args()) != 1 || strings.TrimSpace(reason) == "" {
		return c.invalid("expected an app name and --reason")
	}
	res, err := c.client.OverrideSupplyChainGate(context.Background(), c.fs.Arg(0), reason)
	if err != nil {
		return c.fail(fmt.Errorf("apps scan override for %q: %w", c.fs.Arg(0), err))
	}
	return c.render(res, func() { printScanSettings(stdout, res) })
}

func runScanStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCLICall(prog, "apps scan status", appsScanUsage(prog), args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	rest := c.fs.Args()
	if len(rest) < 1 || len(rest) > 2 {
		return c.invalid("expected an app name and an optional deploy id")
	}
	ctx := context.Background()
	settings, err := c.client.GetSupplyChain(ctx, rest[0])
	if err != nil {
		return c.fail(fmt.Errorf("apps scan status for %q: %w", rest[0], err))
	}
	var report *apiclient.VulnReport
	if id, rerr := resolveSupplyChainDeploy(ctx, c.client, rest, false); rerr == nil {
		if r, gerr := c.client.GetVulnerabilities(ctx, rest[0], id); gerr == nil {
			report = &r
		}
	}
	data := map[string]any{"settings": settings, "latest": report}
	return c.render(data, func() {
		printScanSettings(stdout, settings)
		if report != nil {
			printVulnReport(stdout, *report)
		}
	})
}

func runScanRun(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCLICall(prog, "apps scan run", appsScanUsage(prog), args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	rest := c.fs.Args()
	if len(rest) < 1 || len(rest) > 2 {
		return c.invalid("expected an app name and an optional deploy id")
	}
	ctx := context.Background()
	id, err := resolveSupplyChainDeploy(ctx, c.client, rest, true)
	if err != nil {
		return c.fail(err)
	}
	report, err := c.client.ScanDeployment(ctx, rest[0], id)
	if err != nil {
		return c.fail(fmt.Errorf("scan deploy %q: %w", id, err))
	}
	return c.render(report, func() { printVulnReport(stdout, report) })
}

func printScanSettings(out io.Writer, s apiclient.SupplyChainSettings) {
	state := "off"
	if s.ScanEnabled {
		state = "on"
	}
	_, _ = fmt.Fprintf(out, "app:        %s\n", s.App)
	_, _ = fmt.Fprintf(out, "scanning:   %s (gate %s, scanner %s)\n", state, s.ScanGate, s.Scanner)
	if !s.ServerEnabled {
		_, _ = fmt.Fprintln(out, "server:     scanning is switched off (APP_SCAN_ENABLED=false)")
	}
	if !s.BuildAttest {
		_, _ = fmt.Fprintln(out, "sbom:       builds produce no SBOM (APP_BUILD_ATTEST is not true)")
	}
	if s.OverrideArmed {
		_, _ = fmt.Fprintf(out, "override:   armed for the next blocked release: %s\n", s.OverrideReason)
	}
}

func printVulnReport(out io.Writer, r apiclient.VulnReport) {
	_, _ = fmt.Fprintf(out, "deploy:     %s\n", r.DeploymentID)
	switch {
	case r.Scan.Counts != nil:
		c := r.Scan.Counts
		_, _ = fmt.Fprintf(out, "scan:       %s, %d critical, %d high, %d medium, %d low, %d unknown (%d fixable)\n", r.Scan.Scanner, c.Critical, c.High, c.Medium, c.Low, c.Unknown, r.Scan.Fixable)
	case r.Scan.Status != "":
		_, _ = fmt.Fprintf(out, "scan:       %s %s\n", r.Scan.Status, r.Scan.Error)
	default:
		_, _ = fmt.Fprintln(out, "scan:       not scanned")
	}
	if r.Gate != nil {
		_, _ = fmt.Fprintf(out, "gate:       %s %s\n", r.Gate.Action, r.Gate.Reason)
	}
	for _, v := range r.Scan.Top {
		fix := "no fix"
		if v.FixedVersion != "" {
			fix = "fixed in " + v.FixedVersion
		}
		_, _ = fmt.Fprintf(out, "  %-8s %s %s %s (%s)\n", v.Severity, v.ID, v.Package, v.Version, fix)
	}
}
