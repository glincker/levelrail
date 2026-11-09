package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// applyJournal records one --apply run step by step; a nil *applyJournal records nothing.
type applyJournal struct {
	path   string
	j      *migrateJournal
	resume bool
	now    func() time.Time
	err    error
	unlock func()
}

// openApplyJournal locks the journal, then loads it (resume) or writes a fresh one before anything is created.
func openApplyJournal(prog, source, path, sourceURL, targetURL string, resume bool, lookupEnv func(string) (string, bool)) (*applyJournal, error) {
	now := time.Now
	isDefault := path == ""
	if isDefault {
		if resume {
			return nil, newValidationError("%s", "resume needs --journal, the file the earlier --apply run printed")
		}
		var err error
		if path, err = defaultJournalPath(prog, source, now()); err != nil {
			return nil, err
		}
	}
	if err := prepareJournalDir(path, isDefault); err != nil {
		return nil, err
	}
	unlock, err := lockJournal(path)
	if err != nil {
		return nil, err
	}
	jr := &applyJournal{path: path, resume: resume, now: now, unlock: unlock}
	if resume {
		j, err := loadJournal(path, source, journalMaxBytes(lookupEnv))
		if err != nil {
			unlock()
			return nil, err
		}
		if j.TargetHost != originOf(targetURL) || j.SourceHost != originOf(sourceURL) {
			unlock()
			return nil, newValidationError("journal %s was written for %s from %s, not %s from %s", path, j.TargetHost, j.SourceHost, originOf(targetURL), originOf(sourceURL))
		}
		jr.j = j
		return jr, nil
	}
	if _, err := os.Lstat(path); err == nil || !errors.Is(err, os.ErrNotExist) {
		unlock()
		return nil, newValidationError("journal %s already exists: continue it with resume, or pick a new --journal path", path)
	}
	ts := now().UTC().Format(time.RFC3339)
	jr.j = &migrateJournal{Version: journalVersion, Source: source, SourceHost: originOf(sourceURL), TargetHost: originOf(targetURL), StartedAt: ts, UpdatedAt: ts, Resources: []journalResource{}}
	if err := saveJournal(path, jr.j, now()); err != nil {
		unlock()
		return nil, err
	}
	return jr, nil
}

func (jr *applyJournal) close() {
	if jr != nil && jr.unlock != nil {
		jr.unlock()
	}
}

func (jr *applyJournal) failed() bool { return jr != nil && jr.err != nil }

func (jr *applyJournal) record(r *journalResource, step, state string) {
	if jr == nil {
		return
	}
	r.set(step, state, jr.now())
	if err := saveJournal(jr.path, jr.j, jr.now()); err != nil && jr.err == nil {
		jr.err = err
	}
}

// beginApp decides whether an app runs: a journal-created app continues its open steps; on resume an app
// that exists but was not created by this run is never touched, and neither is one changed since.
func (jr *applyJournal) beginApp(ctx context.Context, client *Client, a *mappedApp) (*journalResource, bool) {
	if jr == nil {
		return &journalResource{Kind: journalKindApp, Name: a.ServiceName}, true
	}
	r := jr.j.find(journalKindApp, a.ServiceName)
	if r != nil && r.CreatedByRun {
		if !jr.resume {
			return r, true
		}
		app, err := client.GetApp(ctx, a.ServiceName)
		switch {
		case err == nil && r.Fingerprint == "":
			a.CreateError = "this run created the app but recorded no fingerprint, so it cannot tell whether the app is still the one it made; left untouched"
			return r, false
		case isAPINotFound(err):
			a.CreateError = "this run created the app but it is gone from the target now, left alone"
			return r, false
		case err != nil:
			a.CreateError = "could not read the app to resume it: " + err.Error()
			return r, false
		case appFingerprint(app) != r.Fingerprint:
			a.CreateError = "the app changed on the target since this run created it, left untouched"
			return r, false
		}
		return r, true
	}
	if jr.resume {
		_, err := client.GetApp(ctx, a.ServiceName)
		switch {
		case err == nil:
			state := stateSkipped
			a.CreateError = "already exists on the target and this run did not create it, left untouched"
			if r != nil && (r.state(stepCreated) == statePending || r.state(stepCreated) == stateUnknown) {
				state = stateUnknown
				a.CreateError = "exists on the target, but the journal cannot confirm this run created it (interrupted mid create), left untouched"
			}
			r = jr.j.ensure(journalKindApp, a.ServiceName)
			jr.record(r, stepCreated, state)
			return r, false
		case !isAPINotFound(err):
			a.CreateError = "could not check whether the app already exists: " + err.Error()
			return nil, false
		}
	}
	r = jr.j.ensure(journalKindApp, a.ServiceName)
	jr.record(r, stepCreated, statePending)
	return r, !jr.failed()
}

func (jr *applyJournal) markCreated(ctx context.Context, client *Client, r *journalResource) {
	r.CreatedByRun = true
	if jr == nil {
		return
	}
	jr.refresh(ctx, client, r)
	jr.record(r, stepCreated, stateDone)
}

// seal stores the app's fingerprint and deploy count once its steps ran, so undo can tell a later change.
func (jr *applyJournal) seal(ctx context.Context, client *Client, r *journalResource) {
	if jr == nil || !r.CreatedByRun {
		return
	}
	app, err := client.GetApp(ctx, r.Name)
	if err != nil {
		return
	}
	r.Fingerprint, r.HasData = appFingerprint(app), appHasData(app)
	if list, err := client.ListDeployments(ctx, apiclient.DeploymentListOptions{App: r.Name}); err == nil {
		r.Deploys = len(list.Items)
	}
	if err := saveJournal(jr.path, jr.j, jr.now()); err != nil && jr.err == nil {
		jr.err = err
	}
}

func (jr *applyJournal) recordWebhooks(apps []mappedApp) {
	if jr == nil {
		return
	}
	for _, a := range apps {
		r := jr.j.find(journalKindApp, a.ServiceName)
		if r == nil || !r.CreatedByRun || a.Webhook == nil {
			continue
		}
		state := stateSkipped
		switch a.Webhook.State {
		case webhookCreated, webhookAlready:
			state = stateDone
		case webhookFailed:
			state = stateFailed
		}
		jr.record(r, stepWebhook, state)
	}
}

// runStep runs one step when the journal says it is still open and records its outcome.
func (jr *applyJournal) runStep(ctx context.Context, client *Client, r *journalResource, step string, fn func() string) {
	if !r.needsRun(step) {
		return
	}
	state := fn()
	if state == stateDone || state == stateReview {
		jr.refresh(ctx, client, r)
	}
	jr.record(r, step, state)
}

// refresh re-reads the app after a state changing step, so a crash at any point leaves a current fingerprint.
func (jr *applyJournal) refresh(ctx context.Context, client *Client, r *journalResource) {
	if jr == nil || !r.CreatedByRun {
		return
	}
	if app, err := client.GetApp(ctx, r.Name); err == nil {
		r.Fingerprint, r.HasData = appFingerprint(app), appHasData(app)
	}
}

func secretsOutcome(a *mappedApp) string {
	switch {
	case len(a.Service.EnvLiteral) == 0:
		return stateSkipped
	case len(a.SecretsFailed) > 0:
		return stateFailed
	}
	return stateDone
}

func issueOutcome(skip bool, before int, a *mappedApp) string {
	switch {
	case skip:
		return stateSkipped
	case len(a.Issues) > before:
		return stateReview
	}
	return stateDone
}

func ingressOutcome(ctx context.Context, client *Client, name string, a *mappedApp) string {
	before := len(a.Issues)
	applyIngress(ctx, client, name, a)
	p := a.Service.Ingress
	return issueOutcome(p == nil || !p.Translated(), before, a)
}

func journalSummary(jr *applyJournal) (created, failed int) {
	if jr == nil {
		return 0, 0
	}
	for _, r := range jr.j.Resources {
		if r.CreatedByRun {
			created++
		}
		if slices.ContainsFunc(r.Steps, func(s journalStep) bool { return s.State == stateFailed }) {
			failed++
		}
	}
	return created, failed
}

func printJournalFooter(w io.Writer, prog, source string, jr *applyJournal) {
	if jr == nil {
		return
	}
	created, failed := journalSummary(jr)
	_, _ = fmt.Fprintf(w, "apply journal: %s (%d created, %d with a failed step)\n", jr.path, created, failed)
	if failed > 0 {
		_, _ = fmt.Fprintf(w, "  continue:  %s migrate %s resume --journal %s <the same flags as this run>\n", prog, source, jr.path)
	}
	if created > 0 {
		_, _ = fmt.Fprintf(w, "  roll back: %s migrate %s undo --journal %s (dry run first)\n", prog, source, jr.path)
	}
	if jr.err != nil {
		_, _ = fmt.Fprintf(w, "  warning: the journal stopped updating (%v); later apps were not attempted\n", jr.err)
	}
}

const journalFlagHelp = "with --apply, the apply journal to write (default under the config dir); resume reads it and continues the open steps"
