package deploylog

import (
	"context"
	"time"

	"github.com/glincker/levelrail/internal/build"
	"github.com/glincker/levelrail/internal/store"
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

// phaseRec is one static phase's state, kept apart from the step history so
// step summaries, failing_step and live step subscribers never see it.
type phaseRec struct {
	status  string
	started time.Time
	ended   time.Time
}

// noteStaticPhase records a phase transition from a static pipeline progress
// event. Only a status change is stored, not each progress line.
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
	now := time.Now()
	r.mu.Lock()
	st, live := r.active[attemptID]
	if !live {
		r.mu.Unlock()
		return
	}
	if st.phases == nil {
		st.phases = map[string]*phaseRec{}
	}
	rec := st.phases[phase]
	if rec != nil && rec.status == status {
		r.mu.Unlock()
		return
	}
	if rec == nil {
		rec = &phaseRec{started: now}
		if status != "running" {
			rec.started = previousPhaseEnd(st.phases, phase, now)
		}
		st.phases[phase] = rec
	}
	rec.status = status
	if status != "running" {
		rec.ended = now
	}
	row := *rec
	r.mu.Unlock()
	r.persistStaticPhase(attemptID, phase, row)
}

// previousPhaseEnd is when the phase before phase ended, or fallback when none did.
func previousPhaseEnd(phases map[string]*phaseRec, phase string, fallback time.Time) time.Time {
	for i, name := range StaticPhases {
		if name != phase {
			continue
		}
		for j := i - 1; j >= 0; j-- {
			if p := phases[StaticPhases[j]]; p != nil && !p.ended.IsZero() {
				return p.ended
			}
		}
	}
	return fallback
}

// persistStaticPhase keeps the phase state past the attempt so a finished
// attempt can report real states; a missing sink or a write error is ignored.
func (r *Recorder) persistStaticPhase(attemptID, phase string, rec phaseRec) {
	r.mu.Lock()
	sink := r.stageSink
	r.mu.Unlock()
	if sink == nil {
		return
	}
	row := store.DeployTraceStep{AttemptID: attemptID, Key: StepStaticPhase + "." + phase, Step: StepStaticPhase, Status: rec.status, Detail: phase, StartedAt: rec.started, EndedAt: rec.ended}
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
	out := make(map[string]string, len(st.phases))
	for name, rec := range st.phases {
		out[name] = rec.status
	}
	return out, len(out) > 0
}
