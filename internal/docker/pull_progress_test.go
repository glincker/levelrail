package docker

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func collect(t *testing.T, stream string, cfg PullConfig, clock func() time.Time) ([]PullEvent, *PullTracker, error) {
	t.Helper()
	var evs []PullEvent
	p := NewPullTracker("img:1", cfg, func(e PullEvent) { evs = append(evs, e) })
	if clock != nil {
		p.now = clock
	}
	err := p.Consume(strings.NewReader(stream))
	return evs, p, err
}

func count(evs []PullEvent, phase PullPhase) int {
	n := 0
	for _, e := range evs {
		if e.Phase == phase {
			n++
		}
	}
	return n
}

func TestPullTracker_Streams(t *testing.T) {
	cfg := PullConfig{Interval: time.Hour, MinPercent: 10, MaxLayerLine: 100}
	tests := []struct {
		name      string
		stream    string
		wantErr   string
		check     func(t *testing.T, evs []PullEvent, p *PullTracker)
		malformed int
	}{
		{
			name:   "empty stream reports start and a done with no layers",
			stream: "",
			check: func(t *testing.T, evs []PullEvent, _ *PullTracker) {
				if len(evs) != 2 || evs[0].Phase != PullPhaseStart || evs[1].Phase != PullPhaseDone {
					t.Fatalf("events = %+v, want start then done", evs)
				}
				if evs[1].Layers != 0 || evs[1].Percent != 100 {
					t.Errorf("done = %+v, want 0 layers at 100%%", evs[1])
				}
			},
		},
		{
			name: "malformed lines are counted and skipped",
			stream: `not json
{"status":"Pulling fs layer","id":"a"}
{broken
{"status":"Downloading","id":"a","progressDetail":{"current":50,"total":100}}
`,
			malformed: 2,
			check: func(t *testing.T, evs []PullEvent, _ *PullTracker) {
				last := evs[len(evs)-1]
				if last.Phase != PullPhaseDone || last.Layers != 1 {
					t.Errorf("last = %+v, want done with the one good layer", last)
				}
			},
		},
		{
			name: "full pull with digest and status",
			stream: `{"status":"Pulling from library/x","id":"1.0"}
{"status":"Pulling fs layer","id":"a"}
{"status":"Pulling fs layer","id":"b"}
{"status":"Already exists","id":"c"}
{"status":"Downloading","id":"a","progressDetail":{"current":100,"total":100}}
{"status":"Download complete","id":"a"}
{"status":"Extracting","id":"a","progressDetail":{"current":10,"total":100}}
{"status":"Pull complete","id":"a"}
{"status":"Downloading","id":"b","progressDetail":{"current":200,"total":200}}
{"status":"Pull complete","id":"b"}
{"status":"Digest: sha256:abc"}
{"status":"Status: Downloaded newer image for img:1"}
`,
			check: func(t *testing.T, evs []PullEvent, _ *PullTracker) {
				done := evs[len(evs)-1]
				if done.Digest != "sha256:abc" || !strings.HasPrefix(done.Status, "Downloaded newer") {
					t.Errorf("done = %+v, want digest and status", done)
				}
				if done.LayersDone != 3 || done.Existing != 1 || done.BytesTotal != 300 || done.BytesCurrent != 300 {
					t.Errorf("done = %+v, want 3 layers done, 1 existing, 300/300 bytes", done)
				}
				var states []string
				for _, e := range evs {
					if e.Phase == PullPhaseLayer {
						states = append(states, e.LayerID+":"+e.LayerState)
					}
				}
				want := "c:already exists a:extracting a:complete b:complete"
				if got := strings.Join(states, " "); got != want {
					t.Errorf("layer events = %q, want %q", got, want)
				}
			},
		},
		{
			name: "daemon error mid layer is returned verbatim",
			stream: `{"status":"Downloading","id":"a","progressDetail":{"current":1,"total":100}}
{"errorDetail":{"message":"unexpected EOF"},"error":"unexpected EOF"}
`,
			wantErr: "unexpected EOF",
		},
		{
			name:    "auth denied keeps the registry text",
			stream:  `{"errorDetail":{"message":"pull access denied for x, repository does not exist or may require 'docker login'"},"error":"pull access denied for x, repository does not exist or may require 'docker login'"}` + "\n",
			wantErr: "pull access denied",
		},
		{
			name:   "final line without a newline is still read",
			stream: `{"status":"Digest: sha256:tail"}`,
			check: func(t *testing.T, evs []PullEvent, _ *PullTracker) {
				if evs[len(evs)-1].Digest != "sha256:tail" {
					t.Errorf("done = %+v, want the unterminated last line parsed", evs[len(evs)-1])
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evs, p, err := collect(t, tt.stream, cfg, nil)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Consume() error = %v", err)
			}
			if p.Malformed() != tt.malformed {
				t.Errorf("malformed = %d, want %d", p.Malformed(), tt.malformed)
			}
			tt.check(t, evs, p)
		})
	}
}

func TestPullTracker_ThrottlesProgress(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"status":"Pulling fs layer","id":"a"}` + "\n")
	for i := 1; i <= 1000; i++ {
		fmt.Fprintf(&b, `{"status":"Downloading","id":"a","progressDetail":{"current":%d,"total":1000}}`+"\n", i)
	}
	tests := []struct {
		name         string
		cfg          PullConfig
		maxProgress  int
		minProgress  int
		tickPerEvent time.Duration
	}{
		{"percent step only", PullConfig{Interval: time.Hour, MinPercent: 10, MaxLayerLine: 5}, 12, 9, 0},
		{"coarse step is rarer", PullConfig{Interval: time.Hour, MinPercent: 50, MaxLayerLine: 5}, 3, 1, 0},
		{"time based reports even without a percent step", PullConfig{Interval: time.Second, MinPercent: 100, MaxLayerLine: 5}, 1001, 100, 100 * time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := time.Unix(0, 0)
			clock := func() time.Time { now = now.Add(tt.tickPerEvent); return now }
			evs, _, err := collect(t, b.String(), tt.cfg, clock)
			if err != nil {
				t.Fatalf("Consume() error = %v", err)
			}
			got := count(evs, PullPhaseProgress)
			if got > tt.maxProgress || got < tt.minProgress {
				t.Errorf("progress events = %d, want %d..%d out of 1000 updates", got, tt.minProgress, tt.maxProgress)
			}
		})
	}
}

func TestPullTracker_HugeLayerCountIsBounded(t *testing.T) {
	const layers = 5000
	var b strings.Builder
	for i := 0; i < layers; i++ {
		fmt.Fprintf(&b, `{"status":"Pulling fs layer","id":"l%d"}`+"\n", i)
		fmt.Fprintf(&b, `{"status":"Downloading","id":"l%d","progressDetail":{"current":5,"total":10}}`+"\n", i)
		fmt.Fprintf(&b, `{"status":"Pull complete","id":"l%d"}`+"\n", i)
	}
	cfg := PullConfig{Interval: time.Hour, MinPercent: 10, MaxLayerLine: 12}
	evs, _, err := collect(t, b.String(), cfg, nil)
	if err != nil {
		t.Fatalf("Consume() error = %v", err)
	}
	if n := count(evs, PullPhaseLayer); n != 12 {
		t.Errorf("layer events = %d, want capped at 12", n)
	}
	if n := len(evs); n > 200 {
		t.Errorf("total events = %d for %d layers, want a bounded log", n, layers)
	}
	done := evs[len(evs)-1]
	if done.Layers != layers || done.LayersDone != layers {
		t.Errorf("done = %+v, want all %d layers aggregated", done, layers)
	}
}

func TestPullConfigFromEnv(t *testing.T) {
	env := map[string]string{EnvPullInterval: "500ms", EnvPullMinPercent: "25", EnvPullMaxLayerLine: "3"}
	cfg := PullConfigFromEnv(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	if cfg.Interval != 500*time.Millisecond || cfg.MinPercent != 25 || cfg.MaxLayerLine != 3 {
		t.Errorf("cfg = %+v, want env values", cfg)
	}
	bad := map[string]string{EnvPullInterval: "soon", EnvPullMinPercent: "200", EnvPullMaxLayerLine: "-1"}
	cfg = PullConfigFromEnv(func(k string) (string, bool) { v, ok := bad[k]; return v, ok })
	if cfg.Interval != 2*time.Second || cfg.MinPercent != 10 || cfg.MaxLayerLine != 12 {
		t.Errorf("cfg = %+v, want defaults for invalid values", cfg)
	}
}

func TestNormalizeArch(t *testing.T) {
	for in, want := range map[string]string{"x86_64": "amd64", "aarch64": "arm64", "arm64": "arm64", "armv7l": "arm", "riscv64": "riscv64"} {
		if got := NormalizeArch(in); got != want {
			t.Errorf("NormalizeArch(%q) = %q, want %q", in, got, want)
		}
	}
}
