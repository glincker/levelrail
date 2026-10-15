package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func auditSinksUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s audit-sinks list [flags]                                list streaming sinks with cursor and health
  %[1]s audit-sinks create --name N --kind K [flags]            add a syslog, webhook or s3 sink
  %[1]s audit-sinks test <id> [flags]                           send a probe entry and report the result
  %[1]s audit-sinks deliveries <id> [flags]                     show the batch delivery history
  %[1]s audit-sinks enable|disable <id> [flags]                 pause or resume streaming
  %[1]s audit-sinks rotate-secret <id> [flags]                  new webhook signing secret, printed once
  %[1]s audit-sinks delete <id> [flags]                         remove a sink and its history
`, prog)
}

// runAuditSinks handles "audit-sinks ...".
func runAuditSinks(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, auditSinksUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, auditSinksUsage(prog))
		return exitOK
	case "list":
		return runAuditSinkList(prog, rest, stdout, stderr, lookupEnv)
	case "create":
		return runAuditSinkCreate(prog, rest, stdout, stderr, lookupEnv)
	case "test":
		return runAuditSinkTest(prog, rest, stdout, stderr, lookupEnv)
	case "deliveries":
		return runAuditSinkDeliveries(prog, rest, stdout, stderr, lookupEnv)
	case "enable", "disable":
		return runAuditSinkToggle(prog, args[0], rest, stdout, stderr, lookupEnv)
	case "rotate-secret":
		return runAuditSinkRotate(prog, rest, stdout, stderr, lookupEnv)
	case "delete":
		return runAuditSinkDelete(prog, rest, stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown audit-sinks subcommand %q\n\n%s", prog, args[0], auditSinksUsage(prog))
	return exitUsage
}

func auditSinkIDCall(prog, label string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) (cliCall, string, int, bool) {
	usage := fmt.Sprintf("Usage:\n  %s %s <sink id> [flags]\n%s", prog, label, commonFlagsHelp)
	c, code, ok := parseCLICall(prog, label, usage, args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return cliCall{}, "", code, false
	}
	id, ok := requireOneArg(c.fs, stderr, prog, label, "sink id")
	if !ok {
		return cliCall{}, "", exitUsage, false
	}
	return c, id, exitOK, true
}

func auditSinkTarget(k apiclient.AuditSink) string {
	switch k.Kind {
	case "syslog":
		return k.Config.Address
	case "webhook":
		return k.Config.URL
	}
	return k.Config.Bucket
}

func runAuditSinkList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCLICall(prog, "audit-sinks list", auditSinksUsage(prog)+commonFlagsHelp, args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	sinks, err := c.client.ListAuditSinks(context.Background())
	if err != nil {
		return c.fail(fmt.Errorf("list audit sinks: %w", err))
	}
	return c.render(sinks, func() {
		if len(sinks) == 0 {
			_, _ = fmt.Fprintln(stdout, "no audit sinks")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "ID\tNAME\tKIND\tENABLED\tCURSOR\tFAILURES\tLAST DELIVERED\tTARGET\tLAST ERROR")
		for _, k := range sinks {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%t\t%d\t%d\t%s\t%s\t%s\n", k.ID, k.Name, k.Kind, k.Enabled, k.CursorSeq, k.ConsecutiveFailures, dash(k.LastDeliveredAt), auditSinkTarget(k), dash(k.LastError))
		}
		_ = tw.Flush()
	})
}

func runAuditSinkCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var req apiclient.CreateAuditSinkRequest
	var disabled bool
	usage := fmt.Sprintf(`Usage:
  %[1]s audit-sinks create --name NAME --kind syslog|webhook|s3 [flags]

Flags:
  --name string               sink name (required)
  --kind string               syslog, webhook or s3 (required)
  --address string            syslog host:port
  --tls                       syslog over TLS
  --url string                webhook https endpoint, signed with an HMAC secret
  --endpoint string           s3 endpoint URL (empty for AWS)
  --region string             s3 region
  --bucket string             s3 bucket
  --prefix string             s3 key prefix
  --path-style                s3 path-style addressing
  --access-key-id string      s3 access key id (stored encrypted)
  --secret-access-key string  s3 secret access key (stored encrypted)
  --backfill                  stream the retained history, not only new entries
  --disabled                  create the sink paused
%[2]s`, prog, commonFlagsHelp)
	c, code, ok := parseCLICall(prog, "audit-sinks create", usage, args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&req.Name, "name", "", "sink name")
		fs.StringVar(&req.Kind, "kind", "", "syslog, webhook or s3")
		fs.StringVar(&req.Config.Address, "address", "", "syslog host:port")
		fs.BoolVar(&req.Config.TLS, "tls", false, "syslog over TLS")
		fs.StringVar(&req.Config.URL, "url", "", "webhook URL")
		fs.StringVar(&req.Config.Endpoint, "endpoint", "", "s3 endpoint")
		fs.StringVar(&req.Config.Region, "region", "", "s3 region")
		fs.StringVar(&req.Config.Bucket, "bucket", "", "s3 bucket")
		fs.StringVar(&req.Config.Prefix, "prefix", "", "s3 key prefix")
		fs.BoolVar(&req.Config.PathStyle, "path-style", false, "s3 path-style addressing")
		fs.StringVar(&req.AccessKeyID, "access-key-id", "", "s3 access key id")
		fs.StringVar(&req.SecretAccessKey, "secret-access-key", "", "s3 secret access key")
		fs.BoolVar(&req.Backfill, "backfill", false, "stream retained history")
		fs.BoolVar(&disabled, "disabled", false, "create paused")
	})
	if !ok {
		return code
	}
	if req.Name == "" || req.Kind == "" {
		return c.invalid("--name and --kind are required")
	}
	enabled := !disabled
	req.Enabled = &enabled
	created, err := c.client.CreateAuditSink(context.Background(), req)
	if err != nil {
		return c.fail(fmt.Errorf("create audit sink %q: %w", req.Name, err))
	}
	return c.render(created, func() {
		_, _ = fmt.Fprintf(stdout, "audit sink %q created (id %s)\n", created.Name, created.ID)
		if created.SigningSecret != "" {
			_, _ = fmt.Fprintf(stdout, "signing secret (shown once): %s\n", created.SigningSecret)
		}
	})
}

func runAuditSinkTest(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, id, code, ok := auditSinkIDCall(prog, "audit-sinks test", args, stdout, stderr, lookupEnv)
	if !ok {
		return code
	}
	res, err := c.client.TestAuditSink(context.Background(), id)
	if err != nil {
		return c.fail(fmt.Errorf("test audit sink %q: %w", id, err))
	}
	status := c.render(res, func() {
		if res.OK {
			_, _ = fmt.Fprintf(stdout, "sink %s accepted the test entry\n", id)
			return
		}
		_, _ = fmt.Fprintf(stdout, "sink %s failed: %s\n", id, res.Error)
	})
	if status == exitOK && !res.OK {
		return exitCheckFailed
	}
	return status
}

func runAuditSinkDeliveries(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, id, code, ok := auditSinkIDCall(prog, "audit-sinks deliveries", args, stdout, stderr, lookupEnv)
	if !ok {
		return code
	}
	ds, err := c.client.ListAuditSinkDeliveries(context.Background(), id)
	if err != nil {
		return c.fail(fmt.Errorf("list deliveries for %q: %w", id, err))
	}
	return c.render(ds, func() {
		if len(ds) == 0 {
			_, _ = fmt.Fprintln(stdout, "no deliveries yet")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "ID\tSEQ\tENTRIES\tSTATE\tATTEMPT\tWHEN\tERROR")
		for _, d := range ds {
			_, _ = fmt.Fprintf(tw, "%s\t%d-%d\t%d\t%s\t%d\t%s\t%s\n", d.ID, d.FromSeq, d.ToSeq, d.Entries, d.State, d.Attempt, d.CreatedAt, dash(d.LastError))
		}
		_ = tw.Flush()
	})
}

func runAuditSinkToggle(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, id, code, ok := auditSinkIDCall(prog, "audit-sinks "+verb, args, stdout, stderr, lookupEnv)
	if !ok {
		return code
	}
	enabled := verb == "enable"
	k, err := c.client.UpdateAuditSink(context.Background(), id, apiclient.UpdateAuditSinkRequest{Enabled: &enabled})
	if err != nil {
		return c.fail(fmt.Errorf("%s audit sink %q: %w", verb, id, err))
	}
	return c.render(k, func() { _, _ = fmt.Fprintf(stdout, "audit sink %q %sd\n", k.Name, verb) })
}

func runAuditSinkRotate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, id, code, ok := auditSinkIDCall(prog, "audit-sinks rotate-secret", args, stdout, stderr, lookupEnv)
	if !ok {
		return code
	}
	k, err := c.client.RotateAuditSinkSecret(context.Background(), id)
	if err != nil {
		return c.fail(fmt.Errorf("rotate secret for %q: %w", id, err))
	}
	return c.render(k, func() {
		_, _ = fmt.Fprintf(stdout, "new signing secret for %q (shown once): %s\n", k.Name, k.SigningSecret)
	})
}

func runAuditSinkDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, id, code, ok := auditSinkIDCall(prog, "audit-sinks delete", args, stdout, stderr, lookupEnv)
	if !ok {
		return code
	}
	if err := c.client.DeleteAuditSink(context.Background(), id); err != nil {
		return c.fail(fmt.Errorf("delete audit sink %q: %w", id, err))
	}
	return c.render(map[string]bool{"deleted": true}, func() {
		_, _ = fmt.Fprintf(stdout, "audit sink %q deleted\n", id)
	})
}

// runAuditVerify handles "audit-verify": walk the audit hash chain and
// report the first broken link. Exits non-zero when the chain is broken.
func runAuditVerify(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	usage := fmt.Sprintf("Usage:\n  %s audit-verify [flags]\n\nVerifies the audit log hash chain. Entries written before chaining\nwas enabled are reported as unchained and not checked.\n%s", prog, commonFlagsHelp)
	c, code, ok := parseCLICall(prog, "audit-verify", usage, args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	rep, err := c.client.VerifyAuditChain(context.Background())
	if err != nil {
		return c.fail(fmt.Errorf("verify audit chain: %w", err))
	}
	status := c.render(rep, func() {
		if rep.OK {
			_, _ = fmt.Fprintf(stdout, "audit chain intact: %d chained entries verified, %d unchained (written before chaining)\n", rep.Chained, rep.Unchained)
			return
		}
		b := rep.FirstBroken
		_, _ = fmt.Fprintf(stdout, "audit chain BROKEN at seq %d (%s): %s\n", b.Seq, dash(b.ID), b.Reason)
	})
	if status == exitOK && !rep.OK {
		return exitCheckFailed
	}
	return status
}
