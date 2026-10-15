package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runDataExport handles "export destinations ..." and "export audit ...",
// the outbound data export surface. It is reached from runExport, which
// keeps the existing IaC export for every other argument shape.
func runDataExport(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if args[0] == "audit" {
		return runExportAudit(prog, args[1:], stdout, stderr, lookupEnv)
	}
	rest := args[1:]
	if len(rest) == 0 {
		_, _ = fmt.Fprint(stderr, exportDestinationsUsage(prog))
		return exitUsage
	}
	switch rest[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, exportDestinationsUsage(prog))
		return exitOK
	case "list":
		return runExportDestList(prog, rest[1:], stdout, stderr, lookupEnv)
	case "create":
		return runExportDestCreate(prog, rest[1:], stdout, stderr, lookupEnv)
	case "test":
		return runExportDestTest(prog, rest[1:], stdout, stderr, lookupEnv)
	case "status":
		return runExportDestStatus(prog, rest[1:], stdout, stderr, lookupEnv)
	case "enable", "disable":
		return runExportDestToggle(prog, rest[0], rest[1:], stdout, stderr, lookupEnv)
	case "delete":
		return runExportDestDelete(prog, rest[1:], stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown export destinations subcommand %q\n\n%s", prog, rest[0], exportDestinationsUsage(prog))
	return exitUsage
}

func exportDestinationsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s export destinations list [flags]                 list data export destinations and their delivery state
  %[1]s export destinations create --name N --kind K [flags]   add a destination
  %[1]s export destinations test <id> [flags]            send one probe and report the result
  %[1]s export destinations status <id> [flags]          show delivery state: last success, error, queue depth
  %[1]s export destinations enable|disable <id> [flags]  pause or resume delivery
  %[1]s export destinations delete <id> [flags]          remove a destination and its stored secrets
  %[1]s export audit [--from T] [--to T] [--format ndjson|csv] [flags]   download the audit log

Kinds: logs_http, logs_loki, logs_otlp, logs_syslog, logs_newrelic, logs_datadog, logs_axiom,
logs_betterstack, audit_webhook, audit_s3, metrics_otlp.
Run "%[1]s export destinations create -h" for the create flags.
`, prog)
}

func runExportDestList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCLICall(prog, "export destinations list", exportDestinationsUsage(prog)+commonFlagsHelp, args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	dests, err := c.client.ListExportDestinations(context.Background())
	if err != nil {
		return c.fail(fmt.Errorf("list export destinations: %w", err))
	}
	return c.render(dests, func() { printExportTable(stdout, dests) })
}

func printExportTable(out io.Writer, dests []apiclient.ExportDestinationResource) {
	if len(dests) == 0 {
		_, _ = fmt.Fprintln(out, "no export destinations")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tKIND\tENABLED\tSENT\tDROPPED\tQUEUE\tCIRCUIT\tLAST SUCCESS\tLAST ERROR")
	for _, d := range dests {
		circuit := "closed"
		if d.Status.CircuitOpen {
			circuit = "open"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%t\t%d\t%d\t%d\t%s\t%s\t%s\n", d.ID, d.Name, d.Kind, d.Enabled,
			d.Status.SentTotal, d.Status.DroppedTotal, d.Status.QueueDepth, circuit, dash(d.Status.LastSuccessAt), dash(d.Status.LastError))
	}
	_ = tw.Flush()
}

func runExportDestCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var name, kind, endpoint, minLevel, target, prefix, hmacKey string
	var apiKeyEnv, region, site, dataset string
	var interval int
	var backfill, disabled bool
	var apps csvListFlag
	headers := stringMapFlag{}
	labels := stringMapFlag{}
	usage := fmt.Sprintf(`Usage:
  %[1]s export destinations create --name NAME --kind KIND [flags]

Flags:
  --name string          destination name (required)
  --kind string          logs_http, logs_loki, logs_otlp, logs_syslog, logs_newrelic, logs_datadog, logs_axiom,
                         logs_betterstack, audit_webhook, audit_s3 or metrics_otlp (required)
  --endpoint string      http(s) URL, or tcp://host:port or tls://host:port for syslog; optional for vendor kinds
  --api-key-env NAME     environment variable holding a vendor key (logs_newrelic, logs_datadog, logs_axiom, logs_betterstack)
  --region us|eu         New Relic and Axiom region
  --site HOST            Datadog site, for example datadoghq.eu
  --dataset NAME         Axiom dataset
  --app string           only these apps (comma separated or repeated), log kinds and metrics_otlp
  --min-level string     debug, info, warn or error, log kinds only
  --header KEY=VALUE     request header sent with every delivery, stored encrypted (repeatable)
  --hmac-key string      signing key for audit_webhook, at least 16 characters, stored encrypted
  --storage-target ID    storage destination id for audit_s3
  --prefix string        object key prefix for audit_s3 (default audit-log)
  --label KEY=VALUE      static Loki label or OTLP resource attribute (repeatable)
  --interval int         metrics_otlp push interval in seconds (10 to 3600)
  --backfill             audit kinds: start from the oldest audit row instead of now
  --disabled             create the destination paused
%[2]s`, prog, commonFlagsHelp)
	c, code, ok := parseCLICall(prog, "export destinations create", usage, args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&name, "name", "", "destination name")
		fs.StringVar(&kind, "kind", "", "destination kind")
		fs.StringVar(&endpoint, "endpoint", "", "destination endpoint")
		fs.Var(&apps, "app", "only these apps")
		fs.StringVar(&minLevel, "min-level", "", "minimum log level")
		fs.Var(headers, "header", "request header KEY=VALUE")
		fs.StringVar(&hmacKey, "hmac-key", "", "audit webhook signing key")
		fs.StringVar(&target, "storage-target", "", "storage destination id")
		fs.StringVar(&prefix, "prefix", "", "object key prefix")
		fs.Var(labels, "label", "static Loki label KEY=VALUE")
		fs.StringVar(&apiKeyEnv, "api-key-env", "", "environment variable holding a vendor key")
		fs.StringVar(&region, "region", "", "vendor region, us or eu")
		fs.StringVar(&site, "site", "", "Datadog site")
		fs.StringVar(&dataset, "dataset", "", "Axiom dataset")
		fs.IntVar(&interval, "interval", 0, "metrics push interval in seconds")
		fs.BoolVar(&backfill, "backfill", false, "start audit export from the oldest row")
		fs.BoolVar(&disabled, "disabled", false, "create paused")
	})
	if !ok {
		return code
	}
	if name == "" || kind == "" {
		return c.invalid("--name and --kind are required")
	}
	enabled := !disabled
	req := apiclient.CreateExportDestinationRequest{
		Name: name, Kind: kind, Endpoint: endpoint, Enabled: &enabled, Apps: apps, MinLevel: minLevel, HMACKey: hmacKey,
		Options: apiclient.ExportOptions{StorageTargetID: target, Prefix: prefix, IntervalSeconds: interval, Backfill: backfill, Region: region, Site: site, Dataset: dataset},
	}
	if apiKeyEnv != "" {
		v, found := lookupEnv(apiKeyEnv)
		if !found || v == "" {
			return c.invalid("environment variable %s is not set", apiKeyEnv)
		}
		req.APIKey = v
	}
	if len(headers) > 0 {
		req.Headers = headers
	}
	if len(labels) > 0 {
		req.Options.Labels = labels
	}
	created, err := c.client.CreateExportDestination(context.Background(), req)
	if err != nil {
		return c.fail(fmt.Errorf("create export destination %q: %w", name, err))
	}
	return c.render(created, func() {
		_, _ = fmt.Fprintf(stdout, "export destination %q created (id %s, kind %s)\n", created.Name, created.ID, created.Kind)
	})
}

func exportIDCall(prog, label string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) (cliCall, string, int, bool) {
	usage := fmt.Sprintf("Usage:\n  %s %s <id> [flags]\n%s", prog, label, commonFlagsHelp)
	c, code, ok := parseCLICall(prog, label, usage, args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return cliCall{}, "", code, false
	}
	id, ok := requireOneArg(c.fs, stderr, prog, label, "export destination id")
	if !ok {
		return cliCall{}, "", exitUsage, false
	}
	return c, id, exitOK, true
}

func runExportDestTest(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, id, code, ok := exportIDCall(prog, "export destinations test", args, stdout, stderr, lookupEnv)
	if !ok {
		return code
	}
	res, err := c.client.TestExportDestination(context.Background(), id)
	if err != nil {
		return c.fail(fmt.Errorf("test export destination %q: %w", id, err))
	}
	status := c.render(res, func() {
		if res.OK {
			_, _ = fmt.Fprintf(stdout, "destination %s accepted the test probe\n", id)
			return
		}
		_, _ = fmt.Fprintf(stdout, "destination %s failed: %s\n", id, res.Error)
	})
	if status == exitOK && !res.OK {
		return exitCheckFailed
	}
	return status
}

func runExportDestStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, id, code, ok := exportIDCall(prog, "export destinations status", args, stdout, stderr, lookupEnv)
	if !ok {
		return code
	}
	d, err := c.client.GetExportDestination(context.Background(), id)
	if err != nil {
		return c.fail(fmt.Errorf("get export destination %q: %w", id, err))
	}
	return c.render(d, func() {
		s := d.Status
		_, _ = fmt.Fprintf(stdout, "%s (%s, %s)\n  enabled:       %t\n  last success:  %s\n  last error:    %s\n  sent:          %d\n  dropped:       %d\n  failures:      %d\n  queue depth:   %d\n  circuit open:  %t\n",
			d.Name, d.ID, d.Kind, d.Enabled, dash(s.LastSuccessAt), dash(strings.TrimSpace(s.LastError)), s.SentTotal, s.DroppedTotal, s.FailuresTotal, s.QueueDepth, s.CircuitOpen)
		if s.CursorAt != "" {
			_, _ = fmt.Fprintf(stdout, "  audit cursor:  %s\n", s.CursorAt)
		}
	})
}

func runExportDestToggle(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, id, code, ok := exportIDCall(prog, "export destinations "+verb, args, stdout, stderr, lookupEnv)
	if !ok {
		return code
	}
	enabled := verb == "enable"
	d, err := c.client.UpdateExportDestination(context.Background(), id, apiclient.UpdateExportDestinationRequest{Enabled: &enabled})
	if err != nil {
		return c.fail(fmt.Errorf("%s export destination %q: %w", verb, id, err))
	}
	return c.render(d, func() { _, _ = fmt.Fprintf(stdout, "export destination %q %sd\n", d.Name, verb) })
}

func runExportDestDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, id, code, ok := exportIDCall(prog, "export destinations delete", args, stdout, stderr, lookupEnv)
	if !ok {
		return code
	}
	if err := c.client.DeleteExportDestination(context.Background(), id); err != nil {
		return c.fail(fmt.Errorf("delete export destination %q: %w", id, err))
	}
	return c.render(map[string]bool{"deleted": true}, func() {
		_, _ = fmt.Fprintf(stdout, "export destination %q deleted\n", id)
	})
}

func runExportAudit(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var from, to, format string
	usage := fmt.Sprintf(`Usage:
  %[1]s export audit [--from RFC3339] [--to RFC3339] [--format ndjson|csv] [flags]

Streams the audit log, oldest first, with tokens and credentials redacted.
%[2]s`, prog, commonFlagsHelp)
	c, code, ok := parseCLICall(prog, "export audit", usage, args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&from, "from", "", "start of the range, inclusive (RFC3339)")
		fs.StringVar(&to, "to", "", "end of the range, exclusive (RFC3339)")
		fs.StringVar(&format, "format", "ndjson", "ndjson or csv")
	})
	if !ok {
		return code
	}
	body, err := c.client.ExportAuditLog(context.Background(), from, to, format)
	if err != nil {
		return c.fail(fmt.Errorf("export audit log: %w", err))
	}
	_, _ = stdout.Write(body)
	return exitOK
}
