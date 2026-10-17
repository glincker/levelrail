package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func ms(v int64) *int64 { return &v }

func TestWakeTimingText(t *testing.T) {
	tests := []struct {
		name  string
		in    apiclient.WakeTiming
		text  string
		short string
	}{
		{"no history says so", apiclient.WakeTiming{}, wakeTimingNone, "-"},
		{"samples without a median is still no numbers", apiclient.WakeTiming{Samples: 2}, wakeTimingNone, "-"},
		{"seconds", apiclient.WakeTiming{Samples: 5, ReadyP50Ms: ms(4200), ReadyP95Ms: ms(9100), ReadyLastMs: ms(3900)}, "usually wakes in about 4.2 s (p95 9.1 s), last 3.9 s, from 5 wakes", "4.2 s (p95 9.1 s)"},
		{"milliseconds", apiclient.WakeTiming{Samples: 1, ReadyP50Ms: ms(350), ReadyP95Ms: ms(350)}, "usually wakes in about 350 ms (p95 350 ms), from 1 wake", "350 ms (p95 350 ms)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := wakeTimingText(tc.in); got != tc.text {
				t.Errorf("text = %q, want %q", got, tc.text)
			}
			if got := wakeTimingShort(tc.in); got != tc.short {
				t.Errorf("short = %q, want %q", got, tc.short)
			}
		})
	}
}

func TestRun_AppsSleepStatusAndListShowWakeTime(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/sleep":
			_, _ = w.Write([]byte(`{"enabled":2,"sleeping":1,"reclaimed_bytes":0,"apps":[` +
				`{"name":"web","idle_minutes":30,"sleeping":true,"memory_limit_bytes":0,"wake_timing":{"samples":4,"ready_p50_ms":4200,"ready_p95_ms":9000}},` +
				`{"name":"api","idle_minutes":30,"sleeping":false,"memory_limit_bytes":0,"wake_timing":{"samples":0}}]}`))
		case "/api/v1/apps/web/sleep/history":
			_, _ = w.Write([]byte(`{"enabled":true,"events":[],"stats":{}}`))
		default:
			_, _ = w.Write([]byte(`{"enabled":true,"idle_minutes":30,"eligible":true,"wake_timing":{"samples":4,"ready_p50_ms":4200,"ready_p95_ms":9000,"ready_last_ms":3000}}`))
		}
	}))
	t.Cleanup(srv.Close)
	for _, tc := range []struct {
		args []string
		want []string
	}{
		{[]string{"status", "web"}, []string{"wake time: usually wakes in about 4.2 s (p95 9.0 s), last 3.0 s, from 4 wakes"}},
		{[]string{"list"}, []string{"USUALLY WAKES", "4.2 s (p95 9.0 s)"}},
	} {
		var stdout, stderr bytes.Buffer
		args := append([]string{"apps", "sleep"}, tc.args...)
		args = append(args, "--token", "t", "--api-url", srv.URL)
		if got := run("levelrail-cli-test", args, &stdout, &stderr, envMap()); got != exitOK {
			t.Fatalf("%v: exit = %d (stderr=%q)", tc.args, got, stderr.String())
		}
		for _, w := range tc.want {
			if !strings.Contains(stdout.String(), w) {
				t.Errorf("%v: output missing %q:\n%s", tc.args, w, stdout.String())
			}
		}
	}
}
