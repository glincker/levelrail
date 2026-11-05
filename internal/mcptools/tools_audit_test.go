package mcptools

import (
	"testing"
	"time"
)

func TestAgoOrTime(t *testing.T) {
	if got := agoOrTime("2026-01-01T00:00:00Z"); got != "2026-01-01T00:00:00Z" {
		t.Errorf("timestamp changed: %q", got)
	}
	if got := agoOrTime(""); got != "" {
		t.Errorf("empty changed: %q", got)
	}
	got, err := time.Parse(time.RFC3339, agoOrTime("24h"))
	if err != nil || time.Since(got) < 23*time.Hour || time.Since(got) > 25*time.Hour {
		t.Errorf("24h resolved to %v, %v", got, err)
	}
}
