package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const (
	undoRemove = "remove"
	undoSkip   = "skip"
)

type undoItem struct {
	r      *journalResource
	action string
	reason string
	data   bool
}

type migrateUndoFlags struct {
	journal, targetToken, targetAPIURL, targetProfile string
	dryRun, yes, force, includeData                   bool
	forceApps                                         ackList
}

// runMigrateUndo removes only what the journal records this run created, newest first, through the
// caller's own token and the normal delete routes; it prints the plan and acts only with --yes.
func runMigrateUndo(prog, source string, args []string, stdin io.Reader, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs := flag.NewFlagSet(prog+" migrate "+source+" undo", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var f migrateUndoFlags
	fs.StringVar(&f.journal, "journal", "", "apply journal written by the --apply run to undo (required)")
	fs.BoolVar(&f.dryRun, "dry-run", false, "print the plan only (the default unless --yes is given)")
	fs.BoolVar(&f.yes, "yes", false, "remove the resources the plan marks remove")
	fs.BoolVar(&f.force, "force", false, "also remove apps deployed since the run (never an app whose fingerprint does not match)")
	fs.Var(&f.forceApps, "force-app", "remove this one app even though its fingerprint is missing or does not match (repeatable)")
	fs.BoolVar(&f.includeData, "include-data", false, "also remove apps holding volume data, with their volumes, after a typed confirmation")
	fs.StringVar(&f.targetToken, "target-token", "", "Levelrail API token (overrides "+apiTokenEnvHelp+" and the credentials file)")
	fs.StringVar(&f.targetAPIURL, "target-api-url", "", "Levelrail control plane base URL (overrides "+apiURLEnvHelp+" and the credentials file)")
	fs.StringVar(&f.targetProfile, "target-profile", "", "named credentials profile for the target Levelrail instance")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, migrateUndoUsage(prog, source)) }
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	if f.journal == "" {
		return reportError(stdout, stderr, false, newValidationError("%s", "--journal is required"))
	}
	unlock, err := lockJournal(f.journal)
	if err != nil {
		return reportError(stdout, stderr, false, err)
	}
	defer unlock()
	j, err := loadJournal(f.journal, source, journalMaxBytes(lookupEnv))
	if err != nil {
		return reportError(stdout, stderr, false, err)
	}
	for _, name := range f.forceApps {
		if r := j.find(journalKindApp, name); r == nil || !r.CreatedByRun {
			return reportError(stdout, stderr, false, newValidationError("--force-app %q is not an app this run created", name))
		}
	}
	profile := resolveProfile(f.targetProfile, lookupEnv)
	targetURL := resolveAPIURL(f.targetAPIURL, lookupEnv, prog, profile)
	if j.TargetHost != originOf(targetURL) {
		return reportError(stdout, stderr, false, newValidationError("journal %s was written for %s, not %s: refusing to undo against a different control plane", f.journal, j.TargetHost, originOf(targetURL)))
	}
	client := NewClient(targetURL, resolveToken(f.targetToken, lookupEnv, prog, profile))
	ctx := context.Background()

	plan := planUndo(ctx, client, j, f)
	removes, dataRemoves := printUndoPlan(stdout, plan)
	if !f.yes || f.dryRun {
		_, _ = fmt.Fprintf(stdout, "\ndry run: nothing was removed. Re-run with --yes to remove the %d resource(s) marked remove.\n", removes)
		return exitOK
	}
	if removes == 0 {
		return exitOK
	}
	if dataRemoves > 0 && !confirmDataCount(stdin, stderr, dataRemoves) {
		return reportError(stdout, stderr, false, newValidationError("%s", "the typed count did not match, nothing was removed"))
	}
	return executeUndo(ctx, client, f, j, plan, stdout, stderr)
}

func planUndo(ctx context.Context, client *Client, j *migrateJournal, f migrateUndoFlags) []undoItem {
	plan := make([]undoItem, 0, len(j.Resources))
	for i := len(j.Resources) - 1; i >= 0; i-- {
		r := &j.Resources[i]
		item := undoItem{r: r, action: undoSkip}
		switch {
		case !r.CreatedByRun:
			item.reason = "not created by this run, never touched"
		case r.state(stepUndone) == stateDone:
			item.reason = "already undone"
		default:
			item = planOne(ctx, client, r, f)
		}
		plan = append(plan, item)
	}
	return plan
}

func planOne(ctx context.Context, client *Client, r *journalResource, f migrateUndoFlags) undoItem {
	item := undoItem{r: r, action: undoSkip}
	if r.Kind != journalKindApp {
		item.action, item.reason = planBackupUndo(ctx, client, r)
		return item
	}
	app, err := client.GetApp(ctx, r.Name)
	switch {
	case isAPINotFound(err):
		item.reason = "already gone from the target"
		return item
	case err != nil:
		item.reason = "could not read it: " + err.Error()
		return item
	}
	item.data = appHasData(app) || r.HasData
	named := slices.Contains(f.forceApps, r.Name)
	// A fingerprint mismatch means it may not be the app this run made, so only naming the app overrides it.
	var identity, changed string
	switch {
	case r.Fingerprint == "":
		identity = "the run recorded no fingerprint for it"
	case appFingerprint(app) != r.Fingerprint:
		identity = "changed since the run"
	default:
		list, err := client.ListDeployments(ctx, apiclient.DeploymentListOptions{App: r.Name})
		if err != nil {
			changed = "could not count its deploys: " + err.Error()
		} else if n := len(list.Items); n > r.Deploys {
			changed = fmt.Sprintf("%d deploy(s) since the run", n-r.Deploys)
		}
	}
	if identity != "" {
		changed = identity
	}
	switch {
	case identity != "" && !named:
		item.reason = identity + ", pass --force-app " + r.Name + " to remove this one app anyway"
	case changed != "" && !f.force && !named:
		item.reason = changed + ", pass --force to remove it anyway"
	case item.data && !f.includeData:
		item.reason = "holds volume data, pass --include-data to remove it with its volumes"
	default:
		item.action, item.reason = undoRemove, changed
		if item.data {
			item.reason = strings.TrimPrefix(item.reason+", with its volume data", ", ")
		}
	}
	return item
}

func printUndoPlan(out io.Writer, plan []undoItem) (removes, dataRemoves int) {
	_, _ = fmt.Fprintln(out, "Undo plan (newest first):")
	for _, it := range plan {
		line := fmt.Sprintf("  %-6s %s %s", it.action, it.r.Kind, it.r.Name)
		if it.reason != "" {
			line += ": " + it.reason
		}
		_, _ = fmt.Fprintln(out, line)
		if it.action == undoRemove {
			removes++
			if it.data {
				dataRemoves++
			}
		}
	}
	return removes, dataRemoves
}

func confirmDataCount(stdin io.Reader, stderr io.Writer, n int) bool {
	_, _ = fmt.Fprintf(stderr, "This deletes %d app(s) together with their volume data. Type %d to confirm: ", n, n)
	line, _ := bufio.NewReader(stdin).ReadString('\n')
	return strings.TrimSpace(line) == strconv.Itoa(n)
}

func executeUndo(ctx context.Context, client *Client, f migrateUndoFlags, j *migrateJournal, plan []undoItem, stdout, stderr io.Writer) int {
	path := f.journal
	code := exitOK
	for _, it := range plan {
		if it.action != undoRemove {
			continue
		}
		// Checked again right before the delete: the prompt can sit open while the app changes.
		if again := planOne(ctx, client, it.r, f); again.action != undoRemove || again.data != it.data {
			_, _ = fmt.Fprintf(stderr, "skipped %s %s: it changed after the plan was printed (%s)\n", it.r.Kind, it.r.Name, again.reason)
			continue
		}
		var err error
		if it.r.Kind != journalKindApp {
			err = undoBackupResource(ctx, client, it.r)
		} else if it.data {
			_, err = client.DeleteAppAndData(ctx, it.r.Name)
		} else {
			_, err = client.DeleteAppResult(ctx, it.r.Name)
		}
		state := stateDone
		if err != nil {
			state, code = stateFailed, exitAPIError
			_, _ = fmt.Fprintf(stderr, "remove %s %s: %v\n", it.r.Kind, it.r.Name, err)
		} else {
			_, _ = fmt.Fprintf(stdout, "removed %s %s\n", it.r.Kind, it.r.Name)
		}
		it.r.set(stepUndone, state, time.Now())
		if err := saveJournal(path, j, time.Now()); err != nil {
			_, _ = fmt.Fprintf(stderr, "warning: %v\n", err)
		}
	}
	return code
}

func migrateUndoUsage(prog, source string) string {
	return fmt.Sprintf(`Usage:
  %[1]s migrate %[2]s undo --journal FILE [--dry-run] [--yes] [--force] [--include-data]

Removes only the apps the journal records as created by that --apply run,
newest first, through your own token and the normal delete routes, so
protected environments and delete confirmations still apply. Apps that
existed before the run are never touched. Without --yes it prints the plan
and removes nothing.

Flags:
  --journal string          apply journal written by the --apply run (required)
  --dry-run                 print the plan only (the default unless --yes is given)
  --yes                     remove the apps the plan marks remove
  --force                   also remove apps deployed since the run; never overrides a
                            missing or mismatched fingerprint
  --force-app string        remove this one app even though its fingerprint is missing or
                            does not match (repeatable, must be an app the run created)
  --include-data            also remove apps holding volume data, with their volumes,
                            after you type the number of such apps to confirm
  --target-token string     Levelrail API token (default: %[3]s env var, then the credentials file)
  --target-api-url string   Levelrail control plane base URL (default: %[4]s env var, then the credentials file)
  --target-profile string   named credentials profile for the target Levelrail instance
  -h, --help                show this help

The journal must be the target the run wrote it for, a regular file (not a
symlink, in a folder that is not a symlink) and readable only by you.

Apps have no immutable id in the API, so the fingerprint is their shape: an
app deleted and re-created with the same shape and name looks unchanged.
`, prog, source, apiTokenEnvHelp, apiURLEnvHelp)
}
