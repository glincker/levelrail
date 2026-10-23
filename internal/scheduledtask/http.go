package scheduledtask

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// HTTPTarget wakes an app if it sleeps, waits until it can answer, and
// returns the base URL its HTTP port is reachable on from the control plane.
type HTTPTarget interface {
	Prepare(ctx context.Context, svc *store.DesiredService) (baseURL string, err error)
}

// HTTPConfigStore loads the request an http-mode job sends.
type HTTPConfigStore interface {
	GetScheduledTaskHTTP(ctx context.Context, taskID string) (store.ScheduledTaskHTTP, error)
}

// Headers sent on every http-mode invocation so the app can tell a job call
// from user traffic and correlate it with run history.
const (
	HeaderJobRun        = "X-Job-Run"
	HeaderJobName       = "X-Job-Name"
	HeaderJobBackground = "X-Job-Background"
)

// httpTimeout picks the attempt timeout: an explicit per-job value wins, then
// the longer background timeout for background jobs, then the default.
func (r *Runner) httpTimeout(task store.ScheduledTask, cfg store.ScheduledTaskHTTP) time.Duration {
	if r.Timeout > 0 || task.TimeoutSeconds > 0 || !cfg.Background {
		return r.attemptTimeout(task)
	}
	return r.limits().BackgroundTimeout
}

// attemptHTTP performs one invocation of the job's HTTP target.
func (r *Runner) attemptHTTP(runCtx context.Context, task store.ScheduledTask, runID string) result {
	svc, err := r.Apps.GetDesiredService(runCtx, task.ServiceName)
	if err != nil {
		return result{
			status: store.ScheduledTaskStatusFailed, message: "load app: " + err.Error(),
			infraErr: fmt.Errorf("scheduledtask: run %q: load service %q: %w", task.ID, task.ServiceName, err),
		}
	}
	res := result{nodeID: svc.NodeID}
	if r.Target == nil || r.HTTPConfigs == nil {
		res.status, res.output = store.ScheduledTaskStatusFailed, "http jobs are not available on this control plane"
		return res
	}
	cfg, err := r.HTTPConfigs.GetScheduledTaskHTTP(runCtx, task.ID)
	if err != nil {
		res.status, res.output = store.ScheduledTaskStatusFailed, "load http target: "+err.Error()
		return res
	}

	timeout := r.httpTimeout(task, cfg)
	execCtx, cancel := context.WithTimeout(runCtx, timeout)
	defer cancel()

	base, err := r.Target.Prepare(execCtx, svc)
	if err != nil {
		return r.httpFailure(runCtx, execCtx, res, timeout, "wake app: "+err.Error(), store.ScheduledTaskStatusContainerNotRunning)
	}
	target, err := joinTarget(base, cfg.Path)
	if err != nil {
		res.status, res.output = store.ScheduledTaskStatusFailed, err.Error()
		return res
	}

	var body io.Reader
	if cfg.Body != "" {
		body = strings.NewReader(cfg.Body)
	}
	req, err := http.NewRequestWithContext(execCtx, orMethod(cfg.Method), target, body)
	if err != nil {
		res.status, res.output = store.ScheduledTaskStatusFailed, "build request: "+err.Error()
		return res
	}
	req.Header.Set(HeaderJobRun, runID)
	req.Header.Set(HeaderJobName, task.Name)
	if cfg.Background {
		req.Header.Set(HeaderJobBackground, "1")
	}
	if cfg.Body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := r.httpClient().Do(req)
	if err != nil {
		return r.httpFailure(runCtx, execCtx, res, timeout, "request failed: "+err.Error(), store.ScheduledTaskStatusFailed)
	}
	defer func() { _ = resp.Body.Close() }()

	capped := newTailCappedWriter(r.outputCap())
	_, copyErr := io.Copy(capped, resp.Body)
	code := resp.StatusCode
	res.exitCode = &code
	res.output = capped.String()
	switch {
	case copyErr != nil && execCtx.Err() != nil:
		return r.httpFailure(runCtx, execCtx, res, timeout, "reading response: "+copyErr.Error(), store.ScheduledTaskStatusFailed)
	case code >= 200 && code < 300:
		res.status = store.ScheduledTaskStatusSuccess
	default:
		res.status = store.ScheduledTaskStatusFailed
		res.message = fmt.Sprintf("app answered HTTP %d", code)
	}
	return res
}

// httpFailure classifies a transport-level failure as a shutdown, a timeout
// or the given fallback status.
func (r *Runner) httpFailure(runCtx, execCtx context.Context, res result, timeout time.Duration, msg, fallback string) result {
	switch {
	case isReplaced(runCtx):
		res.status, res.output = store.ScheduledTaskStatusReplaced, replacedOutputMessage
	case errors.Is(runCtx.Err(), context.Canceled):
		res.status, res.output = store.ScheduledTaskStatusInterrupted, shutdownMessage
	case errors.Is(execCtx.Err(), context.DeadlineExceeded):
		res.status, res.output = store.ScheduledTaskStatusTimeout, fmt.Sprintf("request timed out after %s", timeout)
	default:
		res.status, res.output = fallback, msg
	}
	return res
}

// appClient never follows redirects: a job calls its own app, not wherever the
// app points it. The target is the app's own published port, so the shared
// SSRF-guarded client (which refuses loopback) does not apply.
var appClient = &http.Client{
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

func (r *Runner) httpClient() *http.Client {
	if r.Client != nil {
		return r.Client
	}
	return appClient
}

func orMethod(m string) string {
	if m == "" {
		return http.MethodPost
	}
	return m
}

// joinTarget joins a base URL and an absolute path, refusing a path that
// could redirect the call to another host.
func joinTarget(base, path string) (string, error) {
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return "", fmt.Errorf("path %q must start with a single /", path)
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("base url %q: %w", base, err)
	}
	ref, err := url.Parse(path)
	if err != nil || ref.Host != "" || ref.Scheme != "" {
		return "", fmt.Errorf("path %q is not a plain path", path)
	}
	return u.ResolveReference(ref).String(), nil
}
