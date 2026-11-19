package deploylog

import "github.com/GLINCKER/levelrail/internal/build"

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
	case ev.Completed:
		status = "done"
	}
	r.Step(attemptID, "static:"+phase, status)
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
			// A later "running" for the same phase must not undo a "done" from its sibling label.
			phase := ev.Step[7:]
			if out[phase] == "done" && ev.Status == "running" {
				continue
			}
			out[phase] = ev.Status
		}
	}
	return out, len(out) > 0
}
