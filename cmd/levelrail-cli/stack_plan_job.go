package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// Poll interval grows from min to max so a long cloud plan is not hammered. Vars so tests can shrink them.
var (
	planPollFirst = time.Second
	planPollCeil  = 5 * time.Second
	planPollSleep = func(ctx context.Context, d time.Duration) error {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			return nil
		}
	}
)

const defaultPlanWait = 30 * time.Minute

// stackJobFlags are the flags of the calls that may start an async cloud plan.
type stackJobFlags struct {
	waitTimeout *time.Duration
	supersede   *bool
}

func bindStackJobFlags(fs *flag.FlagSet) *stackJobFlags {
	return &stackJobFlags{
		waitTimeout: fs.Duration("wait-timeout", defaultPlanWait, "how long to wait for an async cloud plan"),
		supersede:   fs.Bool("supersede", false, "retire a revision awaiting approval that this plan would replace"),
	}
}

const stackJobFlagsHelp = `
  --wait-timeout D         how long to wait for a cloud plan job (default 30m)
  --supersede              retire a pending approval revision this plan replaces
`

var errPlanJobDetached = errors.New("plan job left running")

// planJobError is a failed or expired job, carrying the stable server code.
type planJobError struct{ code, msg string }

func (e *planJobError) Error() string {
	if e.code == "" {
		return e.msg
	}
	return e.code + ": " + e.msg
}

// planStackWait plans and, on a 202, polls the job. Ctrl-C or the timeout leaves the job running.
func planStackWait(ctx context.Context, c cliCall, name string, req apiclient.StackRequest, jf *stackJobFlags) (apiclient.StackPlanResponse, json.RawMessage, error) {
	plan, job, raw, err := c.client.PlanStackAsync(ctx, name, req, *jf.supersede)
	if err != nil || job == nil {
		return plan, raw, err
	}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *jf.waitTimeout)
	defer cancel()
	if !c.jsonOut {
		_, _ = fmt.Fprintf(c.stderr, "cloud plan job %s started\n", job.ID)
	}
	wait := planPollFirst
	for {
		switch job.Status {
		case "done":
			if job.Result == nil {
				return plan, nil, &planJobError{msg: "the plan job finished without a plan"}
			}
			out, _ := json.Marshal(job.Result)
			return *job.Result, out, nil
		case "failed", "expired":
			msg := job.Error
			if msg == "" {
				msg = "the plan job " + job.Status
			}
			return plan, nil, &planJobError{code: job.Code, msg: msg}
		}
		if err := planPollSleep(ctx, wait); err != nil {
			_, _ = fmt.Fprintf(c.stderr, "stopped waiting; job %s keeps running. Poll it with GET /api/v1/stacks/%s/plan-jobs/%s\n", job.ID, name, job.ID)
			return plan, nil, errPlanJobDetached
		}
		if wait *= 2; wait > planPollCeil {
			wait = planPollCeil
		}
		// A poll must outlive the wait context only for the final error; plain ctx is enough here.
		next, _, perr := c.client.StackPlanJobStatus(ctx, name, job.ID)
		if perr != nil {
			if ctx.Err() != nil {
				_, _ = fmt.Fprintf(c.stderr, "stopped waiting; job %s keeps running. Poll it with GET /api/v1/stacks/%s/plan-jobs/%s\n", job.ID, name, job.ID)
				return plan, nil, errPlanJobDetached
			}
			return plan, nil, perr
		}
		job = &next
	}
}
