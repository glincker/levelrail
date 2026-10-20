package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const (
	evidencePollEvery   = 2 * time.Second
	evidenceStatusDone  = "complete"
	evidenceStatusError = "failed"
)

func complianceUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s compliance evidence create --from DATE --to DATE [--out FILE] [--no-wait]   build a signed evidence pack and download it
  %[1]s compliance evidence list [flags]                                          list evidence packs
  %[1]s compliance evidence download <id> [--out FILE] [flags]                    download a finished pack
  %[1]s compliance evidence verify <id> [flags]                                   re-check a stored pack against its manifest and signature
  %[1]s compliance access-review start [--name N] [--due-days D] [--repeat-days R]  open a review of every user, membership, token and grant
  %[1]s compliance access-review list [flags]                                     list review campaigns
  %[1]s compliance access-review show <id> [flags]                                show a campaign and its rows
  %[1]s compliance access-review keep|revoke <id> <item-id>... [flags]            record a decision per row
  %[1]s compliance access-review complete <id> [flags]                            apply revocations and close the campaign

Evidence packs are records for an auditor to examine. They are not a certification.
`, prog)
}

// runCompliance handles "compliance ...".
func runCompliance(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, complianceUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, complianceUsage(prog))
		return exitOK
	case "evidence":
		return runComplianceEvidence(prog, args[1:], stdout, stderr, lookupEnv)
	case "access-review":
		return runComplianceAccessReview(prog, args[1:], stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown compliance command %q\n\n%s", prog, args[0], complianceUsage(prog))
	return exitUsage
}

func runComplianceEvidence(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, complianceUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "create":
		return runEvidenceCreate(prog, rest, stdout, stderr, lookupEnv)
	case "list":
		return runEvidenceList(prog, rest, stdout, stderr, lookupEnv)
	case "download":
		return runEvidenceDownload(prog, rest, stdout, stderr, lookupEnv)
	case "verify":
		return runEvidenceVerify(prog, rest, stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown compliance evidence command %q\n\n%s", prog, args[0], complianceUsage(prog))
	return exitUsage
}

func runComplianceAccessReview(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, complianceUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "start":
		return runReviewStart(prog, rest, stdout, stderr, lookupEnv)
	case "list":
		return runReviewList(prog, rest, stdout, stderr, lookupEnv)
	case "show":
		return runReviewShow(prog, rest, stdout, stderr, lookupEnv)
	case "keep", "revoke":
		return runReviewDecide(prog, args[0], rest, stdout, stderr, lookupEnv)
	case "complete":
		return runReviewComplete(prog, rest, stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown compliance access-review command %q\n\n%s", prog, args[0], complianceUsage(prog))
	return exitUsage
}

func runEvidenceCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var from, to, out string
	var noWait bool
	usage := fmt.Sprintf("Usage:\n  %s compliance evidence create --from DATE --to DATE [--out FILE] [--no-wait]\n%s", prog, commonFlagsHelp)
	c, code, ok := parseCLICall(prog, "compliance evidence create", usage, args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&from, "from", "", "first day covered (YYYY-MM-DD)")
		fs.StringVar(&to, "to", "", "last day covered, inclusive (YYYY-MM-DD)")
		fs.StringVar(&out, "out", "", "file to write the archive to (default <id>.zip)")
		fs.BoolVar(&noWait, "no-wait", false, "print the pack id and return without waiting")
	})
	if !ok {
		return code
	}
	if from == "" || to == "" {
		return c.invalid("--from and --to are required")
	}
	ctx := context.Background()
	pack, err := c.client.CreateEvidencePack(ctx, from, to)
	if err != nil {
		return c.fail(fmt.Errorf("create evidence pack: %w", err))
	}
	if noWait {
		return c.render(pack, func() { _, _ = fmt.Fprintf(stdout, "evidence pack %s started\n", pack.ID) })
	}
	for pack.Status == "running" {
		time.Sleep(evidencePollEvery)
		if pack, err = c.client.GetEvidencePack(ctx, pack.ID); err != nil {
			return c.fail(fmt.Errorf("check evidence pack: %w", err))
		}
		if !c.jsonOut {
			_, _ = fmt.Fprintf(stderr, "\r%-24s %3d%%", pack.Step, pack.Progress)
		}
	}
	if pack.Status != evidenceStatusDone {
		return c.fail(fmt.Errorf("evidence pack %s %s: %s", pack.ID, pack.Status, pack.Error))
	}
	if out == "" {
		out = pack.ID + ".zip"
	}
	if err := downloadPack(ctx, c.client, pack.ID, out); err != nil {
		return c.fail(err)
	}
	return c.render(pack, func() {
		_, _ = fmt.Fprintf(stdout, "\nwrote %s (%d bytes, sha256 %s)\n", out, pack.SizeBytes, pack.SHA256)
	})
}

func downloadPack(ctx context.Context, client *apiclient.Client, id, out string) error {
	f, err := os.OpenFile(out, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600) //nolint:gosec // operator-chosen output path
	if err != nil {
		return fmt.Errorf("create %s: %w", out, err)
	}
	if err := client.DownloadEvidencePack(ctx, id, f); err != nil {
		_ = f.Close()
		_ = os.Remove(out)
		return fmt.Errorf("download evidence pack: %w", err)
	}
	return f.Close()
}

func runEvidenceList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCLICall(prog, "compliance evidence list", complianceUsage(prog)+commonFlagsHelp, args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	packs, err := c.client.ListEvidencePacks(context.Background())
	if err != nil {
		return c.fail(fmt.Errorf("list evidence packs: %w", err))
	}
	return c.render(packs, func() {
		if len(packs) == 0 {
			_, _ = fmt.Fprintln(stdout, "no evidence packs")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "ID\tSTATUS\tFROM\tTO\tSIZE\tCREATED")
		for _, p := range packs {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%s\n", p.ID, p.Status, p.RangeFrom, p.RangeTo, p.SizeBytes, p.CreatedAt)
		}
		_ = tw.Flush()
	})
}

func runEvidenceDownload(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var out string
	usage := fmt.Sprintf("Usage:\n  %s compliance evidence download <id> [--out FILE]\n%s", prog, commonFlagsHelp)
	c, code, ok := parseCLICall(prog, "compliance evidence download", usage, args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&out, "out", "", "file to write the archive to (default <id>.zip)")
	})
	if !ok {
		return code
	}
	id, ok := requireOneArg(c.fs, stderr, prog, "compliance evidence download", "pack id")
	if !ok {
		return exitUsage
	}
	if out == "" {
		out = id + ".zip"
	}
	if err := downloadPack(context.Background(), c.client, id, out); err != nil {
		return c.fail(err)
	}
	return c.render(map[string]string{"file": out}, func() { _, _ = fmt.Fprintf(stdout, "wrote %s\n", out) })
}

func runEvidenceVerify(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, id, code, ok := eventWebhookIDCall(prog, "compliance evidence verify", "pack id", args, stdout, stderr, lookupEnv)
	if !ok {
		return code
	}
	rep, err := c.client.VerifyEvidencePack(context.Background(), id)
	if err != nil {
		return c.fail(fmt.Errorf("verify evidence pack: %w", err))
	}
	status := c.render(rep, func() {
		for _, f := range rep.Files {
			state := "ok"
			if !f.OK {
				state = "FAILED: " + f.Reason
			}
			_, _ = fmt.Fprintf(stdout, "%-28s %s\n", f.Name, state)
		}
		_, _ = fmt.Fprintf(stdout, "signature valid: %t (key %s)\npack valid: %t\n", rep.SignatureValid, rep.KeyID, rep.Valid)
	})
	if status == exitOK && !rep.Valid {
		return exitCheckFailed
	}
	return status
}

func runReviewStart(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var req apiclient.CreateAccessReviewRequest
	usage := fmt.Sprintf("Usage:\n  %s compliance access-review start [--name N] [--due-days D] [--repeat-days R]\n%s", prog, commonFlagsHelp)
	c, code, ok := parseCLICall(prog, "compliance access-review start", usage, args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&req.Name, "name", "", "campaign name")
		fs.IntVar(&req.DueDays, "due-days", 14, "days until the review is overdue")
		fs.IntVar(&req.RepeatDays, "repeat-days", 0, "open the next review this many days after the previous one (0 for on demand only)")
	})
	if !ok {
		return code
	}
	res, err := c.client.CreateAccessReview(context.Background(), req)
	if err != nil {
		return c.fail(fmt.Errorf("start access review: %w", err))
	}
	return c.render(res, func() {
		_, _ = fmt.Fprintf(stdout, "access review %s opened with %d rows, due %s\n", res.ID, res.Total, res.DueAt)
	})
}

func runReviewList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCLICall(prog, "compliance access-review list", complianceUsage(prog)+commonFlagsHelp, args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	list, err := c.client.ListAccessReviews(context.Background())
	if err != nil {
		return c.fail(fmt.Errorf("list access reviews: %w", err))
	}
	return c.render(list, func() {
		if len(list) == 0 {
			_, _ = fmt.Fprintln(stdout, "no access reviews")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "ID\tNAME\tSTATUS\tDUE\tOVERDUE\tDECIDED\tREVOKE")
		for _, r := range list {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%t\t%d/%d\t%d\n", r.ID, r.Name, r.Status, r.DueAt, r.Overdue, r.Decided, r.Total, r.ToRevoke)
		}
		_ = tw.Flush()
	})
}

func runReviewShow(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, id, code, ok := eventWebhookIDCall(prog, "compliance access-review show", "campaign id", args, stdout, stderr, lookupEnv)
	if !ok {
		return code
	}
	d, err := c.client.GetAccessReview(context.Background(), id)
	if err != nil {
		return c.fail(fmt.Errorf("get access review: %w", err))
	}
	return c.render(d, func() {
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "ITEM\tKIND\tSUBJECT\tROLE\tLAST ACTIVE\tDECISION")
		for _, it := range d.Items {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", it.ID, it.Kind, it.Label, it.Role, it.LastActiveAt, it.Decision)
		}
		_ = tw.Flush()
	})
}

func runReviewDecide(prog, decision string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	label := "compliance access-review " + decision
	usage := fmt.Sprintf("Usage:\n  %s %s <campaign-id> <item-id>... [flags]\n%s", prog, label, commonFlagsHelp)
	c, code, ok := parseCLICall(prog, label, usage, args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	rest := c.fs.Args()
	if len(rest) < 2 {
		return c.invalid("%s needs a campaign id and at least one item id", label)
	}
	for _, item := range rest[1:] {
		if err := c.client.DecideAccessReviewItem(context.Background(), rest[0], item, decision); err != nil {
			return c.fail(fmt.Errorf("%s %s: %w", decision, item, err))
		}
	}
	return c.render(map[string]any{"decision": decision, "items": len(rest) - 1}, func() {
		_, _ = fmt.Fprintf(stdout, "recorded %q for %d row(s)\n", decision, len(rest)-1)
	})
}

func runReviewComplete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, id, code, ok := eventWebhookIDCall(prog, "compliance access-review complete", "campaign id", args, stdout, stderr, lookupEnv)
	if !ok {
		return code
	}
	res, err := c.client.CompleteAccessReview(context.Background(), id)
	if err != nil {
		return c.fail(fmt.Errorf("complete access review: %w", err))
	}
	return c.render(res, func() { _, _ = fmt.Fprintf(stdout, "access review completed, %d revocation(s) applied\n", res.Revoked) })
}
