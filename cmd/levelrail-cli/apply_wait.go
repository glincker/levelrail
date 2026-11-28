package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/glincker/levelrail/internal/apiclient"
	"github.com/glincker/levelrail/internal/iac"
)

const (
	applyWaitTimeoutEnv      = "LEVELRAIL_APPLY_WAIT_TIMEOUT"
	applyWaitIntervalEnv     = "LEVELRAIL_APPLY_WAIT_INTERVAL"
	applyWaitTimeoutDefault  = 5 * time.Minute
	applyWaitIntervalDefault = 3 * time.Second
)

const applyWaitHelp = `
  --wait                   after a successful apply, poll each applied app until it is running or failed
  --ready-timeout D        how long --wait polls (default 5m, env LEVELRAIL_APPLY_WAIT_TIMEOUT)
  --ready-interval D       pause between polls (default 3s, env LEVELRAIL_APPLY_WAIT_INTERVAL)
`

type applyWaitFlags struct {
	on       *bool
	timeout  *time.Duration
	interval *time.Duration
}

func bindApplyWaitFlags(fs *flag.FlagSet) *applyWaitFlags {
	return &applyWaitFlags{
		on:       fs.Bool("wait", false, "poll applied apps until each is running or failed"),
		timeout:  fs.Duration("ready-timeout", 0, "how long --wait polls (default from LEVELRAIL_APPLY_WAIT_TIMEOUT, else 5m)"),
		interval: fs.Duration("ready-interval", 0, "pause between --wait polls (default from LEVELRAIL_APPLY_WAIT_INTERVAL, else 3s)"),
	}
}

// Per app outcomes of --wait.
const (
	waitStateRunning = "running"
	waitStateStopped = "stopped"
	waitStateFailed  = "failed"
	waitStateTimeout = "timeout"
)

type appWaitResult struct {
	App     string `json:"app"`
	State   string `json:"state"`
	Label   string `json:"label,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// waitedApply is the --json shape of an apply that waited.
type waitedApply struct {
	Result iac.ApplyResult `json:"result"`
	Wait   []appWaitResult `json:"wait"`
}

// appsToWaitFor lists the apps an apply wrote; deleted and unchanged ones have nothing to start.
func appsToWaitFor(res iac.ApplyResult) []string {
	var names []string
	seen := map[string]bool{}
	for _, it := range res.Results {
		if it.Kind != iac.KindApp || it.Status != iac.StatusApplied || it.Action == iac.ActionDelete || seen[it.Name] {
			continue
		}
		seen[it.Name] = true
		names = append(names, it.Name)
	}
	return names
}

func waitFailed(rs []appWaitResult) bool {
	for _, r := range rs {
		if r.State == waitStateFailed || r.State == waitStateTimeout {
			return true
		}
	}
	return false
}

// waitForApps polls the app list until every name is running, stopped or failed, or the timeout ends.
func waitForApps(ctx context.Context, c cliCall, wf *applyWaitFlags, lookupEnv func(string) (string, bool), names []string) []appWaitResult {
	if len(names) == 0 {
		return nil
	}
	timeout, interval := *wf.timeout, *wf.interval
	if timeout <= 0 {
		timeout = durationFromEnv(lookupEnv, applyWaitTimeoutEnv, applyWaitTimeoutDefault)
	}
	if interval <= 0 {
		interval = durationFromEnv(lookupEnv, applyWaitIntervalEnv, applyWaitIntervalDefault)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if !c.jsonOut {
		_, _ = fmt.Fprintf(c.stderr, "waiting up to %s for %d app(s)\n", timeout, len(names))
	}
	done := map[string]appWaitResult{}
	last := map[string]appWaitResult{}
	for {
		statuses, err := c.client.ListAppStatuses(ctx)
		if err == nil {
			for _, e := range statuses {
				last[e.Name] = settleApp(e)
			}
			for _, n := range names {
				if r, ok := last[n]; ok && r.State != "" {
					done[n] = r
				}
			}
		}
		if len(done) == len(names) || planPollSleep(ctx, interval) != nil {
			break
		}
	}
	out := make([]appWaitResult, 0, len(names))
	for _, n := range names {
		if r, ok := done[n]; ok {
			out = append(out, r)
			continue
		}
		r := appWaitResult{App: n, State: waitStateTimeout, Label: "not found"}
		if l, ok := last[n]; ok {
			r.Label = l.Label
		}
		out = append(out, r)
	}
	return out
}

// settleApp maps a status row to a final state, or an empty State while the app is still moving.
func settleApp(e apiclient.AppStatusEntry) appWaitResult {
	r := appWaitResult{App: e.Name, Label: e.Status.Label, Reason: e.Status.Reason, Message: e.Status.Message}
	switch {
	case e.Status.Variant == "success":
		r.State = waitStateRunning
	case e.Status.Variant == "destructive":
		r.State = waitStateFailed
	case e.Status.Label == "Stopped":
		r.State = waitStateStopped
	}
	return r
}

func printWaitResults(w io.Writer, rs []appWaitResult) {
	if len(rs) == 0 {
		return
	}
	_, _ = fmt.Fprintln(w, "\nApp status:")
	for _, r := range rs {
		line := fmt.Sprintf("  %-8s %s", r.State, r.App)
		switch r.State {
		case waitStateFailed:
			line += ": " + r.Label
			if r.Reason != "" {
				line += " (" + r.Reason + ")"
			}
			if r.Message != "" {
				line += ": " + r.Message
			}
		case waitStateTimeout:
			line += ": still " + r.Label + " when the wait ended"
		}
		_, _ = fmt.Fprintln(w, line)
	}
}

// finishStackWait renders a stack apply that waits, then maps the outcome to an exit code.
func finishStackWait(ctx context.Context, c cliCall, wf *applyWaitFlags, lookupEnv func(string) (string, bool), res apiclient.StackApplyResponse) int {
	var waited []appWaitResult
	if res.Result != nil && res.Result.OK() && res.Revision != nil && iac.RevisionStatus(res.Revision.Status) == iac.RevisionApplied {
		waited = waitForApps(ctx, c, wf, lookupEnv, appsToWaitFor(*res.Result))
	}
	out := struct {
		Apply apiclient.StackApplyResponse `json:"apply"`
		Wait  []appWaitResult              `json:"wait"`
	}{res, waited}
	rc := c.render(out, func() {
		printStackRun(c.stdout, res, "applied")
		printWaitResults(c.stdout, waited)
	})
	if rc != exitOK {
		return rc
	}
	if rc := stackRunExit(c, res); rc != exitOK {
		return rc
	}
	if waitFailed(waited) {
		return iacExitError
	}
	return exitOK
}
