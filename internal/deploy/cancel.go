package deploy

import (
	"context"
	"errors"
	"sync"
)

// ErrCanceled marks a deploy an operator canceled before it wrote desired state.
var ErrCanceled = errors.New("deploy canceled")

// CancelOutcome is what CancelRegistry.Cancel found.
type CancelOutcome int

const (
	// CancelUnknown means no in-process deploy is registered under that id.
	CancelUnknown CancelOutcome = iota
	// CancelRequested means the deploy was stopped before it could commit.
	CancelRequested
	// CancelTooLate means the deploy already committed desired state.
	CancelTooLate
)

type cancelEntry struct {
	cancel    context.CancelFunc
	canceled  bool
	committed bool
	by        string
}

// CancelRegistry tracks in-flight deploys so an operator can stop one. A
// deploy is cancelable until it calls Commit, right before it writes desired
// state, so a cancel can never leave desired state half-applied.
type CancelRegistry struct {
	mu      sync.Mutex
	entries map[string]*cancelEntry
}

// NewCancelRegistry returns an empty registry.
func NewCancelRegistry() *CancelRegistry {
	return &CancelRegistry{entries: map[string]*cancelEntry{}}
}

// Register starts tracking deploy id.
func (r *CancelRegistry) Register(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.entries[id]; !ok {
		r.entries[id] = &cancelEntry{}
	}
}

// Bind derives the context the deploy runs under, canceled by Cancel. An
// unregistered id gets parent back unchanged.
func (r *CancelRegistry) Bind(parent context.Context, id string) context.Context {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[id]
	if !ok {
		return parent
	}
	ctx, cancel := context.WithCancel(parent)
	e.cancel = cancel
	if e.canceled {
		cancel()
	}
	return ctx
}

// Cancel stops deploy id unless it already committed. by is recorded for
// Canceled.
func (r *CancelRegistry) Cancel(id, by string) CancelOutcome {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[id]
	if !ok {
		return CancelUnknown
	}
	if e.committed {
		return CancelTooLate
	}
	if !e.canceled {
		e.canceled, e.by = true, by
	}
	if e.cancel != nil {
		e.cancel()
	}
	return CancelRequested
}

// Commit marks the point of no return. It fails with ErrCanceled when the
// deploy was canceled first; after it succeeds Cancel reports CancelTooLate.
func (r *CancelRegistry) Commit(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[id]
	if !ok {
		return nil
	}
	if e.canceled {
		return ErrCanceled
	}
	e.committed = true
	return nil
}

// Canceled reports whether deploy id was canceled, and by whom.
func (r *CancelRegistry) Canceled(id string) (by string, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, found := r.entries[id]
	if !found || !e.canceled {
		return "", false
	}
	return e.by, true
}

// Release stops tracking deploy id and frees its context.
func (r *CancelRegistry) Release(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.entries[id]; ok {
		if e.cancel != nil {
			e.cancel()
		}
		delete(r.entries, id)
	}
}
