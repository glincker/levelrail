package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestPrintLogEntriesHumanShowsCollapsedCount(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var b bytes.Buffer
	printLogEntriesHuman(&b, []logEntryResource{{Timestamp: t0, Stream: "stdout", Message: "retry", Count: 3, Last: t0.Add(time.Minute)}})
	if !strings.Contains(b.String(), "(x3 until 2026-01-01T00:01:00Z)") {
		t.Fatalf("got %q", b.String())
	}
}
