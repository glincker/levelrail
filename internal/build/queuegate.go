package build

import (
	"context"
	"sort"
	"sync/atomic"
	"time"
)

// QueueGate holds new builds while an operator paused or is draining the build queue.
type QueueGate interface {
	Hold(ctx context.Context, wait func(msg string)) error
}

var queueGate atomic.Pointer[QueueGate]

// SetQueueGate wires the operator switch at boot; nil removes it.
func SetQueueGate(g QueueGate) {
	if g == nil {
		queueGate.Store(nil)
		return
	}
	queueGate.Store(&g)
}

// holdQueue runs before the host pressure gate and the slot wait, on every node,
// because the switch is control plane wide. Without a gate it returns at once.
func holdQueue(ctx context.Context, progress func(ProgressEvent)) error {
	g := queueGate.Load()
	if g == nil {
		return nil
	}
	return (*g).Hold(ctx, func(msg string) {
		if progress != nil {
			progress(ProgressEvent{Step: "queue", Log: msg, Stream: "stderr"})
		}
	})
}

// Active build phases as the limiter sees them.
const (
	ActiveHeld     = "held"
	ActiveWaiting  = "waiting"
	ActiveBuilding = "building"
)

// ActiveBuild is one build the limiter is holding, waiting on a slot for, or running.
type ActiveBuild struct {
	App       string
	Tag       string
	Node      string
	Phase     string
	QueuedAt  time.Time
	StartedAt time.Time
}

func (l *Limiter) track(tag, node string) (uint64, func(phase string)) {
	l.activeMu.Lock()
	defer l.activeMu.Unlock()
	if l.active == nil {
		l.active = map[uint64]*ActiveBuild{}
	}
	l.activeSeq++
	id := l.activeSeq
	l.active[id] = &ActiveBuild{App: AppFromTag(tag), Tag: tag, Node: node, Phase: ActiveHeld, QueuedAt: l.clock.Now()}
	return id, func(phase string) {
		l.activeMu.Lock()
		defer l.activeMu.Unlock()
		if e := l.active[id]; e != nil {
			e.Phase = phase
			if phase == ActiveBuilding {
				e.StartedAt = l.clock.Now()
			}
		}
	}
}

func (l *Limiter) untrack(id uint64) {
	l.activeMu.Lock()
	delete(l.active, id)
	l.activeMu.Unlock()
}

// Active lists the builds in flight, oldest first.
func (l *Limiter) Active() []ActiveBuild {
	l.activeMu.Lock()
	out := make([]ActiveBuild, 0, len(l.active))
	for _, e := range l.active {
		out = append(out, *e)
	}
	l.activeMu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].QueuedAt.Before(out[j].QueuedAt) })
	return out
}
