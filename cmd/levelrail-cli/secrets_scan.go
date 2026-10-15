package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/deploy/secretscan"
)

func secretsScanUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s secrets scan --path DIR                      scan a local directory for committed secrets
  %[1]s secrets scan <app>                           scan the app's plain env values now
  %[1]s secrets scan <app> --deploy ID               show the scan recorded for a deploy
  %[1]s secrets scan <app> --policy off|warn|block   set what a finding does to a deploy
  %[1]s secrets scan <app> --allow FINGERPRINT --reason TEXT
                                                     allow-list one finding (audited)
  %[1]s secrets scan <app> --unallow FINGERPRINT     remove an allow-list entry
  %[1]s secrets scan <app> --policy-show             show the policy and allow-list

Findings carry a rule id, a location and a short fingerprint, never the value.
A scan with findings exits 1 so it can gate a CI job.
%[2]s`, prog, commonFlagsHelp)
}

type secretsScanFlags struct {
	path, deploy, policy, allow, reason, unallow string
	show                                         bool
}

func runSecretsScan(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var f secretsScanFlags
	c, code, ok := parseCLICall(prog, "secrets scan", secretsScanUsage(prog), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&f.path, "path", "", "scan this local directory instead of calling the control plane")
		fs.StringVar(&f.deploy, "deploy", "", "show the scan recorded for this deploy id")
		fs.StringVar(&f.policy, "policy", "", "set the policy: off, warn or block")
		fs.StringVar(&f.allow, "allow", "", "allow-list this fingerprint (needs --reason)")
		fs.StringVar(&f.reason, "reason", "", "why the fingerprint is safe")
		fs.StringVar(&f.unallow, "unallow", "", "remove this fingerprint from the allow-list")
		fs.BoolVar(&f.show, "policy-show", false, "show the policy and allow-list")
	})
	if !ok {
		return code
	}
	if f.path != "" {
		return runSecretsScanLocal(c, f.path, stdout)
	}
	if len(c.fs.Args()) != 1 {
		return c.invalid("expected an app name, or --path DIR for a local scan")
	}
	return runSecretsScanApp(c, c.fs.Arg(0), f, stdout)
}

func runSecretsScanLocal(c cliCall, dir string, stdout io.Writer) int {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return c.invalid("--path %q is not a readable directory", dir)
	}
	res, err := secretscan.ScanDir(context.Background(), dir, secretscan.LimitsFromEnv(os.LookupEnv))
	if err != nil {
		return c.fail(fmt.Errorf("scan %q: %w", dir, err))
	}
	findings := make([]apiclient.SecretFinding, 0, len(res.Findings))
	for _, x := range res.Findings {
		findings = append(findings, apiclient.SecretFinding{RuleID: x.RuleID, Source: x.Source, Path: x.Path, Line: x.Line, Var: x.Var, Fingerprint: x.Fingerprint})
	}
	out := apiclient.SecretScanRun{Action: actionFor(len(findings)), Status: secretscan.StatusComplete, FilesScanned: res.FilesScanned, Truncated: res.Truncated, Findings: findings}
	if code := c.render(out, func() { printSecretScanRun(stdout, out) }); code != exitOK {
		return code
	}
	if len(findings) > 0 {
		return exitCheckFailed
	}
	return exitOK
}

func actionFor(n int) string {
	if n > 0 {
		return secretscan.ActionWarn
	}
	return secretscan.ActionAllow
}

func runSecretsScanApp(c cliCall, app string, f secretsScanFlags, stdout io.Writer) int {
	ctx := context.Background()
	switch {
	case f.policy != "":
		res, err := c.client.SetSecretScanPolicy(ctx, app, f.policy)
		if err != nil {
			return c.fail(fmt.Errorf("set secret scan policy for %q: %w", app, err))
		}
		return c.render(res, func() { printSecretScanSettings(stdout, res) })
	case f.allow != "":
		if strings.TrimSpace(f.reason) == "" {
			return c.invalid("--allow needs --reason")
		}
		e, err := c.client.AllowSecretFingerprint(ctx, app, f.allow, f.reason)
		if err != nil {
			return c.fail(fmt.Errorf("allow-list %q for %q: %w", f.allow, app, err))
		}
		return c.render(e, func() { _, _ = fmt.Fprintf(stdout, "allowed %s (%s) by %s\n", e.Fingerprint, e.Reason, e.Actor) })
	case f.unallow != "":
		if err := c.client.UnallowSecretFingerprint(ctx, app, f.unallow); err != nil {
			return c.fail(fmt.Errorf("remove allow-list entry %q for %q: %w", f.unallow, app, err))
		}
		_, _ = fmt.Fprintf(stdout, "removed %s\n", f.unallow)
		return exitOK
	case f.show:
		res, err := c.client.GetSecretScan(ctx, app)
		if err != nil {
			return c.fail(fmt.Errorf("get secret scan settings for %q: %w", app, err))
		}
		return c.render(res, func() { printSecretScanSettings(stdout, res) })
	case f.deploy != "":
		run, err := c.client.GetDeploymentSecretScan(ctx, app, f.deploy)
		if err != nil {
			return c.fail(fmt.Errorf("get secret scan of deploy %q: %w", f.deploy, err))
		}
		return c.render(run, func() { printSecretScanRun(stdout, run) })
	}
	res, err := c.client.RunSecretScan(ctx, app)
	if err != nil {
		return c.fail(fmt.Errorf("scan env of %q: %w", app, err))
	}
	run := apiclient.SecretScanRun{App: app, Action: actionFor(activeFindings(res.Findings)), Status: secretscan.StatusComplete, Truncated: res.Truncated, Findings: res.Findings}
	if code := c.render(run, func() { printSecretScanRun(stdout, run) }); code != exitOK {
		return code
	}
	if activeFindings(res.Findings) > 0 {
		return exitCheckFailed
	}
	return exitOK
}

func activeFindings(f []apiclient.SecretFinding) int {
	n := 0
	for _, x := range f {
		if !x.Allowed {
			n++
		}
	}
	return n
}

func printSecretScanSettings(out io.Writer, s apiclient.SecretScanSettings) {
	_, _ = fmt.Fprintf(out, "policy:     %s\n", s.Policy)
	_, _ = fmt.Fprintf(out, "allow-list: %d entries\n", len(s.AllowList))
	for _, e := range s.AllowList {
		_, _ = fmt.Fprintf(out, "  %s  %s (by %s)\n", e.Fingerprint, e.Reason, e.Actor)
	}
}

func printSecretScanRun(out io.Writer, r apiclient.SecretScanRun) {
	if r.Policy != "" {
		_, _ = fmt.Fprintf(out, "policy:   %s\n", r.Policy)
	}
	_, _ = fmt.Fprintf(out, "result:   %s", r.Action)
	if r.Reason != "" {
		_, _ = fmt.Fprintf(out, " (%s)", r.Reason)
	}
	_, _ = fmt.Fprintln(out)
	if r.FilesScanned > 0 {
		_, _ = fmt.Fprintf(out, "files:    %d scanned\n", r.FilesScanned)
	}
	if r.Truncated {
		_, _ = fmt.Fprintln(out, "note:     scan stopped at a size cap, results are incomplete")
	}
	for _, f := range r.Findings {
		loc := f.Var
		if f.Path != "" {
			loc = fmt.Sprintf("%s:%d", f.Path, f.Line)
		}
		suffix := ""
		if f.Allowed {
			suffix = "  [allowed]"
		}
		_, _ = fmt.Fprintf(out, "  %-26s %-32s %s%s\n", f.RuleID, loc, f.Fingerprint, suffix)
	}
}
