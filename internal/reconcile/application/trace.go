package application

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/deploytrace"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// TraceStore is where a controller writes the deploy trace and finds the
// attempt it belongs to. *store.DB satisfies it; New picks it up from the
// ServiceStore when that is a *store.DB.
type TraceStore interface {
	deploytrace.Sink
	deploytrace.Resolver
}

// WithTraceStore overrides the trace store. nil records no trace.
func WithTraceStore(s TraceStore) Option {
	return func(ctrl *Controller) { ctrl.traceStore = s }
}

// EnvTraceEvents set to 0 or false turns the Docker event watch off.
const EnvTraceEvents = "APP_DEPLOY_TRACE_EVENTS"

// tracer returns the trace writer for the attempt converging desired.Image,
// or nil when tracing is off, no attempt matches, or its trace is sealed.
func (c *Controller) tracer(ctx context.Context, desired *store.DesiredService) *deploytrace.Tracer {
	if c.traceStore == nil || desired == nil {
		return nil
	}
	return deploytrace.ForImage(ctx, c.traceStore, c.traceStore, c.serviceName, desired.Image, slog.Default())
}

type createTrace struct {
	ctx    context.Context
	tr     *deploytrace.Tracer
	name   string
	pull   *deploytrace.Span
	create *deploytrace.Span
}

func (c *Controller) newCreateTrace(ctx context.Context, desired *store.DesiredService, name string) *createTrace {
	tr := c.tracer(ctx, desired)
	if tr == nil {
		return nil
	}
	return &createTrace{ctx: ctx, tr: tr, name: name}
}

func (t *createTrace) onPull(ev docker.PullEvent) {
	if t == nil {
		return
	}
	switch ev.Phase {
	case docker.PullPhaseStart:
		t.pull = t.tr.Begin(t.ctx, deploytrace.StepImagePull, "pulling "+ev.Ref)
	case docker.PullPhaseProgress:
		t.pull.Progress(t.ctx, pullDetail(ev))
	case docker.PullPhaseLayer:
		status := store.TraceStatusRunning
		if ev.LayerState == docker.LayerComplete || ev.LayerState == docker.LayerExists {
			status = store.TraceStatusDone
		}
		detail := ev.LayerState
		if ev.LayerTotal > 0 {
			detail += " " + deploytrace.Bytes(ev.LayerTotal)
		}
		t.tr.NoteKey(t.ctx, deploytrace.StepImagePullLayer, deploytrace.StepImagePullLayer+"."+ev.LayerID, status, "", ev.LayerID+" "+detail)
	case docker.PullPhaseDone:
		if ev.Status == docker.PullStatusCached {
			t.tr.Note(t.ctx, deploytrace.StepImagePull, store.TraceStatusSkipped, "Cached", "image already on this node, nothing pulled")
		} else {
			t.pull.Done(t.ctx, pullDetail(ev)+digestSuffix(ev))
			t.pull = nil
		}
		t.create = t.tr.Begin(t.ctx, deploytrace.StepContainerCreate, "creating "+t.name)
	}
}

func pullDetail(ev docker.PullEvent) string {
	d := fmt.Sprintf("%d%%, %d/%d layers", ev.Percent, ev.LayersDone, ev.Layers)
	if ev.BytesTotal > 0 {
		d += fmt.Sprintf(", %s of %s", deploytrace.Bytes(ev.BytesCurrent), deploytrace.Bytes(ev.BytesTotal))
	}
	if ev.Extracting > 0 {
		d += fmt.Sprintf(", %d extracting", ev.Extracting)
	}
	if ev.Existing > 0 {
		d += fmt.Sprintf(", %d already exist", ev.Existing)
	}
	return d
}

func digestSuffix(ev docker.PullEvent) string {
	switch {
	case ev.Digest != "" && ev.Status != "":
		return " (" + ev.Status + ", " + ev.Digest + ")"
	case ev.Digest != "":
		return " (" + ev.Digest + ")"
	}
	return ""
}

// created closes the pull and create spans with the Create result. A pull
// that never finished is the failing step, not the create after it.
func (t *createTrace) created(id string, err error) {
	if t == nil {
		return
	}
	if err != nil {
		if t.pull != nil {
			reason := classifyPullError(err)
			t.pull.Fail(t.ctx, reason, err)
			if reason == reasonRegistryAuthDenied {
				t.tr.Note(t.ctx, deploytrace.StepRegistryAuth, store.TraceStatusFailed, reason, errorText(err))
			}
			return
		}
		if t.create == nil {
			t.create = t.tr.Begin(t.ctx, deploytrace.StepContainerCreate, "creating "+t.name)
		}
		t.create.Fail(t.ctx, "CreateFailed", err)
		return
	}
	if t.create == nil {
		t.create = t.tr.Begin(t.ctx, deploytrace.StepContainerCreate, "creating "+t.name)
	}
	t.create.Done(t.ctx, fmt.Sprintf("container %s (%s)", t.name, shortID(id)))
}

const (
	reasonRegistryAuthDenied = "RegistryAuthDenied"
	reasonImageNotFound      = "ImageNotFound"
	reasonPlatformMismatch   = "PlatformMismatch"
	reasonImagePullFailed    = "ImagePullFailed"
)

func classifyPullError(err error) string {
	m := strings.ToLower(err.Error())
	switch {
	case strings.Contains(m, "no matching manifest"):
		return reasonPlatformMismatch
	case strings.Contains(m, "unauthorized"), strings.Contains(m, "denied"), strings.Contains(m, "authentication required"), strings.Contains(m, "no basic auth"):
		return reasonRegistryAuthDenied
	case strings.Contains(m, "manifest unknown"), strings.Contains(m, "not found"):
		return reasonImageNotFound
	}
	return reasonImagePullFailed
}

func errorText(err error) string { return strings.TrimSpace(err.Error()) }

// traceStep runs fn as a span, closing it with the error text on failure.
func traceStep(ctx context.Context, tr *deploytrace.Tracer, name, key, begin string, fn func() error, failReason, doneDetail string) error {
	span := tr.BeginKey(ctx, name, key, begin)
	if err := fn(); err != nil {
		span.Fail(ctx, failReason, err)
		return err
	}
	span.Done(ctx, doneDetail)
	return nil
}

// configDetail reports what the container will be created with. Env values
// are never printed, only names, with their source tag.
func configDetail(desired *store.DesiredService, env map[string]string) string {
	tagged := map[string]string{}
	for _, s := range desired.SecretEnv {
		tagged[s.Name] = "secret"
	}
	for k := range desired.DatabaseEnv {
		tagged[k] = "database"
	}
	for k := range desired.VaultEnv {
		tagged[k] = "vault"
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	parts := []string{deploytrace.EnvSummary(keys, tagged)}
	if desired.Port != 0 {
		parts = append(parts, fmt.Sprintf("port %d", desired.Port))
	}
	if n := len(desired.Volumes); n > 0 {
		names := make([]string, n)
		for i, v := range desired.Volumes {
			names[i] = v.Name + ":" + v.ContainerPath
		}
		parts = append(parts, "volumes "+strings.Join(names, ", "))
	}
	if r := desired.Resources; r != nil && (r.MemoryBytes > 0 || r.NanoCPUs > 0) {
		lim := []string{}
		if r.MemoryBytes > 0 {
			lim = append(lim, "memory "+deploytrace.Bytes(r.MemoryBytes))
		}
		if r.NanoCPUs > 0 {
			lim = append(lim, fmt.Sprintf("cpu %.2f", float64(r.NanoCPUs)/1e9))
		}
		parts = append(parts, "limits "+strings.Join(lim, ", "))
	} else {
		parts = append(parts, "no resource limits")
	}
	return strings.Join(parts, "; ")
}

// traceRegistryAuth records which credential the pull will use.
func traceRegistryAuth(ctx context.Context, tr *deploytrace.Tracer, desired *store.DesiredService, auth *docker.RegistryAuth) {
	switch {
	case tr == nil:
	case desired.RegistryCredentialID == "":
		tr.Note(ctx, deploytrace.StepRegistryAuth, store.TraceStatusSkipped, "NoCredential", "no registry credential configured, pulling anonymously")
	case auth != nil:
		tr.Note(ctx, deploytrace.StepRegistryAuth, store.TraceStatusDone, "", "credential resolved for user "+auth.Username)
	}
}

// traceImageInspect records the pulled image's shape and whether it matches
// the node. A mismatch is a warning here: the daemon may still run it under
// emulation, and the create or start step reports if it does not.
func (c *Controller) traceImageInspect(ctx context.Context, tr *deploytrace.Tracer, image string) {
	insp, ok := c.runtime.(docker.ImageDetailInspector)
	if !ok || tr == nil {
		return
	}
	span := tr.Begin(ctx, deploytrace.StepImageInspect, "inspecting "+image)
	d, err := insp.InspectImageDetails(ctx, image)
	if err != nil {
		span.Fail(ctx, "ImageInspectFailed", err)
		return
	}
	node, nodeErr := insp.NodePlatform(ctx)
	detail := fmt.Sprintf("%s, %s/%s, entrypoint %s, ports %s, healthcheck %s",
		deploytrace.Bytes(d.SizeBytes), d.OS, d.Arch, listOrNone(d.Entrypoint), listOrNone(d.ExposedPorts), healthText(d))
	if nodeErr == nil && node.Arch != "" && d.Arch != "" && node.Arch != d.Arch {
		span.End(ctx, store.TraceStatusWarning, reasonPlatformMismatch,
			fmt.Sprintf("%s; image is %s but this node is %s/%s", detail, d.Arch, node.OS, node.Arch))
		return
	}
	span.Done(ctx, detail+nodeMatch(node, nodeErr))
}

func nodeMatch(n docker.NodePlatform, err error) string {
	if err != nil || n.Arch == "" {
		return ""
	}
	return "; matches node " + n.OS + "/" + n.Arch
}

func healthText(d *docker.ImageDetails) string {
	if !d.HasHealthcheck {
		return "none"
	}
	return strings.Join(d.Healthcheck, " ")
}

func listOrNone(l []string) string {
	if len(l) == 0 {
		return "none"
	}
	return strings.Join(l, " ")
}

// watchLifecycle mirrors the daemon's events for one container into the trace.
// It matches by name, not label: a new label would change the config and force a recreate.
func (c *Controller) watchLifecycle(ctx context.Context, tr *deploytrace.Tracer, name string, budget time.Duration) (stop func()) {
	if tr == nil || traceEventsOff() {
		return func() {}
	}
	w, ok := c.runtime.(docker.LifecycleWatcher)
	if !ok {
		return func() {}
	}
	wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), budget+15*time.Second)
	events, errs := w.WatchContainer(wctx, name)
	go func() {
		defer cancel()
		lost := func(cause string) {
			tr.NoteKey(wctx, deploytrace.StepDockerEvent, deploytrace.StepDockerEvent+".stream", store.TraceStatusWarning, "EventStreamLost",
				"docker event stream for "+name+" ended"+cause+"; the trace may be missing daemon events")
		}
		for {
			select {
			case <-wctx.Done():
				return
			case err, ok := <-errs:
				if !ok {
					errs = nil
					continue
				}
				if err != nil {
					lost(": " + errorText(err))
					return
				}
			case ev, ok := <-events:
				if !ok {
					if wctx.Err() == nil {
						lost("")
					}
					return
				}
				if traceLifecycle(wctx, tr, name, ev) {
					return
				}
			}
		}
	}()
	return cancel
}

func traceEventsOff() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(EnvTraceEvents)))
	return v == "0" || v == "false" || v == "off"
}

// traceLifecycle writes one daemon event and reports whether it ends the watch.
func traceLifecycle(ctx context.Context, tr *deploytrace.Tracer, name string, ev docker.LifecycleEvent) bool {
	key := deploytrace.StepDockerEvent + "." + ev.Action
	detail := fmt.Sprintf("daemon: %s %s", ev.Action, name)
	status, reason := store.TraceStatusDone, ""
	switch ev.Action {
	case "health_status":
		key += "." + ev.Health
		detail = fmt.Sprintf("daemon: %s health %s", name, ev.Health)
		if ev.Health == "unhealthy" {
			status, reason = store.TraceStatusWarning, "Unhealthy"
		}
	case "oom":
		status, reason = store.TraceStatusFailed, "OOMKilled"
		detail = fmt.Sprintf("daemon: %s was killed by the kernel for exceeding its memory limit", name)
	case "die":
		detail = fmt.Sprintf("daemon: %s exited", name)
		if ev.HasExit {
			detail += fmt.Sprintf(" with code %d", ev.ExitCode)
		}
		if ev.OOMKilled {
			detail += " (oomKilled)"
			status, reason = store.TraceStatusFailed, "OOMKilled"
		} else if ev.HasExit && ev.ExitCode != 0 {
			status, reason = store.TraceStatusFailed, "ExitedNonZero"
		}
	}
	tr.NoteKey(ctx, deploytrace.StepDockerEvent, key, status, reason, detail)
	return ev.Action == "die"
}

// traceGate maps a deploy safety gate event onto the matching trace step.
func (c *Controller) traceGate(ctx context.Context, desired *store.DesiredService, container, phase, status, reason, message string) {
	if c.traceStore == nil {
		return
	}
	var step string
	switch phase {
	case deploysafety.PhaseReadiness:
		step = deploytrace.StepReadinessGate
	case deploysafety.PhaseStability:
		step = deploytrace.StepStabilityGate
	case deploysafety.PhaseCutover:
		step = deploytrace.StepCutover
	case deploysafety.PhaseDrain:
		step = deploytrace.StepDrainOld
	case deploysafety.PhasePin:
		step = deploytrace.StepRetainImage
	default:
		return
	}
	tr := c.tracer(ctx, desired)
	traceStatus := store.TraceStatusDone
	if status == deploysafety.StatusFailed {
		traceStatus = store.TraceStatusFailed
	}
	if container != "" {
		message = container + ": " + message
	}
	tr.Note(ctx, step, traceStatus, reason, message)
}

// traceAbort records that a new container never took traffic. It lists the
// containers still serving so the trace says what the half-finished deploy
// left running.
func (c *Controller) traceAbort(ctx context.Context, desired *store.DesiredService, target, reason string, cause error) {
	tr := c.tracer(ctx, desired)
	if tr == nil {
		return
	}
	serving := "no previous container is running"
	if stale, err := c.staleContainers(ctx, []string{target}); err == nil {
		var names []string
		for _, s := range stale {
			if s.Running {
				names = append(names, s.Name)
			}
		}
		if len(names) > 0 {
			serving = "previous container " + strings.Join(names, ", ") + " keeps serving"
		}
	}
	tr.Note(ctx, deploytrace.StepCutover, store.TraceStatusSkipped, "NotCutOver",
		fmt.Sprintf("traffic not moved to %s: %s", target, serving))
	tr.Note(ctx, deploytrace.StepSummary, store.TraceStatusFailed, reason, errorText(cause))
}

// traceSummary closes the trace once a deploy cut over; later restarts of the
// same image no longer write to it.
func (c *Controller) traceSummary(ctx context.Context, desired *store.DesiredService, replicas int) {
	tr := c.tracer(ctx, desired)
	tr.Note(ctx, deploytrace.StepSummary, store.TraceStatusDone, "", fmt.Sprintf("%s live on %d replica(s)", desired.Image, replicas))
}

func networkTraceLabel(prefix string, desired *store.DesiredService) string {
	if desired.AppID == "" {
		return "default bridge network"
	}
	return NetworkName(prefix, desired.AppID)
}

// traceDependenciesMet records that the services this one waited on are up,
// the point a compose depends_on ordering lets creation proceed.
func (c *Controller) traceDependenciesMet(ctx context.Context, tr *deploytrace.Tracer, desired *store.DesiredService) {
	if tr == nil || len(desired.DependsOn) == 0 {
		return
	}
	healthy := map[string]bool{}
	for _, d := range desired.DependsOnHealthy {
		healthy[d] = true
	}
	parts := make([]string, len(desired.DependsOn))
	for i, d := range desired.DependsOn {
		parts[i] = d + " started"
		if healthy[d] {
			parts[i] = d + " healthy"
		}
	}
	tr.Note(ctx, deploytrace.StepDependsOn, store.TraceStatusDone, "", "dependencies ready: "+strings.Join(parts, ", "))
}

// traceDependencyWait records the dependency this service is held behind.
func (c *Controller) traceDependencyWait(ctx context.Context, desired *store.DesiredService, dep, reason, message string) {
	if c.traceStore == nil {
		return
	}
	c.tracer(ctx, desired).NoteKey(ctx, deploytrace.StepDependsOn, deploytrace.StepDependsOn+"."+dep, store.TraceStatusRunning, reason, message)
}
