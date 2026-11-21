package nodeinsight

import "sync"

// Sample is one buffered agent report (a metrics point or an event).
type Sample struct {
	Kind    string
	Payload []byte
}

// OfflineBuffer holds reports while the control plane is unreachable. It is
// bounded by count and bytes, dropping the oldest first so a long outage keeps
// the most recent picture instead of growing without limit.
type OfflineBuffer struct {
	mu       sync.Mutex
	items    []Sample
	bytes    int
	dropped  int
	maxItems int
	maxBytes int
}

// NewOfflineBuffer builds a buffer. A limit of 0 or less disables buffering.
func NewOfflineBuffer(maxItems, maxBytes int) *OfflineBuffer {
	return &OfflineBuffer{maxItems: maxItems, maxBytes: maxBytes}
}

// SetLimits applies new limits live and trims to fit.
func (b *OfflineBuffer) SetLimits(maxItems, maxBytes int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.maxItems, b.maxBytes = maxItems, maxBytes
	b.trim()
}

// Add stores a sample, evicting the oldest as needed. It reports whether the
// sample was kept.
func (b *OfflineBuffer) Add(s Sample) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.maxItems <= 0 || b.maxBytes <= 0 || len(s.Payload) > b.maxBytes {
		b.dropped++
		return false
	}
	b.items = append(b.items, s)
	b.bytes += len(s.Payload)
	b.trim()
	return true
}

func (b *OfflineBuffer) trim() {
	for len(b.items) > 0 && (len(b.items) > b.maxItems || b.bytes > b.maxBytes) {
		b.bytes -= len(b.items[0].Payload)
		b.items = b.items[1:]
		b.dropped++
	}
}

// Replay sends buffered samples oldest first through send and stops at the
// first error, keeping the unsent rest. It returns how many were delivered.
func (b *OfflineBuffer) Replay(send func(Sample) error) int {
	sent := 0
	for {
		b.mu.Lock()
		if len(b.items) == 0 {
			b.mu.Unlock()
			return sent
		}
		s := b.items[0]
		b.mu.Unlock()
		if err := send(s); err != nil {
			return sent
		}
		b.mu.Lock()
		if len(b.items) > 0 {
			b.bytes -= len(b.items[0].Payload)
			b.items = b.items[1:]
		}
		b.mu.Unlock()
		sent++
	}
}

// Stats reports queued samples, bytes and how many were dropped so far.
func (b *OfflineBuffer) Stats() (items, bytes, dropped int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.items), b.bytes, b.dropped
}
