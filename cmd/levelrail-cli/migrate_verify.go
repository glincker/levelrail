package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/migrateverify"
)

var verifySources = map[string]bool{"coolify": true, "dokploy": true, "caprover": true}

// runMigrateVerify implements "migrate verify <source>": a parity report of
// the source platform against this instance. Exit code 1 when a blocking
// difference exists.
func runMigrateVerify(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs := flag.NewFlagSet(prog+" migrate verify", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var f dataFlags
	var req apiclient.MigrationVerifyRequest
	var only listFlag
	var outputFlag, query, timeoutFlag string
	var concurrency int
	var latest bool
	f.register(fs)
	fs.StringVar(&req.URL, "url", "", "source platform base URL (required unless --recheck or --latest)")
	fs.StringVar(&req.Token, "token", "", "source platform API token, or "+envMigrateSourceToken)
	fs.BoolVar(&req.InsecureTLS, "insecure-tls", false, "skip TLS verification of the source")
	fs.BoolVar(&req.AllowPrivate, "allow-private", false, "allow a private-network source address (the control plane needs APP_IMPORT_ALLOW_PRIVATE_NETWORKS=true)")
	fs.Var(&only, "only", "compare only this source app id or name, repeatable")
	fs.BoolVar(&req.SkipProbes, "no-probes", false, "skip the live HTTP probes")
	fs.StringVar(&timeoutFlag, "probe-timeout", "", "per-request probe timeout, e.g. 10s (default env "+migrateverify.EnvProbeTimeout+", then 10s)")
	fs.IntVar(&concurrency, "probe-concurrency", 0, "domains probed at once (default env "+migrateverify.EnvProbeConcurrency+", then 6)")
	fs.BoolVar(&req.AllowPrivateProbes, "allow-private-probes", false, "let probes reach private and loopback addresses (the control plane needs "+migrateverify.EnvAllowPrivateProbe+"=true)")
	fs.StringVar(&req.TargetIP, "target-ip", "", "IP (or ip:port) of this instance's ingress to probe directly while DNS still points at the source")
	fs.StringVar(&req.SourceIP, "source-ip", "", "IP (or ip:port) of the source's ingress to probe directly")
	fs.BoolVar(&req.Recheck, "recheck", false, "run again with the source credential the control plane still holds from the last check")
	fs.BoolVar(&latest, "latest", false, "print the last stored report without running a check")
	fs.StringVar(&outputFlag, "output", "", "output format: json, table, or text (default table; --json is shorthand for --output json)")
	fs.StringVar(&query, "query", "", "JMESPath expression to filter the result before printing")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, migrateVerifyUsage(prog)) }

	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		req.Platform, args = args[0], args[1:]
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	format, err := resolveOutputFormat(f.jsonOut, outputFlag)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, err)
	}
	jsonOut := format == outputJSON
	client := apiClientFromFlags(prog, f.apiURL, "", f.profile, lookupEnv)
	ctx := context.Background()

	var rep apiclient.MigrationVerifyReport
	if latest {
		rep, err = client.LatestMigrationVerify(ctx)
	} else {
		if err := fillVerifyRequest(&req, only, timeoutFlag, concurrency, lookupEnv); err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
		rep, err = client.VerifyMigration(ctx, req)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	if err := renderResult(stdout, format, query, rep, func() { printVerifyReport(stdout, rep) }); err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	if rep.Counts.Blocking > 0 {
		return exitCheckFailed
	}
	return exitOK
}

func fillVerifyRequest(req *apiclient.MigrationVerifyRequest, only listFlag, timeoutFlag string, concurrency int, lookupEnv func(string) (string, bool)) error {
	if req.Recheck {
		return nil
	}
	if !verifySources[req.Platform] {
		return newValidationError("name the source: coolify, dokploy or caprover")
	}
	if req.Token == "" {
		req.Token, _ = lookupEnv(envMigrateSourceToken)
	}
	if req.URL == "" || req.Token == "" {
		return newValidationError("--url and a source token (--token or %s) are required", envMigrateSourceToken)
	}
	req.Only = only
	if timeoutFlag == "" {
		timeoutFlag, _ = lookupEnv(migrateverify.EnvProbeTimeout)
	}
	if timeoutFlag != "" {
		d, err := time.ParseDuration(timeoutFlag)
		if err != nil || d <= 0 {
			return newValidationError("--probe-timeout %q must be a duration such as 10s", timeoutFlag)
		}
		req.ProbeTimeoutSeconds = int((d + time.Second - 1) / time.Second)
	}
	if concurrency == 0 {
		if v, ok := lookupEnv(migrateverify.EnvProbeConcurrency); ok {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 {
				return newValidationError("%s %q must be a positive number", migrateverify.EnvProbeConcurrency, v)
			}
			concurrency = n
		}
	}
	if concurrency < 0 {
		return newValidationError("--probe-concurrency must be positive")
	}
	req.ProbeConcurrency = concurrency
	return nil
}

func printVerifyReport(w io.Writer, rep apiclient.MigrationVerifyReport) {
	_, _ = fmt.Fprintf(w, "Parity with %s: %s (%d blocking, %d warning, %d info)\n\n", rep.Platform, strings.ToUpper(rep.Verdict),
		rep.Counts.Blocking, rep.Counts.Warning, rep.Counts.Info)
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "AREA\tSOURCE\tHERE\tMATCHED\tBLOCKING\tWARNING\tINFO")
	for _, c := range rep.Categories {
		_, _ = fmt.Fprintf(tw, "%s\t%d\t%d\t%d\t%d\t%d\t%d\n", c.Category, c.Source, c.Target, c.Matched, c.Blocking, c.Warning, c.Info)
	}
	_ = tw.Flush()
	if len(rep.Differences) > 0 {
		_, _ = fmt.Fprintln(w)
		tw = tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "SEVERITY\tAREA\tITEM\tDIFFERENCE\tFIX")
		for _, d := range rep.Differences {
			item := d.Subject
			if d.App != "" {
				item = d.App + " " + d.Subject
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", d.Severity, d.Category, item, d.Detail, d.Fix)
		}
		_ = tw.Flush()
	}
	if len(rep.Probes) > 0 {
		_, _ = fmt.Fprintln(w)
		tw = tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "DOMAIN\tSOURCE\tHERE")
		for _, p := range rep.Probes {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", p.Domain, probeSummary(p.Source), probeSummary(p.Target))
		}
		_ = tw.Flush()
	}
	for _, n := range rep.Notes {
		_, _ = fmt.Fprintf(w, "\nnote: %s\n", n)
	}
	if rep.Counts.Blocking > 0 {
		_, _ = fmt.Fprintln(w, "\nBlocking differences remain: do not switch DNS yet.")
	}
}

func probeSummary(p migrateverify.ProbeResult) string {
	if p.Error != "" {
		return "error: " + p.Error
	}
	s := fmt.Sprintf("%d %dms", p.Status, p.LatencyMS)
	if p.TLS {
		if p.TLSValid {
			s += fmt.Sprintf(" tls %dd", p.TLSDaysLeft)
		} else {
			s += " tls invalid"
		}
	}
	if p.RedirectTarget != "" {
		s += " -> " + p.RedirectTarget
	}
	return s
}

func migrateVerifyUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s migrate verify <coolify|dokploy|caprover> --url URL --token TOKEN [flags]
  %[1]s migrate verify --recheck
  %[1]s migrate verify --latest

Proves this instance matches the source platform before you cut over:
apps (matched by name), domains, env var NAMES (values are never read into
the report), databases and engines, volume mounts, scheduled jobs, and a live
HTTP probe of every domain on both sides (status, TLS validity and expiry,
latency, redirect target). The report is stored for the dashboard and MCP.

Exit code 1 when any blocking difference exists. Probes never follow
redirects and refuse private and loopback addresses unless allowed.

Flags:
  --url string                source platform base URL
  --token string              source API token (or %[2]s)
  --only string               compare only this source app id or name (repeatable)
  --no-probes                 skip the live HTTP probes
  --probe-timeout duration    per-request probe timeout (env %[3]s, default 10s)
  --probe-concurrency int     domains probed at once (env %[4]s, default 6)
  --target-ip string          probe this instance's ingress at this IP while DNS still points at the source
  --source-ip string          probe the source's ingress at this IP
  --allow-private-probes      allow private and loopback probe targets (control plane needs %[5]s=true)
  --allow-private             allow a private-network source address
  --insecure-tls              skip TLS verification of the source
  --recheck                   run again with the credential the control plane still holds
  --latest                    print the last stored report without running a check
  --output string             json, table or text (--json is shorthand for --output json)
  --query string              JMESPath expression to filter the result
  --api-url, --profile        which control plane to ask
`, prog, envMigrateSourceToken, migrateverify.EnvProbeTimeout, migrateverify.EnvProbeConcurrency, migrateverify.EnvAllowPrivateProbe)
}
