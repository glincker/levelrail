package main

import (
	"sync"

	"github.com/GLINCKER/levelrail/internal/footprint"
)

// pendingStages tracks deferred boot stages so /readyz can say what is still starting.
type pendingStages struct {
	mu    sync.Mutex
	names []string
}

func (p *pendingStages) add(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.names = append(p.names, name)
}

func (p *pendingStages) done(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, n := range p.names {
		if n == name {
			p.names = append(p.names[:i], p.names[i+1:]...)
			return
		}
	}
}

// List returns a copy of the stages still running, or nil when none.
func (p *pendingStages) List() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.names) == 0 {
		return nil
	}
	return append([]string(nil), p.names...)
}

type bootStage struct {
	name       string
	fn         func() error
	deferrable bool
}

// runDeferredStages runs queued stages in order once the listener is up.
func (s *bootState) runDeferredStages(queue []bootStage) {
	for _, st := range queue {
		_ = footprint.TimeBootStage(st.name, st.fn)
		s.pending.done(st.name)
	}
}
