package mesh

import (
	"sync"
	"time"
)

// Waker turns node membership changes into a debounced request for one
// reconcile pass. It only asks for a pass: the pass still reads the fleet from the store.
type Waker struct {
	mu      sync.Mutex
	pending bool
	target  func()
	window  func() time.Duration
	after   func(d time.Duration, f func())
}

// NewWaker builds a Waker that calls target once per window after the
// first Wake in it. window is read on every burst, so a settings change applies live.
func NewWaker(target func(), window func() time.Duration) *Waker {
	return &Waker{
		target: target,
		window: window,
		after:  func(d time.Duration, f func()) { time.AfterFunc(d, f) },
	}
}

// Wake requests a pass. It never blocks, so it is safe on an agent session goroutine.
func (w *Waker) Wake() {
	w.mu.Lock()
	if w.pending || w.target == nil {
		w.mu.Unlock()
		return
	}
	w.pending = true
	w.mu.Unlock()
	// The window may be a settings store read, which must not run on the caller's goroutine.
	go w.schedule()
}

func (w *Waker) schedule() {
	var d time.Duration
	if w.window != nil {
		d = max(w.window(), 0)
	}
	w.after(d, w.fire)
}

func (w *Waker) fire() {
	w.mu.Lock()
	w.pending = false
	target := w.target
	w.mu.Unlock()
	target()
}
