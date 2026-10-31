package footprint

import (
	"context"
	"errors"
	"runtime/debug"
	"testing"
	"time"
)

func TestApplyGCPercent(t *testing.T) {
	old := debug.SetGCPercent(100)
	defer debug.SetGCPercent(old)
	cases := []struct {
		name string
		env  map[string]string
		want int
	}{
		{"default", nil, DefaultGCPercent},
		{"override", map[string]string{EnvGCPercent: "80"}, 80},
		{"bad value falls back", map[string]string{EnvGCPercent: "-3"}, DefaultGCPercent},
		{"GOGC wins", map[string]string{"GOGC": "200"}, 0},
	}
	for _, c := range cases {
		got := ApplyGCPercent(func(k string) string { return c.env[k] })
		if got != c.want {
			t.Errorf("%s: got %d want %d", c.name, got, c.want)
		}
	}
}

func TestLimitFor(t *testing.T) {
	cases := []struct {
		name  string
		total int64
		pct   int
		want  int64
	}{
		{"eight gig 80", 8 << 30, 80, (8 << 30) / 100 * 80},
		{"disabled", 8 << 30, 0, 0},
		{"too small", 100 << 20, 80, 0},
		{"unknown", 0, 80, 0},
	}
	for _, c := range cases {
		if got := LimitFor(c.total, c.pct); got != c.want {
			t.Errorf("%s: got %d want %d", c.name, got, c.want)
		}
	}
}

func TestPercent(t *testing.T) {
	cases := map[string]int{"": 80, "60": 60, "0": 0, "99": 80, "x": 80, "-1": 80}
	for in, want := range cases {
		if got := Percent(func(string) string { return in }); got != want {
			t.Errorf("%q: got %d want %d", in, got, want)
		}
	}
}

func TestParseMemTotal(t *testing.T) {
	if got := parseMemTotal("MemFree: 1 kB\nMemTotal:  8192 kB\n"); got != 8192<<10 {
		t.Fatalf("got %d", got)
	}
	if parseMemTotal("none") != 0 {
		t.Fatal("expected 0")
	}
}

func TestEveryRecordsAndStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = Every(ctx, "test-loop", 10*time.Millisecond, nil, func(context.Context) error { return errors.New("boom") })
		close(done)
	}()
	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done
	for _, l := range Loops() {
		if l.Name != "test-loop" {
			continue
		}
		if l.Runs == 0 || l.Errors == 0 || l.Running || l.LastError != "boom" {
			t.Fatalf("unexpected info %+v", l)
		}
		return
	}
	t.Fatal("loop not registered")
}

func TestReadProc(t *testing.T) {
	p := ReadProc()
	if p.Goroutines <= 0 || p.RSSBytes == 0 {
		t.Fatalf("bad proc %+v", p)
	}
}

func TestTrimOnceAfterBootDisabledReturnsAtOnce(t *testing.T) {
	done := make(chan struct{})
	go func() {
		trimOnceAfterBoot(context.Background(), func(k string) string {
			if k == EnvTrimBootDelay {
				return "0"
			}
			return ""
		})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("disabled boot trim should return immediately")
	}
}
