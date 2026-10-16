package alerting

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

func TestMatchLogRuleSampleKeepsRuneBoundary(t *testing.T) {
	// A multi-byte rune straddling maxLogSampleLen must not be cut in half.
	msg := strings.Repeat("a", maxLogSampleLen-1) + "é" + "tail"
	entries := []telemetry.LogEntry{{Message: msg, Timestamp: logRuleClock}}
	res, err := MatchLogRule(KindLogPattern, LogRuleConfig{Count: 1, WindowSeconds: 600}, entries, false, logRuleClock, logRulePolicy())
	if err != nil {
		t.Fatalf("MatchLogRule: %v", err)
	}
	if len(res.Samples) != 1 {
		t.Fatalf("samples = %d, want 1", len(res.Samples))
	}
	if !utf8.ValidString(res.Samples[0]) {
		t.Fatalf("sample is not valid UTF-8: %q", res.Samples[0])
	}
}
