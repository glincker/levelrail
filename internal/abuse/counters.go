package abuse

import "sync/atomic"

const counterStripes = 16

// stripe is one cache-line sized set of counters, so concurrent requests from
// different clients do not fight over one line.
type stripe struct {
	allowed    atomic.Uint64
	wouldBlock [ReasonCount]atomic.Uint64
	blocked    [ReasonCount]atomic.Uint64
	failOpen   atomic.Uint64
	_          [8]byte
}

// Counters tallies one app's decisions without taking a lock.
type Counters struct {
	stripes [counterStripes]stripe
}

// Snapshot is a drained copy of the counters.
type Snapshot struct {
	Allowed    uint64
	WouldBlock [ReasonCount]uint64
	Blocked    [ReasonCount]uint64
	FailOpen   uint64
}

// Zero reports whether nothing was counted.
func (s Snapshot) Zero() bool { return s == Snapshot{} }

// Add merges o into s.
func (s *Snapshot) Add(o Snapshot) {
	s.Allowed += o.Allowed
	s.FailOpen += o.FailOpen
	for i := range s.WouldBlock {
		s.WouldBlock[i] += o.WouldBlock[i]
		s.Blocked[i] += o.Blocked[i]
	}
}

func (c *Counters) at(h uint64) *stripe { return &c.stripes[h&(counterStripes-1)] }

// NoteFailOpen counts one request served because the defense failed.
func (c *Counters) NoteFailOpen() { c.stripes[0].failOpen.Add(1) }

// Drain returns what was counted since the last call and resets it.
func (c *Counters) Drain() Snapshot {
	var s Snapshot
	for i := range c.stripes {
		st := &c.stripes[i]
		s.Allowed += st.allowed.Swap(0)
		s.FailOpen += st.failOpen.Swap(0)
		for r := range s.WouldBlock {
			s.WouldBlock[r] += st.wouldBlock[r].Swap(0)
			s.Blocked[r] += st.blocked[r].Swap(0)
		}
	}
	return s
}
