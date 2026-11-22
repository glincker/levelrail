package telemetry

import (
	"testing"
	"time"
)

func TestPresenceResourceActive(t *testing.T) {
	now := time.Unix(1000, 0)
	p := NewPresence(time.Minute)
	p.now = func() time.Time { return now }
	p.TouchResource("service:web")
	if !p.ResourceActive("service:web") || p.ResourceActive("service:db") {
		t.Fatal("only the touched resource should be active")
	}
	now = now.Add(2 * time.Minute)
	if p.ResourceActive("service:web") {
		t.Fatal("touch should expire after the window")
	}
}
