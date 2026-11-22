package reconcile

import (
	"strings"
	"time"
)

// idleState is what the engine remembers of a controller's last run: when it
// ran and whether it ended settled, so a quiet pass can leave it alone.
type idleState struct {
	at      time.Time
	settled bool
}

// dirtySet collects what happened since the last pass: a desired-state nudge
// (all), a Docker event (container names) or a controller asked to run again.
type dirtySet struct {
	all         bool
	containers  map[string]struct{}
	controllers map[string]struct{}
}

// SetEventDriven makes a pass skip each per-service controller that settled,
// saw no Docker event, no nudge and no requeue since its last run, and ran
// within the full resync window. Call it before Run.
func (e *Engine) SetEventDriven(on bool) { e.eventDriven.Store(on) }

// SetEventDrivenSource makes every pass re-read the on/off choice, so flipping
// the runtime setting applies live. Off means a full pass, as before.
func (e *Engine) SetEventDrivenSource(f func() bool) { e.eventDrivenSource = f }

func (e *Engine) eventDrivenOn() bool {
	if e.eventDrivenSource != nil {
		return e.eventDrivenSource()
	}
	return e.eventDriven.Load()
}

func (e *Engine) markAllDirty() {
	e.dmu.Lock()
	e.dirty.all = true
	e.dmu.Unlock()
}

func (e *Engine) markContainerDirty(name string) {
	if name == "" {
		return
	}
	e.dmu.Lock()
	if e.dirty.containers == nil {
		e.dirty.containers = map[string]struct{}{}
	}
	e.dirty.containers[name] = struct{}{}
	e.dmu.Unlock()
}

func (e *Engine) markControllerDirty(name string) {
	e.dmu.Lock()
	if e.dirty.controllers == nil {
		e.dirty.controllers = map[string]struct{}{}
	}
	e.dirty.controllers[name] = struct{}{}
	e.dmu.Unlock()
}

// takeDirty hands the pass what accumulated and starts a fresh set, so an
// event that lands mid-pass still dirties the next one.
func (e *Engine) takeDirty() dirtySet {
	e.dmu.Lock()
	defer e.dmu.Unlock()
	d := e.dirty
	e.dirty = dirtySet{}
	return d
}

// idleSkippable reports whether c may be left alone this pass.
func (e *Engine) idleSkippable(c Controller, d dirtySet, on bool) bool {
	if !on || d.all || e.keyed == nil {
		return false
	}
	name := c.Name()
	if !e.keyed(name) {
		return false
	}
	if _, ok := d.controllers[name]; ok {
		return false
	}
	e.mu.RLock()
	st, ran := e.idle[name]
	e.mu.RUnlock()
	if !ran || !st.settled || e.now().Sub(st.at) >= e.fullResync {
		return false
	}
	svc := name[strings.IndexByte(name, '/')+1:]
	for cn := range d.containers {
		if strings.HasPrefix(cn, svc+"-") {
			return false
		}
	}
	return true
}

// recordIdle notes how a controller ended; only a clean, Ready-true result with
// no requeue counts as settled, so failing probes and rollouts keep running.
func (e *Engine) recordIdle(name string, r Result, err error, panicked bool) {
	settled := err == nil && !panicked && r.RequeueAfter == 0 && len(r.Conditions) > 0
	if settled {
		for _, cond := range r.Conditions {
			if cond.Status != ConditionTrue {
				settled = false
				break
			}
		}
	}
	e.mu.Lock()
	if e.idle == nil {
		e.idle = map[string]idleState{}
	}
	e.idle[name] = idleState{at: e.now(), settled: settled}
	e.mu.Unlock()
}

func (e *Engine) forgetIdle(live map[string]struct{}) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for name := range e.idle {
		if _, ok := live[name]; !ok {
			delete(e.idle, name)
		}
	}
}
