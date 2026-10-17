package main

import (
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const wakeTimingNone = "no wakes measured yet"

func formatWakeMs(ms int64) string {
	if ms < int64(time.Second/time.Millisecond) {
		return fmt.Sprintf("%d ms", ms)
	}
	return fmt.Sprintf("%.1f s", float64(ms)/1000)
}

// wakeTimingText is the one-line wake expectation, or a plain statement that
// nothing has been measured. It never invents a number.
func wakeTimingText(t apiclient.WakeTiming) string {
	if t.Samples == 0 || t.ReadyP50Ms == nil {
		return wakeTimingNone
	}
	s := "usually wakes in about " + formatWakeMs(*t.ReadyP50Ms)
	if t.ReadyP95Ms != nil {
		s += " (p95 " + formatWakeMs(*t.ReadyP95Ms) + ")"
	}
	if t.ReadyLastMs != nil {
		s += ", last " + formatWakeMs(*t.ReadyLastMs)
	}
	if t.Samples == 1 {
		return s + ", from 1 wake"
	}
	return fmt.Sprintf("%s, from %d wakes", s, t.Samples)
}

// wakeTimingShort is the compact table cell form.
func wakeTimingShort(t apiclient.WakeTiming) string {
	if t.Samples == 0 || t.ReadyP50Ms == nil {
		return "-"
	}
	if t.ReadyP95Ms == nil {
		return formatWakeMs(*t.ReadyP50Ms)
	}
	return formatWakeMs(*t.ReadyP50Ms) + " (p95 " + formatWakeMs(*t.ReadyP95Ms) + ")"
}
