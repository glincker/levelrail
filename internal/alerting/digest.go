package alerting

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// ChannelDigest holds warning notices for channels that opted into digest delivery,
// so a burst of warnings in one window leaves as a single message.
type ChannelDigest struct {
	mu      sync.Mutex
	window  time.Duration
	pending map[string]*digestBatch
}

type digestBatch struct {
	first time.Time
	lines []string
}

// DigestDelivery is one combined message for one channel.
type DigestDelivery struct {
	ChannelID string
	Text      string
	Count     int
}

// NewChannelDigest builds a buffer; window comes from a runtime setting, not a constant.
func NewChannelDigest(window time.Duration) *ChannelDigest {
	return &ChannelDigest{window: window, pending: map[string]*digestBatch{}}
}

// SetWindow applies a changed setting to the next flush.
func (d *ChannelDigest) SetWindow(w time.Duration) {
	d.mu.Lock()
	d.window = w
	d.mu.Unlock()
}

// Offer reports whether the notice was held for a digest. Only warnings are held:
// critical and info notices go out immediately, so a page is never delayed.
func (d *ChannelDigest) Offer(channelID string, sev string, line string, at time.Time) bool {
	if sev != SeverityWarning {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	b := d.pending[channelID]
	if b == nil {
		b = &digestBatch{first: at}
		d.pending[channelID] = b
	}
	b.lines = append(b.lines, line)
	return true
}

// Flush returns one delivery per channel whose window has elapsed and clears those batches.
func (d *ChannelDigest) Flush(now time.Time) []DigestDelivery {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []DigestDelivery
	for id, b := range d.pending {
		if now.Sub(b.first) < d.window {
			continue
		}
		out = append(out, DigestDelivery{ChannelID: id, Count: len(b.lines), Text: renderChannelDigest(b.lines)})
		delete(d.pending, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ChannelID < out[j].ChannelID })
	return out
}

func renderChannelDigest(lines []string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Alert digest: %d warnings.", len(lines))
	for _, l := range lines {
		sb.WriteString("\n- " + l)
	}
	return sb.String()
}
