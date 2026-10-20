package alerting

import (
	"fmt"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

func benchLogEntries(n int) []telemetry.LogEntry {
	out := make([]telemetry.LogEntry, 0, n)
	for i := 0; i < n; i++ {
		msg := fmt.Sprintf("GET /api/v1/apps/%d 200 %dms", i, i%250)
		if i%10 == 0 {
			msg = fmt.Sprintf("ERROR upstream timeout after %dms request=%d", i, i)
		}
		out = append(out, telemetry.LogEntry{
			ResourceID: "svc-web",
			Stream:     "stdout",
			Timestamp:  logRuleClock.Add(-time.Duration(n-i) * time.Second),
			Message:    msg,
		})
	}
	return out
}

func BenchmarkMatchLogRule(b *testing.B) {
	entries := benchLogEntries(500)
	policy := logRulePolicy()
	cases := map[string]LogRuleConfig{
		"regex": {Regex: `timeout after \d+ms`, Count: 5, WindowSeconds: 600},
		"level": {Level: "error", Count: 5, WindowSeconds: 600},
		"plain": {Query: "GET", Count: 5, WindowSeconds: 600},
	}
	for name, cfg := range cases {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := MatchLogRule(KindLogPattern, cfg, entries, false, logRuleClock, policy); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
