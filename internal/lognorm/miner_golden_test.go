package lognorm

import (
	"strings"
	"testing"
)

type goldenCase struct {
	name    string
	lines   []string
	maxTpls int
}

var goldenCases = []goldenCase{
	{"crash panic", []string{
		"2026-11-20T10:00:01Z panic: runtime error: index out of range [5] with length 3",
		"2026-11-20T10:00:02Z panic: runtime error: index out of range [7] with length 3",
		"2026-11-20T10:00:09Z panic: runtime error: index out of range [2] with length 3",
		"goroutine 1 [running]:",
	}, 2},
	{"oom killed", []string{
		"Out of memory: Killed process 4121 (node) total-vm:2048000kB",
		"Out of memory: Killed process 4188 (node) total-vm:2051000kB",
		"container exited with code 137",
		"container exited with code 137",
	}, 2},
	{"port in use", []string{
		"Error: listen EADDRINUSE: address already in use 0.0.0.0:3000",
		"Error: listen EADDRINUSE: address already in use 0.0.0.0:3000",
		"Error: listen EADDRINUSE: address already in use :::8080",
	}, 1},
	{"bad env", []string{
		"2026-11-20T10:00:01Z level=fatal msg=missing required env DATABASE_URL",
		"2026-11-20T10:00:06Z level=fatal msg=missing required env DATABASE_URL",
		"2026-11-20T10:00:11Z level=fatal msg=missing required env DATABASE_URL",
	}, 1},
	{"failed healthcheck", []string{
		"healthcheck failed: GET http://10.0.0.5:8080/health returned 503 after 2000ms",
		"healthcheck failed: GET http://10.0.0.5:8080/health returned 503 after 2001ms",
		"healthcheck failed: GET http://10.0.0.5:8080/health returned 502 after 30000ms",
		"deploy rolled back: health check did not pass in 60s",
	}, 2},
	{"db refused", []string{
		"dial tcp 10.1.2.3:5432: connect: connection refused",
		"dial tcp 10.1.2.3:5432: connect: connection refused",
		"dial tcp 10.1.2.9:5432: connect: connection refused",
		"retrying in 2s attempt 1",
		"retrying in 4s attempt 2",
		"retrying in 8s attempt 3",
	}, 2},
	{"json crash", []string{
		`{"level":"error","msg":"unhandled exception","request_id":"3f2b7c1e-1111-4222-8333-444455556666"}`,
		`{"level":"error","msg":"unhandled exception","request_id":"9a2b7c1e-1111-4222-8333-444455556677"}`,
	}, 1},
	{"module missing", []string{
		"Error: Cannot find module 'express'",
		"Error: Cannot find module 'express'",
		"    at Module._resolveFilename (node:internal/modules/cjs/loader:1039:15)",
	}, 3},
	{"permission denied", []string{
		"open /data/app.db: permission denied",
		"open /data/app.db: permission denied",
		"open /data/app.db: permission denied",
	}, 1},
	{"crash loop restarts", []string{
		"Back-off restarting failed container app in pod web-1",
		"Back-off restarting failed container app in pod web-1",
		"Back-off restarting failed container app in pod web-1",
		"Back-off restarting failed container app in pod web-1",
	}, 1},
}

func TestMinerGoldenFailureLogs(t *testing.T) {
	for _, tc := range goldenCases {
		t.Run(tc.name, func(t *testing.T) {
			entries := Normalize(tc.lines, nil)
			m := NewMiner(MinerConfig{})
			m.AddAll(entries)
			tpls := m.Templates()
			if len(tpls) == 0 || len(tpls) > tc.maxTpls {
				t.Fatalf("templates = %d, want 1..%d: %+v", len(tpls), tc.maxTpls, tpls)
			}
			total := 0
			for _, tp := range tpls {
				total += tp.Count
				if tp.ID == "" || tp.Sample == "" {
					t.Fatalf("empty id or sample: %+v", tp)
				}
			}
			if total != len(entries) {
				t.Fatalf("counts add up to %d, want %d", total, len(entries))
			}
		})
	}
}

func TestMinerClusterCapKeepsCounts(t *testing.T) {
	m := NewMiner(MinerConfig{MaxClusters: 3})
	words := []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot", "golf"}
	for _, w := range words {
		m.Add(Entry{Template: w + " one two", Sample: w + " one two"})
	}
	tpls := m.Templates()
	if len(tpls) > 4 {
		t.Fatalf("templates = %d, want at most cap plus overflow", len(tpls))
	}
	total := 0
	for _, tp := range tpls {
		total += tp.Count
	}
	if total != len(words) {
		t.Fatalf("total = %d, want %d", total, len(words))
	}
}

func TestMinerSampleStaysMasked(t *testing.T) {
	m := NewMiner(MinerConfig{})
	m.AddAll(Normalize([]string{"login failed for 10.0.0.1"}, nil))
	for _, tp := range m.Templates() {
		if strings.Contains(tp.Template, "10.0.0.1") {
			t.Fatalf("template leaked a value: %s", tp.Template)
		}
	}
}
