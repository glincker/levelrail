package exposure

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"
)

// Change is a restriction that is applied in the kernel but not yet confirmed.
type Change struct {
	Port      int       `json:"port"`
	Protocol  string    `json:"protocol"`
	Allow     []string  `json:"allow"`
	ExpiresAt time.Time `json:"expires_at"`
}

type pendingEntry struct {
	change   Change
	previous *Restriction
	stop     func() bool
}

type pendingState struct {
	mu       sync.Mutex
	entries  map[string]*pendingEntry
	now      func() time.Time
	after    func(time.Duration, func()) func() bool
	onRevert func(Change, error)
}

func (p *pendingState) clock() time.Time {
	if p.now != nil {
		return p.now()
	}
	return time.Now()
}

func (p *pendingState) schedule(d time.Duration, f func()) func() bool {
	if p.after != nil {
		return p.after(d, f)
	}
	return time.AfterFunc(d, f).Stop
}

func pendingKey(proto string, port int) string { return proto + "/" + strconv.Itoa(port) }

func (m *Manager) isPending(proto string, port int) bool {
	m.pend.mu.Lock()
	defer m.pend.mu.Unlock()
	_, ok := m.pend.entries[pendingKey(proto, port)]
	return ok
}

func (m *Manager) isPendingTag(tag string) bool {
	m.pend.mu.Lock()
	defer m.pend.mu.Unlock()
	for k := range m.pend.entries {
		if tag == m.prefix+tagInfix+k {
			return true
		}
	}
	return false
}

// SetClock replaces the time source and timer (tests).
func (m *Manager) SetClock(now func() time.Time, after func(time.Duration, func()) func() bool) {
	m.pend.now, m.pend.after = now, after
}

// OnRevert registers a callback run after an unconfirmed restriction was rolled back.
func (m *Manager) OnRevert(f func(Change, error)) { m.pend.onRevert = f }

// ApplyWithRevert installs r like Apply, then rolls it back after window unless
// Confirm is called first. previous is what to restore on rollback (nil removes
// the rules). The control plane forgets pending changes on restart, and the
// next Sync then drops rules that are not stored, which is the same rollback.
func (m *Manager) ApplyWithRevert(ctx context.Context, r Restriction, previous *Restriction, window time.Duration) (Change, error) {
	r, err := m.Normalize(r)
	if err != nil {
		return Change{}, err
	}
	if window <= 0 {
		return Change{}, errors.New("exposure: revert window must be positive")
	}
	key := pendingKey(r.Protocol, r.Port)
	m.pend.mu.Lock()
	if old, ok := m.pend.entries[key]; ok {
		// A second change keeps the original rollback target.
		old.stop()
		previous = old.previous
		delete(m.pend.entries, key)
	}
	m.pend.mu.Unlock()
	if _, err := m.Apply(ctx, r); err != nil {
		return Change{}, err
	}
	ch := Change{Port: r.Port, Protocol: r.Protocol, Allow: r.Allow, ExpiresAt: m.pend.clock().Add(window).UTC()}
	m.pend.mu.Lock()
	defer m.pend.mu.Unlock()
	if m.pend.entries == nil {
		m.pend.entries = map[string]*pendingEntry{}
	}
	m.pend.entries[key] = &pendingEntry{change: ch, previous: previous, stop: m.pend.schedule(window, func() { m.revert(key) })}
	return ch, nil
}

func (m *Manager) revert(key string) {
	m.pend.mu.Lock()
	e, ok := m.pend.entries[key]
	delete(m.pend.entries, key)
	m.pend.mu.Unlock()
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var err error
	if e.previous != nil {
		_, err = m.Apply(ctx, *e.previous)
	} else {
		err = m.Remove(ctx, e.change.Port, e.change.Protocol)
	}
	if cb := m.pend.onRevert; cb != nil {
		cb(e.change, err)
	}
}

// Confirm keeps a pending restriction and returns it so the caller can store it.
func (m *Manager) Confirm(proto string, port int) (Change, bool) {
	m.pend.mu.Lock()
	defer m.pend.mu.Unlock()
	key := pendingKey(proto, port)
	e, ok := m.pend.entries[key]
	if !ok {
		return Change{}, false
	}
	e.stop()
	delete(m.pend.entries, key)
	return e.change, true
}

// Forget cancels a pending rollback without touching the rules.
func (m *Manager) Forget(proto string, port int) {
	m.Confirm(proto, port)
}

// Pending lists the unconfirmed restrictions.
func (m *Manager) Pending() []Change {
	m.pend.mu.Lock()
	defer m.pend.mu.Unlock()
	out := make([]Change, 0, len(m.pend.entries))
	for _, e := range m.pend.entries {
		out = append(out, e.change)
	}
	return out
}
