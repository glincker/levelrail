package scheduledtask

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeTarget struct {
	base  string
	err   error
	calls atomic.Int32
}

func (f *fakeTarget) Prepare(context.Context, *store.DesiredService) (string, error) {
	f.calls.Add(1)
	return f.base, f.err
}

type fakeHTTPConfigs struct {
	cfg store.ScheduledTaskHTTP
	err error
}

func (f fakeHTTPConfigs) GetScheduledTaskHTTP(context.Context, string) (store.ScheduledTaskHTTP, error) {
	return f.cfg, f.err
}

func httpRunner(target HTTPTarget, cfg store.ScheduledTaskHTTP) *Runner {
	return &Runner{
		Apps:        &fakeAppStore{svc: store.DesiredService{Name: "web"}},
		Runs:        &fakeRunStore{},
		Target:      target,
		HTTPConfigs: fakeHTTPConfigs{cfg: cfg},
		Sleep:       func(context.Context, time.Duration) error { return nil },
		Limits:      DefaultLimits(),
	}
}

func TestRunner_HTTPAttempt(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.HandlerFunc
		cfg        store.ScheduledTaskHTTP
		wakeErr    error
		timeout    time.Duration
		wantStatus string
		wantCode   int
		wantOut    string
	}{
		{
			name: "2xx is success and records the response code",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/cron/tick" || r.Header.Get(HeaderJobRun) != "run1" {
					http.Error(w, "bad request shape", http.StatusTeapot)
					return
				}
				w.WriteHeader(http.StatusAccepted)
				_, _ = w.Write([]byte("queued"))
			},
			cfg:        store.ScheduledTaskHTTP{Method: "POST", Path: "/cron/tick"},
			wantStatus: store.ScheduledTaskStatusSuccess, wantCode: 202, wantOut: "queued",
		},
		{
			name: "half success: non-2xx is failed but keeps code and body",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte("db down"))
			},
			cfg:        store.ScheduledTaskHTTP{Method: "GET", Path: "/cron/report"},
			wantStatus: store.ScheduledTaskStatusFailed, wantCode: 500, wantOut: "db down",
		},
		{
			name:       "wake failure is container_not_running",
			handler:    func(http.ResponseWriter, *http.Request) {},
			cfg:        store.ScheduledTaskHTTP{Path: "/x"},
			wakeErr:    errors.New("wake blocked"),
			wantStatus: store.ScheduledTaskStatusContainerNotRunning, wantOut: "wake blocked",
		},
		{
			name: "slow app times out",
			handler: func(_ http.ResponseWriter, r *http.Request) {
				select {
				case <-r.Context().Done():
				case <-time.After(2 * time.Second):
				}
			},
			cfg: store.ScheduledTaskHTTP{Path: "/slow"}, timeout: 50 * time.Millisecond,
			wantStatus: store.ScheduledTaskStatusTimeout,
		},
		{
			name:       "protocol relative path is rejected",
			handler:    func(http.ResponseWriter, *http.Request) {},
			cfg:        store.ScheduledTaskHTTP{Path: "//evil.example/x"},
			wantStatus: store.ScheduledTaskStatusFailed, wantOut: "must start with a single /",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()
			r := httpRunner(&fakeTarget{base: srv.URL, err: tc.wakeErr}, tc.cfg)
			r.Timeout = tc.timeout
			task := store.ScheduledTask{ID: "t1", ServiceName: "web", Name: "tick", Mode: store.ScheduledTaskModeHTTP}

			res := r.attempt(context.Background(), task, "run1")
			if res.status != tc.wantStatus {
				t.Fatalf("status = %q (%s), want %q", res.status, res.output, tc.wantStatus)
			}
			if tc.wantCode != 0 && (res.exitCode == nil || *res.exitCode != tc.wantCode) {
				t.Errorf("exit code = %v, want %d", res.exitCode, tc.wantCode)
			}
			if !strings.Contains(res.output, tc.wantOut) {
				t.Errorf("output = %q, want it to contain %q", res.output, tc.wantOut)
			}
		})
	}
}

func TestRunner_HTTPRetriesWithBackoffThenSucceeds(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := httpRunner(&fakeTarget{base: srv.URL}, store.ScheduledTaskHTTP{Path: "/x"})
	var sleeps []time.Duration
	r.Sleep = func(_ context.Context, d time.Duration) error { sleeps = append(sleeps, d); return nil }
	task := store.ScheduledTask{ID: "t1", ServiceName: "web", Mode: store.ScheduledTaskModeHTTP, MaxRetries: 3, RetryBackoffSeconds: 2}

	res := r.execute(context.Background(), task, "run1")
	if res.status != store.ScheduledTaskStatusSuccess || res.attempts != 3 {
		t.Fatalf("status %q attempts %d, want success after 3", res.status, res.attempts)
	}
	if len(sleeps) != 2 || sleeps[1] <= sleeps[0] {
		t.Errorf("backoff sleeps = %v, want two growing waits", sleeps)
	}
}

func TestRunner_HTTPBackgroundTimeout(t *testing.T) {
	r := httpRunner(&fakeTarget{}, store.ScheduledTaskHTTP{})
	r.Limits.BackgroundTimeout = 15 * time.Minute
	r.Limits.DefaultTimeout = time.Minute
	tests := []struct {
		name string
		task store.ScheduledTask
		cfg  store.ScheduledTaskHTTP
		want time.Duration
	}{
		{"foreground uses default", store.ScheduledTask{}, store.ScheduledTaskHTTP{}, time.Minute},
		{"background uses the longer timeout", store.ScheduledTask{}, store.ScheduledTaskHTTP{Background: true}, 15 * time.Minute},
		{"explicit job timeout wins", store.ScheduledTask{TimeoutSeconds: 30}, store.ScheduledTaskHTTP{Background: true}, 30 * time.Second},
	}
	for _, tc := range tests {
		if got := r.httpTimeout(tc.task, tc.cfg); got != tc.want {
			t.Errorf("%s: timeout = %s, want %s", tc.name, got, tc.want)
		}
	}
}

func TestRunner_QueuePolicySerializesOverlap(t *testing.T) {
	r := &Runner{}
	ctx := context.Background()
	_, cleanup1, skip := r.beginRun(ctx, "t1", store.ScheduledTaskConcurrencyQueue)
	if skip {
		t.Fatal("first queued run skipped")
	}

	started := make(chan struct{})
	go func() {
		_, cleanup2, _ := r.beginRun(ctx, "t1", store.ScheduledTaskConcurrencyQueue)
		close(started)
		cleanup2()
	}()
	select {
	case <-started:
		t.Fatal("queued run started while the first was still in flight")
	case <-time.After(50 * time.Millisecond):
	}
	cleanup1()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("queued run never started after the first finished")
	}
}

func TestJoinTarget(t *testing.T) {
	tests := []struct {
		base, path, want string
		wantErr          bool
	}{
		{"http://127.0.0.1:8080", "/a/b?x=1", "http://127.0.0.1:8080/a/b?x=1", false},
		{"http://127.0.0.1:8080/", "/", "http://127.0.0.1:8080/", false},
		{"http://127.0.0.1:8080", "a", "", true},
		{"http://127.0.0.1:8080", "//evil.example", "", true},
		{"http://127.0.0.1:8080", "/\\evil", "http://127.0.0.1:8080/%5Cevil", false},
	}
	for _, tc := range tests {
		got, err := joinTarget(tc.base, tc.path)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("joinTarget(%q, %q) = %q, %v; want %q, err %v", tc.base, tc.path, got, err, tc.want, tc.wantErr)
		}
	}
}
