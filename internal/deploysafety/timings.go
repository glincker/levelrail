package deploysafety

import "time"

// Windows are the platform-wide deploy timings an operator can change at
// runtime; the hold lives beside them because it is read with the drain.
type Windows struct {
	Drain, Stable, Watch, Hold time.Duration
}

// WithWindows returns e with the platform timings replaced, before any
// per-app override is resolved on top.
func (e Effective) WithWindows(w Windows) Effective {
	e.DrainTimeout, e.StableWindow, e.WatchWindow = w.Drain, w.Stable, w.Watch
	return e
}
