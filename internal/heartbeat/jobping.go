package heartbeat

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Environment variables for per-job ping-out retries.
const (
	EnvJobMaxAttempts = "APP_HEARTBEAT_JOB_MAX_ATTEMPTS"
	EnvJobBackoffBase = "APP_HEARTBEAT_JOB_BACKOFF_BASE"
	EnvJobBackoffMax  = "APP_HEARTBEAT_JOB_BACKOFF_MAX"
	EnvJobQueue       = "APP_HEARTBEAT_JOB_QUEUE"

	defaultJobMaxAttempts = 3
	defaultJobBackoffBase = 2 * time.Second
	defaultJobBackoffMax  = 30 * time.Second
	defaultJobQueue       = 64

	jobSecretsService = "heartbeat-jobs"
)

// JobPingConfig is the env-derived configuration of the per-job sender.
type JobPingConfig struct {
	Timeout      time.Duration
	AllowPrivate bool
	MaxAttempts  int
	BackoffBase  time.Duration
	BackoffMax   time.Duration
	Queue        int
}

// JobPingConfigFromEnv reads the shared heartbeat timeout and private switch plus the job retry knobs.
func JobPingConfigFromEnv(lookup func(string) (string, bool)) (JobPingConfig, error) {
	base, err := ConfigFromEnv(lookup)
	if err != nil {
		return JobPingConfig{}, err
	}
	c := JobPingConfig{Timeout: base.Timeout, AllowPrivate: base.AllowPrivate, MaxAttempts: defaultJobMaxAttempts,
		BackoffBase: defaultJobBackoffBase, BackoffMax: defaultJobBackoffMax, Queue: defaultJobQueue}
	get := func(k string) string {
		v, _ := lookup(k)
		return strings.TrimSpace(v)
	}
	if v := get(EnvJobMaxAttempts); v != "" {
		if c.MaxAttempts, err = strconv.Atoi(v); err != nil || c.MaxAttempts < 1 {
			return JobPingConfig{}, fmt.Errorf("%s must be a positive integer", EnvJobMaxAttempts)
		}
	}
	if v := get(EnvJobQueue); v != "" {
		if c.Queue, err = strconv.Atoi(v); err != nil || c.Queue < 1 {
			return JobPingConfig{}, fmt.Errorf("%s must be a positive integer", EnvJobQueue)
		}
	}
	for _, d := range []struct {
		key string
		dst *time.Duration
	}{{EnvJobBackoffBase, &c.BackoffBase}, {EnvJobBackoffMax, &c.BackoffMax}} {
		if v := get(d.key); v != "" {
			if *d.dst, err = time.ParseDuration(v); err != nil || *d.dst <= 0 {
				return JobPingConfig{}, fmt.Errorf("%s must be a positive duration", d.key)
			}
		}
	}
	return c, nil
}

// DeriveFailURL returns the explicit fail URL, else the ping URL with /fail appended to its path and the query kept.
func DeriveFailURL(pingURL, failURL string) string {
	if strings.TrimSpace(failURL) != "" {
		return strings.TrimSpace(failURL)
	}
	u, err := url.Parse(strings.TrimSpace(pingURL))
	if err != nil {
		return ""
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/fail"
	u.RawPath = ""
	return u.String()
}

type jobEvent struct{ taskID, runID, status string }

// JobPinger sends a per-job ping after a scheduled run settles. It never affects the run result.
type JobPinger struct {
	cfg     JobPingConfig
	secrets Secrets
	logger  *slog.Logger
	client  *http.Client
	queue   chan jobEvent
	sleep   func(ctx context.Context, d time.Duration) error
	jitter  func(limit int64) int64
}

// NewJobPinger builds a sender; secrets must be non-nil.
func NewJobPinger(cfg JobPingConfig, secrets Secrets, logger *slog.Logger) *JobPinger {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &JobPinger{cfg: cfg, secrets: secrets, logger: logger, client: NewClient(cfg.AllowPrivate, cfg.Timeout),
		queue: make(chan jobEvent, cfg.Queue), sleep: sleepCtx, jitter: func(m int64) int64 { return rand.Int64N(m + 1) }} //nolint:gosec,forbidigo // backoff jitter, not security
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func jobKey(taskID, kind string) string { return "task_" + taskID + "_" + kind }

// Set stores the ping URL, and the fail URL when given, encrypted. An empty pingURL clears both.
func (p *JobPinger) Set(ctx context.Context, taskID, pingURL, failURL string) error {
	if strings.TrimSpace(pingURL) == "" {
		return p.Clear(ctx, taskID)
	}
	if _, err := ValidateURL(pingURL, p.cfg.AllowPrivate); err != nil {
		return fmt.Errorf("heartbeat_url: %w", err)
	}
	if strings.TrimSpace(failURL) != "" {
		if _, err := ValidateURL(failURL, p.cfg.AllowPrivate); err != nil {
			return fmt.Errorf("heartbeat_fail_url: %w", err)
		}
	}
	if err := p.secrets.SetValue(ctx, jobSecretsService, jobKey(taskID, "url"), strings.TrimSpace(pingURL)); err != nil {
		return err
	}
	if strings.TrimSpace(failURL) == "" {
		return p.secrets.DeleteValue(ctx, jobSecretsService, jobKey(taskID, "fail_url"))
	}
	return p.secrets.SetValue(ctx, jobSecretsService, jobKey(taskID, "fail_url"), strings.TrimSpace(failURL))
}

// Clear removes both URLs of a task.
func (p *JobPinger) Clear(ctx context.Context, taskID string) error {
	for _, k := range []string{"url", "fail_url"} {
		if err := p.secrets.DeleteValue(ctx, jobSecretsService, jobKey(taskID, k)); err != nil {
			return err
		}
	}
	return nil
}

// Host returns the ping URL host, the only part safe to show, and whether one is set.
func (p *JobPinger) Host(ctx context.Context, taskID string) (string, bool) {
	ok, err := p.secrets.Exists(ctx, jobSecretsService, jobKey(taskID, "url"))
	if err != nil || !ok {
		return "", false
	}
	raw, err := p.secrets.Resolve(ctx, jobSecretsService, jobKey(taskID, "url"))
	if err != nil {
		return "", true
	}
	return HostOf(raw), true
}

// Notify queues a ping for a settled run; a full queue drops it so the job is never delayed.
func (p *JobPinger) Notify(taskID, runID, status string) {
	if !pingWanted(status) {
		return
	}
	select {
	case p.queue <- jobEvent{taskID, runID, status}:
	default:
		p.logger.Warn("job heartbeat queue full, ping dropped", slog.String("task_id", taskID))
	}
}

// pingWanted is false for skipped, replaced and other non-terminal statuses so the monitor's grace period decides.
func pingWanted(status string) bool {
	return status == store.ScheduledTaskStatusSuccess || failureStatus(status)
}

func failureStatus(status string) bool {
	switch status {
	case store.ScheduledTaskStatusFailed, store.ScheduledTaskStatusTimeout,
		store.ScheduledTaskStatusInterrupted, store.ScheduledTaskStatusContainerNotRunning:
		return true
	}
	return false
}

// Run drains the queue until ctx ends.
func (p *JobPinger) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-p.queue:
			p.send(ctx, ev)
		}
	}
}

func (p *JobPinger) send(ctx context.Context, ev jobEvent) {
	pingURL, err := p.secrets.Resolve(ctx, jobSecretsService, jobKey(ev.taskID, "url"))
	if err != nil || pingURL == "" {
		return
	}
	target := pingURL
	if failureStatus(ev.status) {
		explicit, _ := p.secrets.Resolve(ctx, jobSecretsService, jobKey(ev.taskID, "fail_url"))
		target = DeriveFailURL(pingURL, explicit)
	}
	host := HostOf(target)
	if err := p.deliver(ctx, target); err != nil {
		p.logger.Warn("job heartbeat ping failed", slog.String("task_id", ev.taskID), slog.String("run_id", ev.runID),
			slog.String("host", host), slog.String("error", err.Error()))
	}
}

// deliver retries transient failures with jittered exponential backoff; a 4xx answer is final.
func (p *JobPinger) deliver(ctx context.Context, target string) error {
	if _, err := ValidateURL(target, p.cfg.AllowPrivate); err != nil {
		return err
	}
	var last error
	for attempt := 0; attempt < p.cfg.MaxAttempts; attempt++ {
		if attempt > 0 {
			d := p.cfg.BackoffBase << (attempt - 1)
			if d <= 0 || d > p.cfg.BackoffMax {
				d = p.cfg.BackoffMax
			}
			if err := p.sleep(ctx, d/2+time.Duration(p.jitter(int64(d/2)))); err != nil {
				return err
			}
		}
		retry, err := p.once(ctx, target)
		if err == nil {
			return nil
		}
		last = err
		if !retry {
			break
		}
	}
	return last
}

func (p *JobPinger) once(ctx context.Context, target string) (retry bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return false, errors.New("invalid request")
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return true, scrub(err)
	}
	_ = resp.Body.Close()
	switch {
	case resp.StatusCode < 400:
		return false, nil
	case resp.StatusCode < 500:
		return false, fmt.Errorf("monitor answered %d", resp.StatusCode)
	}
	return true, fmt.Errorf("monitor answered %d", resp.StatusCode)
}
