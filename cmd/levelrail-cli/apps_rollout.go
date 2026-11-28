package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/glincker/levelrail/internal/apiclient"
	"github.com/glincker/levelrail/internal/placement"
)

const (
	rolloutTimeoutEnv     = "LEVELRAIL_ROLLOUT_BATCH_TIMEOUT"
	rolloutTimeoutDefault = "5m"
)

// rolloutPollEvery is a var so tests can poll fast.
var rolloutPollEvery = 2 * time.Second

// rolloutTarget is one app on one matched node.
type rolloutTarget struct {
	Node string `json:"node"`
	App  string `json:"app"`
	// Offline nodes get a queued change (pending attempt), so there is nothing to wait for.
	Offline bool `json:"offline,omitempty"`
}

// rolloutResult is the per-node outcome reported to the operator.
type rolloutResult struct {
	Node   string `json:"node"`
	App    string `json:"app"`
	Batch  int    `json:"batch"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// planRollout picks the apps named app (or app-*) that sit on a node matching
// sel and splits them into batches of at most maxUnavailable, ordered by node name.
func planRollout(sets []nodeLabelSet, apps []apiclient.AppResource, app string, sel placement.Selector, maxUnavailable int) [][]rolloutTarget {
	if maxUnavailable < 1 {
		maxUnavailable = 1
	}
	var targets []rolloutTarget
	for _, n := range selectGroupNodes(sets, sel) {
		for _, a := range apps {
			if a.NodeID == n.Node.ID && (a.Name == app || strings.HasPrefix(a.Name, app+"-")) {
				targets = append(targets, rolloutTarget{Node: n.Node.Name, App: a.Name, Offline: n.Node.Status == "offline"})
			}
		}
	}
	var batches [][]rolloutTarget
	for i := 0; i < len(targets); i += maxUnavailable {
		batches = append(batches, targets[i:min(i+maxUnavailable, len(targets))])
	}
	return batches
}

// waitConverged polls until the app serves image with no pending rollout.
func waitConverged(ctx context.Context, client *apiclient.Client, name, image string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		a, err := client.GetApp(ctx, name)
		if err != nil {
			return err
		}
		if a.Image == image && a.PendingImage == "" {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("not serving %s after %s", image, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(rolloutPollEvery):
		}
	}
}

// runAppsRollout implements "apps rollout <app> --image I --group k=v": the
// ordinary deploy call, issued batch by batch to apps on matching nodes.
func runAppsRollout(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps rollout", "print per-node results as JSON", stderr)
	var image, group string
	var maxUnavailable int
	var dryRun bool
	fs.StringVar(&image, "image", "", "image to roll out (required)")
	fs.StringVar(&group, "group", "", "node label selector, key=value[,key=value] (required)")
	fs.IntVar(&maxUnavailable, "max-unavailable", 1, "nodes updated at once")
	fs.BoolVar(&dryRun, "dry-run", false, "print the batches without deploying")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps rollout <app> --image I --group key=value [--max-unavailable N] [--dry-run]\n\nDeploys I to every app named <app> or <app>-* that runs on a node matching the label selector, one batch at a time. A failed batch stops the rollout. Batch wait: %s (default %s).\n", prog, rolloutTimeoutEnv, rolloutTimeoutDefault)
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	app, ok := requireOneArg(fs, stderr, prog, "apps rollout", "app name")
	if !ok {
		return exitUsage
	}
	if image == "" || group == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--image and --group are required"))
	}
	sel, err := parseGroupSelector(group)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	raw, _ := lookupEnv(rolloutTimeoutEnv)
	if raw == "" {
		raw = rolloutTimeoutDefault
	}
	timeout, err := time.ParseDuration(raw)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, newValidationError("%s: %v", rolloutTimeoutEnv, err))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	sets, err := loadNodeLabelSets(ctx, client)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	apps, err := client.ListApps(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list apps: %w", err))
	}
	batches := planRollout(sets, apps, app, sel, maxUnavailable)
	if len(batches) == 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("no app %q runs on a node matching %s", app, sel.String()))
	}

	var results []rolloutResult
	failed := false
	for i, batch := range batches {
		for _, t := range batch {
			r := rolloutResult{Node: t.Node, App: t.App, Batch: i + 1, Status: "planned"}
			switch {
			case dryRun:
			case failed:
				r.Status = "skipped"
			default:
				_, derr := client.DeployAppWith(ctx, t.App, apiclient.DeployTriggerRequest{Image: image, QueueWhileOffline: true})
				if derr != nil {
					r.Status, r.Detail, failed = "failed", derr.Error(), true
				} else if t.Offline {
					r.Status = "queued"
				} else {
					r.Status = "deploying"
				}
			}
			results = append(results, r)
		}
		if dryRun || failed {
			continue
		}
		// Wait for the whole batch before touching the next one.
		for j := range batch {
			r := &results[len(results)-len(batch)+j]
			if r.Status != "deploying" {
				continue
			}
			if werr := waitConverged(ctx, client, r.App, image, timeout); werr != nil {
				r.Status, r.Detail, failed = "failed", werr.Error(), true
			} else {
				r.Status = "ok"
			}
		}
	}
	if err := renderResult(stdout, of.Format, of.Query, results, func() {
		for _, r := range results {
			_, _ = fmt.Fprintf(stdout, "batch %d\t%s\t%s\t%s\t%s\n", r.Batch, r.Node, r.App, r.Status, r.Detail)
		}
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	if failed {
		return exitAPIError
	}
	return exitOK
}
