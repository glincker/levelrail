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

const cutoverPollEvery = 2 * time.Second

type importCutoverFlags struct {
	session, app, runID, apiURL, profile, confirm string
	dryRun, acceptWarnings, noWait, jsonOut       bool
	wait                                          time.Duration
}

// runImportCutover implements "import cutover": plan, run, status and
// rollback of one staged app's move off the source platform.
func runImportCutover(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		_, _ = fmt.Fprint(stdout, importCutoverUsage(prog))
		if len(args) == 0 {
			return exitUsage
		}
		return exitOK
	}
	sub := args[0]
	var f importCutoverFlags
	fs := flag.NewFlagSet(prog+" import cutover "+sub, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, importCutoverUsage(prog)) }
	fs.StringVar(&f.session, "session", "", "import session id")
	fs.StringVar(&f.app, "app", "", "the staged app, by name or source id")
	fs.StringVar(&f.runID, "run", "", "cutover run id (default: the latest run)")
	fs.StringVar(&f.confirm, "confirm", "", "type the app name to confirm a switch")
	fs.BoolVar(&f.dryRun, "dry-run", false, "exercise everything except the switch")
	fs.BoolVar(&f.acceptWarnings, "accept-warnings", false, "go ahead although the plan has warnings")
	fs.BoolVar(&f.noWait, "no-wait", false, "return once the run is started")
	fs.DurationVar(&f.wait, "wait", 20*time.Minute, "how long run waits for the run to settle")
	fs.StringVar(&f.apiURL, "api-url", "", "control plane API base URL")
	fs.StringVar(&f.profile, "profile", "", "named credentials profile for the control plane")
	fs.BoolVar(&f.jsonOut, "json", false, "print JSON")
	if err := fs.Parse(args[1:]); err != nil {
		return exitUsage
	}
	if f.session == "" || f.app == "" {
		_, _ = fmt.Fprintf(stderr, "%s: --session and --app are required\n", prog)
		return exitUsage
	}
	client := apiClientFromFlags(prog, f.apiURL, "", f.profile, lookupEnv)
	ctx := context.Background()
	switch sub {
	case "plan":
		return cutoverPlanCmd(ctx, client, f, stdout, stderr)
	case "run":
		return cutoverRunCmd(ctx, client, f, stdout, stderr)
	case "status":
		return cutoverStatusCmd(ctx, client, f, stdout, stderr)
	case "rollback":
		return cutoverRollbackCmd(ctx, client, f, stdout, stderr)
	case "confirm-dns":
		return cutoverConfirmCmd(ctx, client, f, stdout, stderr)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown cutover command %q\n", prog, sub)
	_, _ = fmt.Fprint(stderr, importCutoverUsage(prog))
	return exitUsage
}

func cutoverPlanCmd(ctx context.Context, c *apiclient.Client, f importCutoverFlags, stdout, stderr io.Writer) int {
	plan, err := c.AppImportCutoverPlan(ctx, f.session, f.app)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("cutover plan: %w", err))
	}
	if f.jsonOut {
		return emitJSON(stdout, stderr, plan)
	}
	printCutoverPlan(stdout, plan)
	if plan.Verdict == "blocked" {
		return exitValidation
	}
	return exitOK
}

func cutoverRunCmd(ctx context.Context, c *apiclient.Client, f importCutoverFlags, stdout, stderr io.Writer) int {
	body := apiclient.ImportCutoverStart{Mode: "switch", Confirm: f.confirm, AcceptWarnings: f.acceptWarnings}
	if f.dryRun {
		body.Mode = "dry_run"
	} else if strings.TrimSpace(f.confirm) == "" {
		_, _ = fmt.Fprintf(stderr, "cutover: a switch needs --confirm %s (the app name), or use --dry-run\n", f.app)
		return exitUsage
	}
	run, err := c.StartAppImportCutover(ctx, f.session, f.app, body)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("start cutover: %w", err))
	}
	if f.noWait {
		return printCutoverRun(stdout, stderr, f.jsonOut, run)
	}
	deadline := time.Now().Add(f.wait)
	lastStep := 0
	for !run.Settled() {
		if time.Now().After(deadline) {
			_, _ = fmt.Fprintf(stderr, "cutover: still %s after %s, follow it with: import cutover status --session %s --app %s --run %s\n", run.State, f.wait, f.session, f.app, run.ID)
			return exitValidation
		}
		time.Sleep(cutoverPollEvery)
		next, err := c.AppImportCutoverRun(ctx, f.session, f.app, run.ID)
		if err != nil {
			return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("poll cutover: %w", err))
		}
		run = next
		if !f.jsonOut {
			for ; lastStep < len(run.Steps); lastStep++ {
				s := run.Steps[lastStep]
				_, _ = fmt.Fprintf(stderr, "  %-14s %-8s %s\n", s.Name, s.State, s.Detail)
			}
		}
	}
	return printCutoverRun(stdout, stderr, f.jsonOut, run)
}

func latestRun(ctx context.Context, c *apiclient.Client, f importCutoverFlags) (apiclient.ImportCutoverRun, error) {
	if f.runID != "" {
		return c.AppImportCutoverRun(ctx, f.session, f.app, f.runID)
	}
	runs, err := c.AppImportCutoverRuns(ctx, f.session, f.app)
	if err != nil {
		return apiclient.ImportCutoverRun{}, err
	}
	if len(runs) == 0 {
		return apiclient.ImportCutoverRun{}, errors.New("no cutover run exists for this app yet")
	}
	return runs[0], nil
}

func cutoverStatusCmd(ctx context.Context, c *apiclient.Client, f importCutoverFlags, stdout, stderr io.Writer) int {
	run, err := latestRun(ctx, c, f)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("cutover status: %w", err))
	}
	return printCutoverRun(stdout, stderr, f.jsonOut, run)
}

func cutoverRollbackCmd(ctx context.Context, c *apiclient.Client, f importCutoverFlags, stdout, stderr io.Writer) int {
	run, err := latestRun(ctx, c, f)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("cutover rollback: %w", err))
	}
	back, err := c.RollbackAppImportCutover(ctx, f.session, f.app, run.ID)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("cutover rollback: %w", err))
	}
	return printCutoverRun(stdout, stderr, f.jsonOut, back)
}

func cutoverConfirmCmd(ctx context.Context, c *apiclient.Client, f importCutoverFlags, stdout, stderr io.Writer) int {
	run, err := latestRun(ctx, c, f)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("cutover confirm-dns: %w", err))
	}
	if _, err := c.ConfirmAppImportCutoverDNS(ctx, f.session, f.app, run.ID); err != nil {
		return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("cutover confirm-dns: %w", err))
	}
	f.runID = run.ID
	f.wait = 5 * time.Minute
	for deadline := time.Now().Add(f.wait); time.Now().Before(deadline); {
		time.Sleep(cutoverPollEvery)
		cur, err := c.AppImportCutoverRun(ctx, f.session, f.app, run.ID)
		if err != nil {
			return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("poll cutover: %w", err))
		}
		if cur.Settled() {
			return printCutoverRun(stdout, stderr, f.jsonOut, cur)
		}
	}
	_, _ = fmt.Fprintln(stderr, "cutover: still verifying, check it with: import cutover status")
	return exitValidation
}

func emitJSON(stdout, stderr io.Writer, v any) int {
	if err := writeJSONValue(stdout, v); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitNetwork
	}
	return exitOK
}

func printCutoverPlan(w io.Writer, p apiclient.ImportCutoverPlan) {
	_, _ = fmt.Fprintf(w, "Cutover plan for %s: %s\n\n", p.App, p.Verdict)
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "STATUS\tCHECK\tDETAIL")
	for _, c := range p.Checks {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", c.Status, c.Title, c.Detail)
	}
	_ = tw.Flush()
	for _, c := range p.Checks {
		if c.Fix != nil && c.Status != "pass" {
			_, _ = fmt.Fprintf(w, "\n%s: %s\n", c.Title, c.Fix.Summary)
		}
	}
	if len(p.Domains) > 0 {
		_, _ = fmt.Fprintln(w, "\nDomains:")
		for _, d := range p.Domains {
			_, _ = fmt.Fprintf(w, "  %s via %s", d.Domain, d.Method)
			if len(d.Current) > 0 {
				_, _ = fmt.Fprintf(w, ", now %s", strings.Join(d.Current, ", "))
			}
			if d.Desired != nil {
				_, _ = fmt.Fprintf(w, ", target %s %s", d.Desired.Type, d.Desired.Value)
			}
			_, _ = fmt.Fprintln(w)
		}
	}
}

func printCutoverRun(stdout, stderr io.Writer, jsonOut bool, r apiclient.ImportCutoverRun) int {
	if jsonOut {
		code := emitJSON(stdout, stderr, r)
		if code == exitOK && r.State == "failed" {
			return exitValidation
		}
		return code
	}
	_, _ = fmt.Fprintf(stdout, "Run %s (%s): %s\n", r.ID, r.Mode, r.State)
	if r.Error != "" {
		_, _ = fmt.Fprintf(stdout, "  %s\n", r.Error)
	}
	tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "STEP\tSTATE\tTIME\tDETAIL")
	for _, s := range r.Steps {
		name := s.Name
		if s.Domain != "" {
			name += " " + s.Domain
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%dms\t%s\n", name, s.State, s.DurationMS, s.Detail)
	}
	_ = tw.Flush()
	for _, d := range r.Domains {
		if d.Manual != nil && r.Awaiting != "" {
			_, _ = fmt.Fprintf(stdout, "\nSet this record by hand, then run confirm-dns:\n  %s %s -> %s\n", d.Manual.Type, d.Domain, d.Manual.Value)
		}
		for _, p := range d.Previous {
			_, _ = fmt.Fprintf(stdout, "\nPrevious value of %s (kept for rollback): %s %s\n", d.Domain, p.Type, p.Value)
		}
	}
	switch r.State {
	case "live":
		_, _ = fmt.Fprintln(stdout, "\nTraffic is switched and verified. Roll back any time with: import cutover rollback")
	case "failed":
		return exitValidation
	}
	return exitOK
}

func importCutoverUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s import cutover plan     --session ID --app NAME [--json]
  %[1]s import cutover run      --session ID --app NAME --dry-run [--no-wait]
  %[1]s import cutover run      --session ID --app NAME --confirm NAME [--accept-warnings] [--no-wait]
  %[1]s import cutover status   --session ID --app NAME [--run ID] [--json]
  %[1]s import cutover rollback --session ID --app NAME [--run ID]
  %[1]s import cutover confirm-dns --session ID --app NAME [--run ID]

A guided, reversible cutover of one staged app. plan prints the readiness
checklist (image, env, database, volumes, health, DNS). run --dry-run starts
the app, waits for it to be healthy and probes it through this node's
ingress, then stops it again without changing any record. run --confirm
does the same, then switches traffic (a DNS record through a connected
provider with the previous value stored, the managed proxy route, or a
record you set by hand), verifies the public name and rolls back on its own
if verification fails. The source platform is never written to.
`, prog)
}
