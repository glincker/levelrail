package deploylog

import (
	"context"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/store"
)

// StepStaticPhase is the persisted trace step of a static phase; its key is "static_phase.<phase>".
const StepStaticPhase = "static_phase"

// Static build phases shown to the Pages UI: coarse names over the pipeline's own step labels.
const (
	PhaseCloning    = "cloning"
	PhaseBuilding   = "building"
	PhasePublishing = "publishing"
	PhaseLive       = "live"
)

// StaticPhases lists the phases in order.
var StaticPhases = []string{PhaseCloning, PhaseBuilding, PhasePublishing, PhaseLive}

// staticStepPhase maps the static pipeline's step labels (internal/deploy) to a phase.
var staticStepPhase = map[string]string{
	"Fetch source":           PhaseCloning,
	"Build static site":      PhaseBuilding,
	"Collect build output":   PhaseBuilding,
	"Compile routing rules":  PhasePublishing,
	"Publish to file server": PhasePublishing,
}

// staticPhaseFinal is the label whose completion finishes a phase, so a phase
// with two labels is not done after the first.
var staticPhaseFinal = map[string]string{
	PhaseCloning:    "Fetch source",
	PhaseBuilding:   "Collect build output",
	PhasePublishing: "Publish to file server",
}

// IsStaticStep reports whether step is one of the static pipeline's labels.
func IsStaticStep(step string) bool {
	_, ok := staticStepPhase[step]
	return ok
}

// noteStaticPhase records a phase transition from a static pipeline progress event.
func (r *Recorder) noteStaticPhase(attemptID string, ev build.ProgressEvent) {
	phase, ok := staticStepPhase[ev.Step]
	if !ok {
		return
	}
	status := "running"
	switch {
	case ev.Error != "":
		status = "failed"
	case ev.Completed && ev.Step == staticPhaseFinal[phase]:
		status = "done"
	}
	r.Step(attemptID, "static:"+phase, status)
	r.persistStaticPhase(attemptID, phase, status)
}

// persistStaticPhase keeps the phase state past the attempt so a finished
// attempt can report real states; a missing sink or a write error is ignored.
func (r *Recorder) persistStaticPhase(attemptID, phase, status string) {
	r.mu.Lock()
	sink := r.stageSink
	r.mu.Unlock()
	if sink == nil {
		return
	}
	now := time.Now()
	row := store.DeployTraceStep{AttemptID: attemptID, Key: StepStaticPhase + "." + phase, Step: StepStaticPhase, Status: status, Detail: phase, StartedAt: now}
	if status != "running" {
		row.EndedAt = now
	}
	if err := sink.SaveDeployTraceStep(context.Background(), row); err != nil {
		r.logger.Warn("deploylog: persist static phase failed", "attempt", attemptID, "phase", phase, "error", err.Error())
	}
}

// StaticPhaseStates returns the latest state of each static phase of a live
// attempt (pending, running, done, failed). ok is false when the attempt is not
// live or has reported no static step.
func (r *Recorder) StaticPhaseStates(attemptID string) (map[string]string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	st, ok := r.active[attemptID]
	if !ok {
		return nil, false
	}
	out := map[string]string{}
	for _, ev := range st.steps {
		if len(ev.Step) > 7 && ev.Step[:7] == "static:" {
			out[ev.Step[7:]] = ev.Status
		}
	}
	return out, len(out) > 0
}
