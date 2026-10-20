package deploy

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// JobStore persists a service's spec-declared scheduled jobs. *store.DB
// satisfies it structurally.
type JobStore interface {
	SyncSpecScheduledTasks(ctx context.Context, serviceName string, jobs []store.ScheduledTask, now time.Time, newID func() (string, error)) error
}

// WithJobStore makes a deploy keep the service's spec-declared jobs in step
// with its jobs: list. A spec with no jobs: key leaves jobs created in the
// dashboard or CLI alone.
func WithJobStore(s JobStore) Option {
	return func(p *Pipeline) { p.jobs = s }
}

func (p *Pipeline) saveJobs(ctx context.Context, req Request) error {
	if p.jobs == nil || req.Service.Jobs == nil {
		return nil
	}
	tasks := make([]store.ScheduledTask, 0, len(req.Service.Jobs))
	for _, j := range req.Service.Jobs {
		task, err := toScheduledTask(j)
		if err != nil {
			return fmt.Errorf("deploy: service %q: job %q: %w", req.ServiceName, j.Name, err)
		}
		tasks = append(tasks, task)
	}
	if err := p.jobs.SyncSpecScheduledTasks(ctx, req.ServiceName, tasks, time.Now().UTC(), newJobID); err != nil {
		return fmt.Errorf("deploy: service %q: save jobs: %w", req.ServiceName, err)
	}
	return nil
}

// toScheduledTask translates one spec job into its storage shape. The
// durations and memory were already validated by spec.Validate.
func toScheduledTask(j spec.Job) (store.ScheduledTask, error) {
	t := store.ScheduledTask{
		Name: j.Name, Command: j.Command, Schedule: j.Schedule, Timezone: j.Timezone,
		Mode: store.ScheduledTaskModeFresh, Enabled: j.Enabled == nil || *j.Enabled,
		ConcurrencyPolicy: store.ScheduledTaskConcurrencyForbid, MaxRetries: j.Retries,
		CatchUp: store.ScheduledTaskCatchUpSkip,
	}
	if j.Mode == spec.JobModeExec {
		t.Mode = store.ScheduledTaskModeExec
	}
	if j.Concurrency != "" {
		t.ConcurrencyPolicy = j.Concurrency
	}
	if j.CatchUp == spec.JobCatchUpRunOnce {
		t.CatchUp = store.ScheduledTaskCatchUpRunOnce
	}
	for dst, raw := range map[*int]string{
		&t.TimeoutSeconds: j.Timeout, &t.RetryBackoffSeconds: j.RetryBackoff,
		&t.CatchUpLookbackSeconds: j.CatchUpLookback, &t.WarnAfterSeconds: j.WarnAfter,
	} {
		if raw == "" {
			continue
		}
		d, err := time.ParseDuration(raw)
		if err != nil {
			return store.ScheduledTask{}, fmt.Errorf("duration %q: %w", raw, err)
		}
		*dst = int(d.Seconds())
	}
	if j.Memory != "" {
		b, err := parseMemoryBytes(strings.TrimSpace(j.Memory))
		if err != nil {
			return store.ScheduledTask{}, err
		}
		t.MemoryLimitMB = int(b >> 20)
	}
	t.CPULimitMillis = int(j.CPU * 1000)
	return t, nil
}

func newJobID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate job id: %w", err)
	}
	return "sct_" + base64.RawURLEncoding.EncodeToString(buf), nil
}
