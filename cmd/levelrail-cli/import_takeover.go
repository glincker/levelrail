package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const (
	takeoverPollEvery   = 3 * time.Second
	takeoverConfirmWord = "takeover"
)

type importTakeoverFlags struct {
	runID, apiURL, profile, confirm, reason  string
	dryRun, acceptUnmanaged, noWait, jsonOut bool
	wait                                     time.Duration
}

// runImportTakeover implements "import takeover": plan, run, status and
// rollback of this server's ports 80 and 443 moving from the old proxy to the
// built-in ingress.
func runImportTakeover(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		_, _ = fmt.Fprint(stdout, importTakeoverUsage(prog))
		if len(args) == 0 {
			return exitUsage
		}
		return exitOK
	}
	sub := args[0]
	var f importTakeoverFlags
	fs := flag.NewFlagSet(prog+" import takeover "+sub, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, importTakeoverUsage(prog)) }
	fs.StringVar(&f.runID, "run", "", "takeover run id (default: the latest run)")
	fs.StringVar(&f.confirm, "confirm", "", "type "+takeoverConfirmWord+" to confirm run")
	fs.StringVar(&f.reason, "reason", "", "why a rollback is requested, kept in the audit log")
	fs.BoolVar(&f.dryRun, "dry-run", false, "print the plan and change nothing")
	fs.BoolVar(&f.acceptUnmanaged, "accept-unmanaged", false, "go ahead although the old proxy serves domains this server does not manage")
	fs.BoolVar(&f.noWait, "no-wait", false, "return once the run is started")
	fs.DurationVar(&f.wait, "wait", 15*time.Minute, "how long run waits for the takeover to settle")
	fs.StringVar(&f.apiURL, "api-url", "", "control plane API base URL")
	fs.StringVar(&f.profile, "profile", "", "named credentials profile for the control plane")
	fs.BoolVar(&f.jsonOut, "json", false, "print JSON")
	if err := fs.Parse(args[1:]); err != nil {
		return exitUsage
	}
	client := apiClientFromFlags(prog, f.apiURL, "", f.profile, lookupEnv)
	ctx := context.Background()
	switch sub {
	case "plan":
		return takeoverPlanCmd(ctx, client, f, stdout, stderr)
	case "run":
		return takeoverRunCmd(ctx, client, f, stdout, stderr)
	case "status":
		return takeoverStatusCmd(ctx, client, f, stdout, stderr)
	case "rollback":
		return takeoverRollbackCmd(ctx, client, f, stdout, stderr)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown takeover command %q\n", prog, sub)
	_, _ = fmt.Fprint(stderr, importTakeoverUsage(prog))
	return exitUsage
}

func takeoverPlanCmd(ctx context.Context, c *apiclient.Client, f importTakeoverFlags, stdout, stderr io.Writer) int {
	st, err := c.TakeoverStatus(ctx, f.acceptUnmanaged)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("takeover plan: %w", err))
	}
	if f.jsonOut {
		return emitJSON(stdout, stderr, st)
	}
	printTakeoverPlan(stdout, st.Plan)
	if st.Plan.Verdict == "blocked" {
		return exitValidation
	}
	return exitOK
}

func takeoverRunCmd(ctx context.Context, c *apiclient.Client, f importTakeoverFlags, stdout, stderr io.Writer) int {
	if f.dryRun {
		res, err := c.StartTakeover(ctx, apiclient.TakeoverStart{AcceptUnmanaged: f.acceptUnmanaged})
		if err != nil {
			return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("takeover dry run: %w", err))
		}
		if f.jsonOut {
			return emitJSON(stdout, stderr, res)
		}
		if res.Plan != nil {
			printTakeoverPlan(stdout, *res.Plan)
			_, _ = fmt.Fprintln(stdout, "\nDry run: nothing was changed.")
		}
		return exitOK
	}
	if strings.TrimSpace(f.confirm) != takeoverConfirmWord {
		_, _ = fmt.Fprintf(stderr, "takeover: this stops the old proxy and restarts the control plane. Pass --confirm %s to go ahead, or use --dry-run to see the plan.\n", takeoverConfirmWord)
		return exitUsage
	}
	res, err := c.StartTakeover(ctx, apiclient.TakeoverStart{Confirm: true, AcceptUnmanaged: f.acceptUnmanaged})
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("takeover run: %w", err))
	}
	if res.Run == nil {
		return reportError(stdout, stderr, f.jsonOut, errors.New("takeover run: the control plane returned no run"))
	}
	if f.noWait {
		return printTakeoverRun(stdout, stderr, f.jsonOut, *res.Run)
	}
	return waitTakeover(ctx, c, f, *res.Run, stdout, stderr)
}

// waitTakeover polls through the control plane's own restart: errors while it
// is down are expected and retried until the deadline.
func waitTakeover(ctx context.Context, c *apiclient.Client, f importTakeoverFlags, run apiclient.TakeoverRun, stdout, stderr io.Writer) int {
	deadline := time.Now().Add(f.wait)
	seen := 0
	for !run.Settled() {
		if time.Now().After(deadline) {
			_, _ = fmt.Fprintf(stderr, "takeover: still %s after %s, follow it with: import takeover status --run %s\n", run.State, f.wait, run.ID)
			return exitValidation
		}
		time.Sleep(takeoverPollEvery)
		next, err := c.TakeoverRun(ctx, run.ID)
		if err != nil {
			continue
		}
		run = next
		if !f.jsonOut {
			for ; seen < len(run.Steps); seen++ {
				s := run.Steps[seen]
				_, _ = fmt.Fprintf(stderr, "  %-18s %-8s %s\n", s.ID, s.State, s.Detail)
			}
		}
	}
	return printTakeoverRun(stdout, stderr, f.jsonOut, run)
}

func takeoverTarget(ctx context.Context, c *apiclient.Client, f importTakeoverFlags) (apiclient.TakeoverRun, error) {
	if f.runID != "" {
		return c.TakeoverRun(ctx, f.runID)
	}
	runs, err := c.TakeoverRuns(ctx)
	if err != nil {
		return apiclient.TakeoverRun{}, err
	}
	if len(runs) == 0 {
		return apiclient.TakeoverRun{}, errors.New("no takeover run exists yet")
	}
	return runs[0], nil
}

func takeoverStatusCmd(ctx context.Context, c *apiclient.Client, f importTakeoverFlags, stdout, stderr io.Writer) int {
	run, err := takeoverTarget(ctx, c, f)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("takeover status: %w", err))
	}
	return printTakeoverRun(stdout, stderr, f.jsonOut, run)
}

func takeoverRollbackCmd(ctx context.Context, c *apiclient.Client, f importTakeoverFlags, stdout, stderr io.Writer) int {
	run, err := takeoverTarget(ctx, c, f)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("takeover rollback: %w", err))
	}
	res, err := c.RollbackTakeover(ctx, run.ID, f.reason)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("takeover rollback: %w", err))
	}
	if res.Run == nil || f.noWait {
		if res.Run == nil {
			return exitOK
		}
		return printTakeoverRun(stdout, stderr, f.jsonOut, *res.Run)
	}
	return waitTakeover(ctx, c, f, *res.Run, stdout, stderr)
}

func printTakeoverPlan(w io.Writer, p apiclient.TakeoverPlan) {
	_, _ = fmt.Fprintf(w, "Port takeover: %s (listening on %s and %s now, %s and %s after)\n\n", p.Verdict, p.CurrentHTTP, p.CurrentHTTPS, p.TargetHTTP, p.TargetHTTPS)
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "STATUS\tCHECK\tDETAIL")
	for _, c := range p.Checks {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", c.Status, c.Title, c.Detail)
	}
	_ = tw.Flush()
	for _, c := range p.Checks {
		if c.Status == "pass" {
			continue
		}
		for _, item := range c.Items {
			_, _ = fmt.Fprintf(w, "\n%s: %s", c.Title, item)
		}
		if c.Fix != nil {
			_, _ = fmt.Fprintf(w, "\nFix (%s): %s", c.Title, c.Fix.Summary)
			if c.Fix.Action != nil && c.Fix.Action.Kind == "copy" {
				_, _ = fmt.Fprintf(w, "\n  %s", c.Fix.Action.Value)
			}
		}
	}
	_, _ = fmt.Fprintln(w, "\n\nSteps:")
	for i, s := range p.Steps {
		_, _ = fmt.Fprintf(w, "  %d. %s: %s\n", i+1, s.Title, s.Detail)
	}
	_, _ = fmt.Fprintln(w, "\nRollback:")
	for _, line := range p.Rollback {
		_, _ = fmt.Fprintf(w, "  - %s\n", line)
	}
}

func printTakeoverRun(stdout, stderr io.Writer, jsonOut bool, r apiclient.TakeoverRun) int {
	if jsonOut {
		code := emitJSON(stdout, stderr, r)
		if code == exitOK && r.State == "failed" {
			return exitValidation
		}
		return code
	}
	_, _ = fmt.Fprintf(stdout, "Takeover %s: %s\n", r.ID, r.State)
	if r.Error != "" {
		_, _ = fmt.Fprintf(stdout, "  %s\n", r.Error)
	}
	tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "STEP\tSTATE\tDETAIL")
	for _, s := range r.Steps {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", s.ID, s.State, s.Detail)
	}
	_ = tw.Flush()
	for _, d := range r.Verify {
		state := "ok"
		if !d.OK {
			state = "failing: " + d.Detail
		}
		_, _ = fmt.Fprintf(stdout, "  %s %s\n", d.Domain, state)
	}
	switch r.State {
	case "done":
		_, _ = fmt.Fprintln(stdout, "\nThis server now serves ports 80 and 443. Undo any time with: import takeover rollback")
	case "failed":
		return exitValidation
	}
	return exitOK
}

func importTakeoverUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s import takeover plan     [--accept-unmanaged] [--json]
  %[1]s import takeover run      --dry-run [--accept-unmanaged] [--json]
  %[1]s import takeover run      --confirm takeover [--accept-unmanaged] [--no-wait] [--wait DURATION] [--json]
  %[1]s import takeover status   [--run ID] [--json]
  %[1]s import takeover rollback [--run ID] [--reason TEXT] [--no-wait] [--wait DURATION] [--json]

Moves this server's ports 80 and 443 from the proxy of the platform you are
migrating off (Coolify's Traefik, Caddy or nginx) to the built-in ingress.
plan prints the preflight (who holds the ports, apps healthy, DNS, certificates,
headroom) and the exact steps, and changes nothing; run --dry-run is the same.
run --confirm takeover records the way back, stops the old proxy container
(stopped, never removed), lets this server terminate TLS, stores the new listen
addresses, restarts the control plane and verifies every domain over HTTPS. If
verification does not succeed within APP_TAKEOVER_WINDOW (default 5m) it rolls
back by itself. rollback restores the previous addresses and TLS settings and
starts the old proxy again. Root only. Common flags: --api-url, --profile.
`, prog)
}
