package api

import (
	"context"
	"net/http"
	"testing"

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
