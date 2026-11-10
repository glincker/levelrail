package database

import "sync"

// SharedState keeps per database state that must outlive one reconcile pass,
// since controllers are rebuilt on every pass. Create one per process.
type SharedState struct {
	mu      sync.Mutex
	watches map[string]paramsWatch
	guards  map[string]*recreateGuard
}

// NewSharedState returns an empty registry.
func NewSharedState() *SharedState {
	return &SharedState{watches: map[string]paramsWatch{}, guards: map[string]*recreateGuard{}}
}

// WithSharedState makes the controller keep its params watch and recreate
// guard in s instead of in the controller itself.
func WithSharedState(s *SharedState) Option {
	return func(c *Controller) {
		if s != nil {
			c.shared = s
		}
	}
}

// Retain forgets every database not in live, so deleted ones do not leak.
func (s *SharedState) Retain(live []string) {
	keep := make(map[string]bool, len(live))
	for _, n := range live {
		keep[n] = true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for n := range s.watches {
		if !keep[n] {
			delete(s.watches, n)
		}
	}
	for n := range s.guards {
		if !keep[n] {
			delete(s.guards, n)
		}
	}
}

func (s *SharedState) guard(name string) *recreateGuard {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.guards[name]
	if !ok {
		g = &recreateGuard{}
		s.guards[name] = g
	}
	return g
}

func (s *SharedState) watch(name string) (paramsWatch, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.watches[name]
	return w, ok
}

func (s *SharedState) setWatch(name string, w paramsWatch) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.watches[name] = w
}

func (s *SharedState) clearWatch(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.watches, name)
}

func (s *SharedState) noteVanished(name, label string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if w, ok := s.watches[name]; ok && w.label == label {
		w.vanished++
		s.watches[name] = w
	}
}
