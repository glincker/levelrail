package api

import (
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/secretmask"
)

// EnvLogMaskIdleFlush is how long a held log tail waits for a following line before it is emitted.
const EnvLogMaskIdleFlush = "APP_LOG_MASK_IDLE_FLUSH"

const defaultLogMaskIdleFlush = 250 * time.Millisecond

// maskedSSE writes log lines through a line carry so a secret split across two
// lines is masked, and a tail held for that is flushed after an idle interval.
type maskedSSE struct {
	w     http.ResponseWriter
	carry *secretmask.LineCarry
	idle  time.Duration
	timer *time.Timer
}

func newMaskedSSE(w http.ResponseWriter) *maskedSSE {
	return &maskedSSE{
		w:     w,
		carry: secretmask.NewLineCarry(),
		idle:  envDuration(EnvLogMaskIdleFlush, defaultLogMaskIdleFlush),
	}
}

// Write queues one event; lines held for the carry window are emitted later.
func (m *maskedSSE) Write(ev sseLogEvent) {
	m.emit(m.carry.Push(ev.Stream, ev.Line))
	m.rearm()
}

// WriteDirect flushes the held tail first so markers keep their order, then writes ev as is.
func (m *maskedSSE) WriteDirect(ev sseLogEvent) {
	m.Flush()
	writeSSEEvent(m.w, ev)
}

// Flush emits every held line. Call it at stream end and when Idle fires.
func (m *maskedSSE) Flush() {
	m.emit(m.carry.Flush())
	m.rearm()
}

// Idle fires once after the last write while lines are held, else it is nil.
func (m *maskedSSE) Idle() <-chan time.Time {
	if m.timer == nil {
		return nil
	}
	return m.timer.C
}

func (m *maskedSSE) emit(lines []secretmask.Line) {
	for _, l := range lines {
		writeSSEEvent(m.w, sseLogEvent{Line: l.Text, Stream: l.Stream})
	}
}

func (m *maskedSSE) rearm() {
	if m.timer != nil {
		if !m.timer.Stop() {
			select {
			case <-m.timer.C:
			default:
			}
		}
		m.timer = nil
	}
	if m.carry.Pending() {
		m.timer = time.NewTimer(m.idle)
	}
}
