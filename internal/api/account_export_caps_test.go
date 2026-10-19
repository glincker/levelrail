package api

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestZipJSONRejectsOversizedExport(t *testing.T) {
	if _, err := zipJSON(map[string]any{"a.json": strings.Repeat("x", exportMaxBytes+1)}); err == nil {
		t.Fatal("an export over the byte cap must fail instead of buffering it")
	}
	if _, err := zipJSON(map[string]any{"a.json": "small"}); err != nil {
		t.Fatalf("a small export must pass: %v", err)
	}
}

func TestStartExportBoundsHeldJobs(t *testing.T) {
	var s accountSelfState
	now := time.Now()
	for i := range exportMaxHeld + 20 {
		s.startExport(fmt.Sprintf("id%d", i), fmt.Sprintf("user%d", i), now.Add(time.Duration(i)*time.Second))
	}
	if len(s.exports) > exportMaxHeld {
		t.Fatalf("held %d exports, cap is %d", len(s.exports), exportMaxHeld)
	}
	if _, ok := s.exports[fmt.Sprintf("id%d", exportMaxHeld+19)]; !ok {
		t.Fatal("the newest export must survive eviction")
	}
	if _, ok := s.exports["id0"]; ok {
		t.Fatal("the oldest export must be evicted first")
	}
}
