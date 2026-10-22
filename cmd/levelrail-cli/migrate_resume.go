package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/migrationgate"
)

const resumeFollowEvery = 2 * time.Second

// hasResumeFlag reports whether args ask to resume a migration job.
func hasResumeFlag(args []string) bool {
	for _, a := range args {
		if a == "--resume" || a == "-resume" || strings.HasPrefix(a, "--resume=") || strings.HasPrefix(a, "-resume=") {
			return true
		}
	}
	return false
}

type ackList []string

func (a *ackList) String() string { return strings.Join(*a, ",") }
func (a *ackList) Set(v string) error {
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			*a = append(*a, p)
		}
	}
	return nil
}

type resumeFlags struct {
	job, only, apiURL, profile, token string
	tokenStdin, retry, retire, probe  bool
	follow, jsonOut                   bool
	wait                              time.Duration
	acks                              ackList
}

// runMigrateResume implements "migrate [source] --resume JOB": show where a migration job stands,
// retry what failed, follow it, or ask whether the old instance is safe to delete.
func runMigrateResume(prog string, args []string, stdin io.Reader, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var f resumeFlags
	fs := flag.NewFlagSet(prog+" migrate --resume", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&f.job, "resume", "", "the migration job (import session) id to resume")
	fs.BoolVar(&f.retry, "retry", false, "stage or verify again the items that failed or were interrupted")
	fs.StringVar(&f.only, "only", "", "comma-separated item names for --retry")
	fs.BoolVar(&f.retire, "retire", false, "check whether the old instance is safe to delete; exits 1 until every gate passes")
	fs.BoolVar(&f.probe, "probe", false, "with --retire, run the live DNS and response checks for every domain")
	fs.Var(&f.acks, "ack", "with --retire, confirm a manual step: "+migrationgate.GateBackup+", "+migrationgate.GateTraffic+" or "+migrationgate.GateLeftovers+" (repeatable)")
	fs.BoolVar(&f.follow, "follow", false, "keep printing progress until the job stops working")
	fs.DurationVar(&f.wait, "wait", 30*time.Minute, "how long --follow waits")
	fs.BoolVar(&f.tokenStdin, "token-stdin", false, "read the source token from stdin when the job must reconnect")
	fs.StringVar(&f.token, "token", "", "source token for a reconnect (prefer "+envImportSourceToken+" or --token-stdin)")
	fs.StringVar(&f.apiURL, "api-url", "", "control plane API base URL")
	fs.StringVar(&f.profile, "profile", "", "named credentials profile for the control plane")
	fs.BoolVar(&f.jsonOut, "json", false, "print JSON")
	rest := args
	if len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
		rest = rest[1:]
	}
	if err := fs.Parse(rest); err != nil {
		return exitUsage
	}
	if strings.TrimSpace(f.job) == "" {
		_, _ = fmt.Fprintf(stderr, "%s: --resume needs a job id\n", prog)
		return exitUsage
	}
	if f.probe && !f.retire {
		_, _ = fmt.Fprintf(stderr, "%s: --probe only works with --retire\n", prog)
		return exitUsage
	}
	client := apiClientFromFlags(prog, f.apiURL, "", f.profile, lookupEnv)
	ctx := context.Background()
	job, err := client.MigrationJob(ctx, f.job)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("load migration job: %w", err))
	}
	if f.retry {
		if job, err = resumeRetry(ctx, prog, client, job, f, stdin, stderr, lookupEnv); err != nil {
			return reportError(stdout, stderr, f.jsonOut, err)
		}
	}
	if f.follow {
		if job, err = followJob(ctx, client, job, f, stdout); err != nil {
			return reportError(stdout, stderr, f.jsonOut, err)
		}
	}
	if f.retire {
		return runResumeRetire(ctx, prog, client, job, f, stdout, stderr)
	}
	if f.jsonOut {
		return emitJSON(stdout, stderr, job)
	}
	printMigrationJob(stdout, prog, job)
	return exitOK
}

func resumeRetry(ctx context.Context, prog string, client *Client, job apiclient.MigrationJob, f resumeFlags, stdin io.Reader, stderr io.Writer, lookupEnv func(string) (string, bool)) (apiclient.MigrationJob, error) {
	wanted := map[string]bool{}
	for _, o := range strings.Split(f.only, ",") {
		if o = strings.TrimSpace(o); o != "" {
			wanted[strings.ToLower(o)] = true
		}
	}
	var stage, verify []string
	for _, it := range job.Items {
		if !it.Retryable || (len(wanted) > 0 && !wanted[strings.ToLower(it.Name)] && !wanted[strings.ToLower(it.SourceID)]) {
			continue
		}
		if it.State == "stage-failed" {
			stage = append(stage, it.SourceID)
		} else {
			verify = append(verify, it.SourceID)
		}
	}
	if len(stage) == 0 && len(verify) == 0 {
		_, _ = fmt.Fprintf(stderr, "%s: nothing to retry\n", prog)
		return job, nil
	}
	if len(stage) > 0 && !job.Connected {
		if err := reconnectJob(ctx, prog, client, job, f, stdin, stderr, lookupEnv); err != nil {
			return job, err
		}
	}
	if len(stage) > 0 {
		if _, err := client.StageAppImportItems(ctx, job.ID, stage); err != nil {
			return job, fmt.Errorf("retry staging: %w", err)
		}
	}
	if len(verify) > 0 {
		if _, err := client.VerifyAppImport(ctx, job.ID, verify); err != nil {
			return job, fmt.Errorf("retry verification: %w", err)
		}
	}
	return client.MigrationJob(ctx, job.ID)
}

func reconnectJob(ctx context.Context, prog string, client *Client, job apiclient.MigrationJob, f resumeFlags, stdin io.Reader, stderr io.Writer, lookupEnv func(string) (string, bool)) error {
	if strings.HasPrefix(job.SourceURL, "ssh://") || strings.HasPrefix(job.SourceURL, "file://") {
		return newValidationError("this job reads its source over ssh or from a pasted file: reconnect it from the dashboard")
	}
	token, _, ok := resolveSourceToken(prog, importPlatformFlags{token: f.token, tokenStdin: f.tokenStdin}, stdin, stderr, lookupEnv)
	if !ok {
		return newValidationError("the job lost its source credential (a restart or its expiry): pass --token-stdin or set %s", envImportSourceToken)
	}
	_, err := client.ConnectAppImportSession(ctx, job.ID, apiclient.AppImportRequest{Platform: job.Platform, URL: job.SourceURL, Token: token})
	if err != nil {
		return fmt.Errorf("reconnect the source: %w", err)
	}
	return nil
}

func followJob(ctx context.Context, client *Client, job apiclient.MigrationJob, f resumeFlags, stdout io.Writer) (apiclient.MigrationJob, error) {
	deadline := time.Now().Add(f.wait)
	last := ""
	for {
		if job.Revision != last && !f.jsonOut {
			_, _ = fmt.Fprintf(stdout, "%s: %d/%d done, %d failed, %s\n", job.ID, job.Done, job.Total, job.Failed, job.Phase)
			last = job.Revision
		}
		if !job.Running && job.States["building"] == 0 {
			return job, nil
		}
		if time.Now().After(deadline) {
			return job, newValidationError("still working after %s: run the command again to keep following", f.wait)
		}
		select {
		case <-ctx.Done():
			return job, ctx.Err()
		case <-time.After(resumeFollowEvery):
		}
		var err error
		if job, err = client.MigrationJob(ctx, job.ID); err != nil {
			return job, fmt.Errorf("follow migration job: %w", err)
		}
	}
}

func runResumeRetire(ctx context.Context, prog string, client *Client, job apiclient.MigrationJob, f resumeFlags, stdout, stderr io.Writer) int {
	rep, err := client.MigrationRetire(ctx, job.ID, f.probe, f.acks)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("safe-to-delete check: %w", err))
	}
	code := exitOK
	if !rep.Safe {
		code = exitCheckFailed
	}
	if f.jsonOut {
		if c := emitJSON(stdout, stderr, rep); c != exitOK {
			return c
		}
		return code
	}
	for _, g := range rep.Gates {
		_, _ = fmt.Fprintf(stdout, "%-8s %-14s %s\n", strings.ToUpper(string(g.Status)), g.ID, g.Detail)
		for _, o := range g.Open {
			_, _ = fmt.Fprintf(stdout, "         - %s\n", o)
		}
		if g.Status != migrationgate.Pass && g.Fix != "" {
			_, _ = fmt.Fprintf(stdout, "         next: %s\n", g.Fix)
		}
	}
	if rep.Safe {
		_, _ = fmt.Fprintln(stdout, "\nEvery gate passed. Stop the old instance first, keep it stopped for a few days, then delete it.")
		return code
	}
	_, _ = fmt.Fprintln(stdout, "\nNot safe to delete yet. Keep the old instance running. Rollback guidance:")
	for _, l := range rep.Fallback {
		_, _ = fmt.Fprintf(stdout, "  - %s\n", l)
	}
	if !f.probe {
		_, _ = fmt.Fprintf(stdout, "\nAdd --probe to run the live DNS and response checks: %s migrate --resume %s --retire --probe\n", prog, job.ID)
	}
	return code
}

var nextWords = map[string]string{
	"reconnect":  "reconnect the source (pass --token-stdin with --retry, or use the dashboard)",
	"stage":      "stage the selected apps",
	"retry":      "retry the failed items with --retry",
	"verify":     "verify the staged apps",
	"copy_data":  "copy volumes and databases, then compare the counts",
	"route":      "enable routing and switch DNS, one app at a time",
	"safe_check": "run the safe-to-delete check with --retire --probe",
	"none":       "nothing selected yet",
}

func printMigrationJob(w io.Writer, prog string, job apiclient.MigrationJob) {
	_, _ = fmt.Fprintf(w, "Job %s from %s (%s): %s, %d of %d done, %d failed\n\n", job.ID, job.SourceURL, job.Platform, job.Phase, job.Done, job.Total, job.Failed)
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ITEM\tSTATE\tVOLUMES LEFT\tNOTE")
	for _, it := range job.Items {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%s\n", it.Name, it.State, it.VolumesPending, it.Reason)
	}
	_ = tw.Flush()
	for _, d := range job.Databases {
		status := d.Status
		if status == "" {
			status = "not copied"
		}
		_, _ = fmt.Fprintf(w, "database %s: %s\n", d.Name, status)
	}
	if len(job.Next) > 0 {
		_, _ = fmt.Fprintln(w, "\nNext:")
		for _, n := range job.Next {
			text, ok := nextWords[n]
			if !ok {
				text = n
			}
			_, _ = fmt.Fprintf(w, "  - %s\n", text)
		}
	}
	_, _ = fmt.Fprintf(w, "\nFollow with: %s migrate --resume %s --follow\n", prog, job.ID)
}
