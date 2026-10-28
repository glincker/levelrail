package deployurls

import (
	"context"
	"io"
	"log/slog"
	"strconv"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeStore struct {
	services []store.DesiredService
	attempts map[string][]store.DeployAttempt
	pins     map[string][]store.RollbackPin
	rows     map[string]store.DeployURL
	sleep    map[string]int
	hold     map[string]bool
}

func newFake() *fakeStore {
	return &fakeStore{
		attempts: map[string][]store.DeployAttempt{},
		pins:     map[string][]store.RollbackPin{},
		rows:     map[string]store.DeployURL{},
		sleep:    map[string]int{},
		hold:     map[string]bool{},
	}
}

func (f *fakeStore) ListDesiredServices(context.Context) ([]store.DesiredService, error) {
	return append([]store.DesiredService(nil), f.services...), nil
}

func (f *fakeStore) SaveDesiredService(_ context.Context, svc store.DesiredService) error {
	for i := range f.services {
		if f.services[i].Name == svc.Name {
			f.services[i] = svc
			return nil
		}
	}
	f.services = append(f.services, svc)
	return nil
}

func (f *fakeStore) ListDeployAttempts(_ context.Context, name string) ([]store.DeployAttempt, error) {
	return f.attempts[name], nil
}

func (f *fakeStore) ListRollbackPins(_ context.Context, name string) ([]store.RollbackPin, error) {
	return f.pins[name], nil
}

func (f *fakeStore) SetAppSleepIdle(_ context.Context, name string, minutes int, _ time.Time) error {
	f.sleep[name] = minutes
	return nil
}

func (f *fakeStore) SetAppSleepHold(_ context.Context, name string, hold bool, _ time.Time) error {
	f.hold[name] = hold
	return nil
}

func (f *fakeStore) ListDeployURLs(_ context.Context, app string) ([]store.DeployURL, error) {
	var out []store.DeployURL
	for _, r := range f.rows {
		if app == "" || r.AppName == app {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeStore) SaveDeployURL(_ context.Context, u store.DeployURL) error {
	f.rows[u.ServiceName] = u
	return nil
}

func (f *fakeStore) DeleteDeployURL(_ context.Context, name string) error {
	delete(f.rows, name)
	return nil
}

func (f *fakeStore) remove(_ context.Context, name string) error {
	for i := range f.services {
		if f.services[i].Name == name {
			f.services = append(f.services[:i], f.services[i+1:]...)
			break
		}
	}
	return nil
}

var t0 = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

func attempt(i int, status, sha string) store.DeployAttempt {
	return store.DeployAttempt{
		ID: "da_" + sha + strconv.Itoa(i), ServiceName: "web", Image: "web:" + sha, CommitSHA: sha,
		Status: status, StartedAt: t0.Add(time.Duration(i) * time.Hour),
		ImageDigest: "sha256:" + sha, DigestReason: store.DigestReasonResolved,
	}
}

func TestPlan(t *testing.T) {
	app := store.DesiredService{Name: "web", Image: "web:ccccccc"}
	attempts := []store.DeployAttempt{
		attempt(1, store.DeployAttemptStatusSucceeded, "aaaaaaa"),
		attempt(2, store.DeployAttemptStatusFailed, "bbbbbbb"),
		attempt(3, store.DeployAttemptStatusSucceeded, "ccccccc"),
		attempt(4, store.DeployAttemptStatusSucceeded, "ccccccc"),
		attempt(5, store.DeployAttemptStatusSucceeded, "ddddddd"),
	}
	unsafe := attempt(6, store.DeployAttemptStatusSucceeded, "eeeeeee")
	unsafe.ImageDigest, unsafe.DigestReason = "", ""

	tests := []struct {
		name     string
		attempts []store.DeployAttempt
		keep     int
		want     []string
	}{
		{"newest first, failures skipped, same commit deduped", attempts, 3, []string{"web-ddddddd", "web-ccccccc", "web-aaaaaaa"}},
		{"cap applies", attempts, 1, []string{"web-ddddddd"}},
		{"disabled", attempts, 0, nil},
		{"content that cannot be redeployed is skipped", append([]store.DeployAttempt{unsafe}, attempts[:1]...), 3, []string{"web-aaaaaaa"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, w := range Plan(app, tc.attempts, nil, tc.keep) {
				got = append(got, w.ServiceName)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestPlanMarksLive(t *testing.T) {
	app := store.DesiredService{Name: "web", Image: "web:ddddddd"}
	got := Plan(app, []store.DeployAttempt{attempt(5, store.DeployAttemptStatusSucceeded, "ddddddd")}, nil, 3)
	if len(got) != 1 || !got[0].Live {
		t.Fatalf("plan = %+v, want the live attempt flagged", got)
	}
}

func TestSyncRetention(t *testing.T) {
	f := newFake()
	f.services = []store.DesiredService{{Name: "web", Image: "web:ccccccc", Port: 80, Volumes: []store.ServiceVolume{{Name: "data"}}}}
	f.attempts["web"] = []store.DeployAttempt{
		attempt(1, store.DeployAttemptStatusSucceeded, "aaaaaaa"),
		attempt(2, store.DeployAttemptStatusSucceeded, "bbbbbbb"),
		attempt(3, store.DeployAttemptStatusSucceeded, "ccccccc"),
	}
	nudged := 0
	s := &Syncer{
		Store: f, Config: Config{Keep: 3, IdleMinutes: 5, Interval: time.Minute},
		HostFor: func(_ context.Context, name string) string { return name + ".apps.example.com" },
		Remove:  f.remove, Nudge: func() { nudged++ }, Now: func() time.Time { return t0 },
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	ctx := context.Background()

	if err := s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if len(f.rows) != 2 || f.rows["web-aaaaaaa"].Host != "web-aaaaaaa.apps.example.com" || f.rows["web-ccccccc"].AppName != "" {
		t.Fatalf("rows = %+v, want old deploys only, the live one has no retained service", f.rows)
	}
	var clone store.DesiredService
	for _, svc := range f.services {
		if svc.Name == "web-aaaaaaa" {
			clone = svc
		}
	}
	if clone.Image != "web:aaaaaaa@sha256:aaaaaaa" || len(clone.Volumes) != 0 || clone.Replicas != 1 || clone.Domains[0] != "web-aaaaaaa.apps.example.com" {
		t.Fatalf("clone = %+v", clone)
	}
	if f.sleep["web-aaaaaaa"] != 5 || !f.hold["web-aaaaaaa"] {
		t.Fatalf("sleep = %v hold = %v, want idle 5 with held requests", f.sleep, f.hold)
	}
	if nudged != 1 {
		t.Fatalf("nudged = %d, want 1", nudged)
	}

	if err := s.Sync(ctx); err != nil || nudged != 1 || len(f.services) != 3 {
		t.Fatalf("second pass must change nothing: err=%v nudged=%d services=%d", err, nudged, len(f.services))
	}

	f.attempts["web"] = append(f.attempts["web"], attempt(4, store.DeployAttemptStatusSucceeded, "ddddddd"))
	f.services[0].Image = "web:ddddddd"
	s.Config.Keep = 2
	if err := s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.rows["web-aaaaaaa"]; ok {
		t.Fatalf("rows = %+v, the oldest deploy must fall out of the window", f.rows)
	}
	if _, ok := f.rows["web-ccccccc"]; !ok {
		t.Fatalf("rows = %+v, the superseded deploy must get its URL", f.rows)
	}

	f.services = f.services[:1]
	s.Config.Keep = 0
	if err := s.Sync(ctx); err != nil || len(f.rows) != 0 {
		t.Fatalf("keep 0 must clear every row: err=%v rows=%+v", err, f.rows)
	}
}

func TestConfigFromEnv(t *testing.T) {
	get := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	tests := []struct {
		name string
		env  map[string]string
		want Config
	}{
		{"defaults", nil, Config{Keep: 3, IdleMinutes: 5, Interval: time.Minute}},
		{"keep zero disables", map[string]string{EnvKeep: "0"}, Config{Keep: 0, IdleMinutes: 5, Interval: time.Minute}},
		{"malformed falls back", map[string]string{EnvKeep: "x", EnvIdleMinutes: "-1", EnvInterval: "soon"}, Config{Keep: 3, IdleMinutes: 5, Interval: time.Minute}},
		{"custom", map[string]string{EnvKeep: "5", EnvIdleMinutes: "9", EnvInterval: "30s"}, Config{Keep: 5, IdleMinutes: 9, Interval: 30 * time.Second}},
	}
	for _, tc := range tests {
		if got := ConfigFromEnv(get(tc.env)); got != tc.want {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestCloneDropsHostBindings(t *testing.T) {
	hp := 8080
	app := store.DesiredService{Name: "vpn", Image: "vpn:ddddddd", HostPort: &hp,
		PublishedPorts: []store.PublishedPort{{ContainerPort: 51820, HostPort: 51820, Protocol: "udp"}}}
	w := Plan(app, []store.DeployAttempt{attempt(5, store.DeployAttemptStatusSucceeded, "ddddddd")}, nil, 3)
	if len(w) != 1 {
		t.Fatalf("plan = %+v, want one retained deploy", w)
	}
	svc := Clone(app, w[0], "d5.example.test")
	if svc.HostPort != nil || len(svc.PublishedPorts) != 0 {
		t.Fatalf("clone host_port = %v, published_ports = %+v, want neither", svc.HostPort, svc.PublishedPorts)
	}
}
