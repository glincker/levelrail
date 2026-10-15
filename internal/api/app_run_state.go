package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// DeployExitStore reads the exits the application controller captured.
// *store.DB satisfies it.
type DeployExitStore interface {
	GetDeployExit(ctx context.Context, serviceName, attemptID string) (*store.DeployExit, error)
	LatestDeployExit(ctx context.Context, serviceName string) (*store.DeployExit, error)
}

// deployExitResource is a captured container exit and its last output.
type deployExitResource struct {
	Container  string               `json:"container"`
	ExitCode   int                  `json:"exit_code"`
	OOMKilled  bool                 `json:"oom_killed,omitempty"`
	Reason     string               `json:"reason"`
	Message    string               `json:"message"`
	StartedAt  *time.Time           `json:"started_at,omitempty"`
	FinishedAt *time.Time           `json:"finished_at,omitempty"`
	CapturedAt time.Time            `json:"captured_at"`
	DeployID   string               `json:"deploy_id,omitempty"`
	Lines      []failureContextLine `json:"lines"`
}

func toDeployExitResource(e *store.DeployExit) *deployExitResource {
	if e == nil {
		return nil
	}
	res := &deployExitResource{
		Container: e.Container, ExitCode: e.ExitCode, OOMKilled: e.OOMKilled, Reason: e.Reason,
		Message: e.Message, StartedAt: timePtr(e.StartedAt), FinishedAt: timePtr(e.FinishedAt),
		CapturedAt: e.CapturedAt, DeployID: e.AttemptID, Lines: make([]failureContextLine, 0, len(e.LogTail)),
	}
	for _, l := range e.LogTail {
		res.Lines = append(res.Lines, failureContextLine{Timestamp: l.Timestamp, Stream: l.Stream, Message: l.Message})
	}
	return res
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// appRunStatus is whether an app's current release container is running,
// and if not, how it stopped. The live log stream sends it as event: status.
type appRunStatus struct {
	Running    bool       `json:"running"`
	Container  string     `json:"container,omitempty"`
	ExitCode   *int       `json:"exit_code,omitempty"`
	OOMKilled  bool       `json:"oom_killed,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Reason     string     `json:"reason,omitempty"`
	Message    string     `json:"message,omitempty"`
}

func (s appRunStatus) equal(o appRunStatus) bool {
	a, _ := json.Marshal(s)
	b, _ := json.Marshal(o)
	return string(a) == string(b)
}

// runStateLookup reports an app's run status; ok false means unknown.
type runStateLookup func(ctx context.Context, name string) (status appRunStatus, ok bool)

// latestExit is the newest capture for name, or nil when none or not configured.
func (rt *Router) latestExit(ctx context.Context, name string) *store.DeployExit {
	if rt.deployExits == nil {
		return nil
	}
	e, err := rt.deployExits.LatestDeployExit(ctx, name)
	if err != nil {
		rt.logger.Warn("api: load latest container exit failed", slog.String("error", err.Error()), slog.String("name", name))
		return nil
	}
	return e
}

// appRunState inspects the current release's first replica. A container
// that is gone but was captured on exit still reports how it stopped.
func (rt *Router) appRunState(ctx context.Context, name string) (appRunStatus, bool) {
	if rt.execRuntime == nil {
		return appRunStatus{}, false
	}
	svc, err := rt.apps.GetDesiredService(ctx, name)
	if err != nil || svc.Suspended {
		return appRunStatus{}, false
	}
	nodeRuntime, err := rt.execRuntime(svc.NodeID)
	if err != nil {
		return appRunStatus{}, false
	}
	inspector, ok := nodeRuntime.(docker.ExitStateInspector)
	if !ok {
		return appRunStatus{}, false
	}
	target := application.ContainerName(svc.Name, application.NameImage(*svc), svc.RestartNonce)
	es, err := inspector.InspectExitState(ctx, target)
	if err != nil {
		return appRunStatus{}, false
	}
	captured := rt.latestExit(ctx, name)
	if captured != nil && captured.Image != svc.Image {
		captured = nil
	}
	switch {
	case es != nil && es.Running:
		return appRunStatus{Running: true, Container: target}, true
	case es != nil:
		v := deploysafety.ClassifyExit(deploysafety.WorkloadService, deploysafety.ExitPhaseReadiness,
			deploysafety.ExitObservation{OOMKilled: es.OOMKilled, ExitCode: es.ExitCode, StartedAt: es.StartedAt, FinishedAt: es.FinishedAt})
		st := appRunStatus{Container: target, ExitCode: &es.ExitCode, OOMKilled: es.OOMKilled, FinishedAt: timePtr(es.FinishedAt), Reason: v.Reason, Message: v.Message}
		if captured != nil && captured.Container == target {
			st.Reason, st.Message = captured.Reason, captured.Message
		} else if r, m := rt.failingReadyCondition(ctx, name); r != "" {
			st.Reason, st.Message = r, m
		}
		return st, true
	case captured != nil:
		code := captured.ExitCode
		return appRunStatus{
			Container: captured.Container, ExitCode: &code, OOMKilled: captured.OOMKilled,
			FinishedAt: timePtr(captured.FinishedAt), Reason: captured.Reason, Message: captured.Message,
		}, true
	}
	return appRunStatus{}, false
}

func (rt *Router) failingReadyCondition(ctx context.Context, name string) (reason, message string) {
	conds, err := rt.deploys.GetConditions(ctx, applicationControllerName(name))
	if err != nil {
		return "", ""
	}
	for _, c := range conds {
		if c.Type == reconcile.ConditionTypeReady && c.Status == reconcile.ConditionFalse {
			return c.Reason, c.Message
		}
	}
	return "", ""
}

// lastOutputLines is a stopped container's last output: the exit capture
// when it is for that container, else read from Docker, which keeps the
// logs of an exited container.
func (rt *Router) lastOutputLines(ctx context.Context, name string, st appRunStatus) []failureContextLine {
	if e := rt.latestExit(ctx, name); e != nil && e.Container == st.Container && len(e.LogTail) > 0 {
		return toDeployExitResource(e).Lines
	}
	svc, err := rt.apps.GetDesiredService(ctx, name)
	if err != nil {
		return nil
	}
	return rt.containerLogLines(ctx, svc, application.ExitLogTailLines)
}

// writeSSEStatus sends st as a named status event.
func writeSSEStatus(w http.ResponseWriter, st appRunStatus) {
	data, err := json.Marshal(st)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "event: status\ndata: %s\n\n", data)
}
