package bench

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestSchedule(t *testing.T) {
	tests := []struct {
		name string
		rate float64
		dur  time.Duration
		want int
		last time.Duration
	}{
		{"100rps 1s", 100, time.Second, 100, 990 * time.Millisecond},
		{"10rps 2s", 10, 2 * time.Second, 20, 1900 * time.Millisecond},
		{"zero rate", 0, time.Second, 0, 0},
		{"zero duration", 10, 0, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Schedule(tt.rate, tt.dur)
			if len(got) != tt.want {
				t.Fatalf("len = %d, want %d", len(got), tt.want)
			}
			if tt.want > 0 && got[len(got)-1] != tt.last {
				t.Errorf("last = %v, want %v", got[len(got)-1], tt.last)
			}
		})
	}
}

func TestPercentile(t *testing.T) {
	var s []time.Duration
	for i := 1; i <= 100; i++ {
		s = append(s, time.Duration(i)*time.Millisecond)
	}
	tests := []struct {
		p    float64
		want time.Duration
	}{
		{50, 50 * time.Millisecond},
		{95, 95 * time.Millisecond},
		{99, 99 * time.Millisecond},
		{100, 100 * time.Millisecond},
		{0.1, time.Millisecond},
	}
	for _, tt := range tests {
		if got := Percentile(s, tt.p); got != tt.want {
			t.Errorf("p%v = %v, want %v", tt.p, got, tt.want)
		}
	}
	if Percentile(nil, 50) != 0 {
		t.Error("empty input must give 0")
	}
}

func TestSummarizeErrorAccounting(t *testing.T) {
	samples := []Sample{
		{Latency: 10 * time.Millisecond},
		{Latency: 20 * time.Millisecond},
		{Latency: 30 * time.Millisecond, Failed: true},
		{Latency: 40 * time.Millisecond},
	}
	s := Summarize(samples, 2*time.Second)
	if s.Sent != 4 || s.Succeeded != 3 || s.Failed != 1 {
		t.Fatalf("counts = %+v", s)
	}
	if s.ErrorRate != 0.25 {
		t.Errorf("error rate = %v", s.ErrorRate)
	}
	if s.AchievedRPS != 1.5 {
		t.Errorf("rps = %v, want 1.5", s.AchievedRPS)
	}
	if s.MaxMS != 40 || s.P50MS != 20 {
		t.Errorf("latency = %+v", s)
	}
}

func TestAnalyzeEvent(t *testing.T) {
	at := 5 * time.Second
	mk := func(off time.Duration, failed bool) Sample { return Sample{Offset: off, Failed: failed} }
	tests := []struct {
		name          string
		samples       []Sample
		wantDropped   int
		wantWindow    float64
		wantRecovery  float64
		wantRecovered bool
	}{
		{"no failures", []Sample{mk(6*time.Second, false)}, 0, 0, 0, true},
		{"failure before event ignored", []Sample{mk(time.Second, true)}, 0, 0, 0, true},
		{"window", []Sample{mk(5100*time.Millisecond, true), mk(5600*time.Millisecond, true), mk(7*time.Second, false)}, 2, 500, 600, true},
		{"never recovered", []Sample{mk(5100*time.Millisecond, true), mk(9800*time.Millisecond, true)}, 2, 4700, 4800, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := AnalyzeEvent(tt.samples, at, 10*time.Second, time.Second)
			if r.Dropped != tt.wantDropped || r.ErrorWindowMS != tt.wantWindow || r.RecoveryMS != tt.wantRecovery || r.Recovered != tt.wantRecovered {
				t.Errorf("got %+v", r)
			}
		})
	}
}

func TestCompare(t *testing.T) {
	o := Compare(Stats{P50MS: 3, P95MS: 8, P99MS: 12}, Stats{P50MS: 1, P95MS: 5, P99MS: 7})
	if o.P50MS != 2 || o.P95MS != 3 || o.P99MS != 5 {
		t.Errorf("overhead = %+v", o)
	}
}

func TestRunCountsAndErrors(t *testing.T) {
	var n atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if n.Add(1)%4 == 0 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	res, err := Run(context.Background(), srv.Client(), Config{URL: srv.URL, Rate: 200, Duration: 500 * time.Millisecond, Timeout: time.Second}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Stats.Sent != 100 {
		t.Fatalf("sent = %d, want 100", res.Stats.Sent)
	}
	if res.Stats.Failed != 25 {
		t.Errorf("failed = %d, want 25", res.Stats.Failed)
	}
}

// A server that stalls must show its queueing delay: latency is measured from
// the intended send time, so requests scheduled during the stall report it.
func TestRunCoordinatedOmission(t *testing.T) {
	var first atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if first.CompareAndSwap(false, true) {
			time.Sleep(300 * time.Millisecond)
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	res, err := Run(context.Background(), srv.Client(), Config{URL: srv.URL, Rate: 100, Duration: 500 * time.Millisecond, Timeout: 2 * time.Second, MaxInFlight: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Stats.Saturated == 0 {
		t.Fatal("expected requests shed while the single slot was stalled")
	}
	if res.Stats.MaxMS < 250 {
		t.Errorf("max = %vms, want the stall reflected in latency", res.Stats.MaxMS)
	}
}

func TestRunEventDropsRequests(t *testing.T) {
	var down atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if down.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	ev := &Event{Name: "kill", At: 200 * time.Millisecond, Fire: func(context.Context) error {
		down.Store(true)
		time.AfterFunc(200*time.Millisecond, func() { down.Store(false) })
		return nil
	}}
	res, err := Run(context.Background(), srv.Client(), Config{URL: srv.URL, Rate: 100, Duration: 1500 * time.Millisecond, Timeout: time.Second}, ev)
	if err != nil {
		t.Fatal(err)
	}
	if res.Event == nil || res.Event.Name != "kill" {
		t.Fatalf("event = %+v", res.Event)
	}
	if res.Event.Dropped < 10 || res.Event.ErrorWindowMS < 100 {
		t.Errorf("event = %+v", res.Event)
	}
	if !res.Event.Recovered {
		t.Errorf("should have recovered: %+v", res.Event)
	}
}

func TestRunValidation(t *testing.T) {
	if _, err := Run(context.Background(), http.DefaultClient, Config{Rate: 1, Duration: time.Second}, nil); err == nil {
		t.Error("empty URL must fail")
	}
	if _, err := Run(context.Background(), http.DefaultClient, Config{URL: "http://x", Duration: time.Second}, nil); err == nil {
		t.Error("zero rate must fail")
	}
}
