package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const (
	upgradeWatchInterval = 3 * time.Second
	upgradeWatchTimeout  = 10 * time.Minute
)

type upgradeApplyOptions struct {
	target string
	ack    string
	plan   bool
	watch  bool
}

// runUpgradeApply implements "upgrade --apply": show the breaking changes
// between the running version and the target, refuse until the flagged ones
// are acknowledged, then ask the control plane to run the host upgrade and
// follow the attempt to its outcome.
func runUpgradeApply(ctx context.Context, client *Client, opts upgradeApplyOptions, jsonOut bool, of outputFlags, stdout, stderr io.Writer) int {
	plan, err := client.GetSelfUpgradePlan(ctx, opts.target)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("upgrade plan: %w", err))
	}
	if opts.plan {
		if err := renderResult(stdout, of.Format, of.Query, plan, func() { printUpgradePlan(stdout, plan) }); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitCodeForError(err)
		}
		return exitOK
	}
	if !jsonOut {
		printUpgradePlan(stdout, plan)
	}
	if !plan.CanApply {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("this host cannot apply the upgrade from the API: %s. Run on the host: %s", plan.CannotApplyWhy, plan.Command))
	}
	acked := splitCSV(opts.ack)
	if missing := unackedBreaking(plan.Breaking, acked); len(missing) > 0 {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("breaking changes need acknowledgement: re-run with --ack-breaking %s", strings.Join(missing, ",")))
	}
	if _, err := client.StartSelfUpgrade(ctx, plan.TargetVersion, acked); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("start upgrade: %w", err))
	}
	if !opts.watch {
		if err := renderResult(stdout, of.Format, of.Query, map[string]string{"status": "started", "target_version": plan.TargetVersion}, func() {
			_, _ = fmt.Fprintf(stdout, "upgrade to %s started on the host\n", plan.TargetVersion)
		}); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitCodeForError(err)
		}
		return exitOK
	}
	return watchUpgrade(ctx, client, plan.TargetVersion, jsonOut, of, stdout, stderr)
}

func watchUpgrade(ctx context.Context, client *Client, target string, jsonOut bool, of outputFlags, stdout, stderr io.Writer) int {
	deadline := time.Now().Add(upgradeWatchTimeout)
	var last *apiclient.SelfUpgradeAttempt
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return reportError(stdout, stderr, jsonOut, ctx.Err())
		case <-time.After(upgradeWatchInterval):
		}
		// The control plane restarts mid-upgrade, so errors here are normal.
		list, err := client.ListSelfUpgradeAttempts(ctx)
		if err != nil {
			continue
		}
		for i := range list.Attempts {
			if list.Attempts[i].ToVersion == target {
				last = &list.Attempts[i]
				break
			}
		}
		if last == nil || last.Outcome == "running" {
			continue
		}
		if err := renderResult(stdout, of.Format, of.Query, *last, func() { printUpgradeAttempt(stdout, *last) }); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitCodeForError(err)
		}
		if last.Outcome == "succeeded" {
			return exitOK
		}
		return exitCheckFailed
	}
	return reportError(stdout, stderr, jsonOut, errors.New("timed out waiting for the upgrade to finish; check upgrade --attempts"))
}

func runUpgradeAttempts(ctx context.Context, client *Client, jsonOut bool, of outputFlags, stdout, stderr io.Writer) int {
	list, err := client.ListSelfUpgradeAttempts(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("upgrade attempts: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, list, func() {
		if len(list.Attempts) == 0 {
			_, _ = fmt.Fprintln(stdout, "no self-upgrade attempts recorded")
		}
		for _, a := range list.Attempts {
			printUpgradeAttempt(stdout, a)
		}
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printUpgradePlan(out io.Writer, p apiclient.SelfUpgradePlan) {
	_, _ = fmt.Fprintf(out, "upgrade %s -> %s\n", p.CurrentVersion, p.TargetVersion)
	if !p.NotesAvailable {
		_, _ = fmt.Fprintln(out, "release notes could not be fetched, breaking changes are unknown")
	}
	for _, b := range p.Breaking {
		mark := "info"
		if b.RequiresAck {
			mark = "ACK REQUIRED"
		}
		_, _ = fmt.Fprintf(out, "breaking [%s] %s (%s): %s\n", mark, b.ID, b.Version, b.Summary)
	}
	if len(p.Breaking) == 0 && p.NotesAvailable {
		_, _ = fmt.Fprintln(out, "no breaking changes declared")
	}
	_, _ = fmt.Fprintf(out, "steps: %s (automatic rollback if the new release is not healthy)\n", strings.Join(p.Steps, ", "))
}

func printUpgradeAttempt(out io.Writer, a apiclient.SelfUpgradeAttempt) {
	_, _ = fmt.Fprintf(out, "%s  %-11s %s -> %s  by %s  %s\n", a.StartedAt, a.Outcome, a.FromVersion, a.ToVersion, a.Initiator, a.ID)
	for _, s := range a.Steps {
		_, _ = fmt.Fprintf(out, "    [%s] %s %s\n", s.Status, s.Name, s.Detail)
	}
	if a.Error != "" {
		_, _ = fmt.Fprintf(out, "    error: %s\n", a.Error)
	}
}

func unackedBreaking(breaking []apiclient.SelfUpgradeBreaking, acked []string) []string {
	have := map[string]bool{}
	for _, id := range acked {
		have[id] = true
	}
	var out []string
	for _, b := range breaking {
		if b.RequiresAck && !have[b.ID] {
			out = append(out, b.ID)
		}
	}
	return out
}
