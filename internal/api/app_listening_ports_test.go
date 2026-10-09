package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestListenVerdict(t *testing.T) {
	tests := []struct {
		name      string
		port      int
		listening []int
		want      string
	}{
		{"listens on configured port", 3000, []int{22, 3000}, listenVerdictListening},
		{"listens elsewhere", 3000, []int{8080}, listenVerdictOtherPort},
		{"nothing listening", 3000, nil, listenVerdictNotListening},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := listenVerdict(tt.port, tt.listening); got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestHandleAppListeningPorts_UnprobedWithoutExec(t *testing.T) {
	rt, db, cookie := newTimelineRouter(t)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	var got listeningPortsResource
	if code := tlJSON(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/listening-ports", "", &got); code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if got.Probed || got.Verdict != listenVerdictUnknown || got.Port != 80 || got.Reason == "" {
		t.Fatalf("got %+v", got)
	}
	if code := tlJSON(t, rt, cookie, http.MethodGet, "/api/v1/apps/nope/listening-ports", "", nil); code != http.StatusNotFound {
		t.Fatalf("missing app = %d, want 404", code)
	}
}

const procNetTCPListen3000 = `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000:0BB8 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 12345 1 0000000000000000 100 0 0 10 0
`

func TestHandleAppListeningPorts_ProbeOutcomes(t *testing.T) {
	cases := []struct {
		name        string
		fake        *fakeExecAppRuntime
		wantProbed  bool
		wantVerdict string
		wantReason  string
	}{
		{
			name: "image without cat is unknown, not not_listening",
			fake: &fakeExecAppRuntime{
				inspectState: &docker.ContainerState{ID: "c1", Name: "web", Running: true},
				execErr:      errors.New("executable file not found"),
			},
			wantVerdict: listenVerdictUnknown,
			wantReason:  listenReasonProbeFailed,
		},
		{
			name: "readable table with the port is listening",
			fake: &fakeExecAppRuntime{
				inspectState: &docker.ContainerState{ID: "c1", Name: "web", Running: true},
				execReader:   io.NopCloser(strings.NewReader(procNetTCPListen3000)),
			},
			wantProbed:  true,
			wantVerdict: listenVerdictListening,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt, db := newTestRouterWithExecRuntime(t, tc.fake)
			cookie := loginTestSession(t, rt, db)
			svc := store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000, ExecEnabled: true}
			if err := db.SaveDesiredService(context.Background(), svc); err != nil {
				t.Fatal(err)
			}
			var got listeningPortsResource
			if code := tlJSON(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/listening-ports", "", &got); code != http.StatusOK {
				t.Fatalf("status = %d", code)
			}
			if got.Probed != tc.wantProbed || got.Verdict != tc.wantVerdict || got.Reason != tc.wantReason {
				t.Fatalf("got probed=%v verdict=%q reason=%q, want probed=%v verdict=%q reason=%q",
					got.Probed, got.Verdict, got.Reason, tc.wantProbed, tc.wantVerdict, tc.wantReason)
			}
		})
	}
}
