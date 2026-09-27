package api

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploylog"
	"github.com/GLINCKER/levelrail/internal/store"
)

func readStreamEvent(t *testing.T, sc *bufio.Scanner) deploymentEvent {
	t.Helper()
	for sc.Scan() {
		if data, ok := strings.CutPrefix(sc.Text(), "data: "); ok {
			var ev deploymentEvent
			if err := json.Unmarshal([]byte(data), &ev); err != nil {
				t.Fatalf("decode event: %v", err)
			}
			return ev
		}
	}
	t.Fatalf("stream ended before an event: %v", sc.Err())
	return deploymentEvent{}
}

func TestDeploymentsStreamPreviewLookups(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	recorder := deploylog.NewRecorder(nil, discardLogger())
	rt.deployRecorder = recorder
	fp := newFakePreview(t)
	fp.addOK(t, "web", "dep_web")
	rt.preview = fp

	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "web:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: "dep_web", ServiceName: "web", Image: "web:1", Source: store.DeployAttemptSourceImage,
		Status: store.DeployAttemptStatusFailed, StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(rt.Handler())
	reqCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(func() {
		cancel()
		srv.Close()
	})
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, srv.URL+"/api/v1/deployments/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	sc := bufio.NewScanner(resp.Body)

	recorder.Start("dep_web")
	recorder.Step("dep_web", "build", "running")

	tests := []struct {
		name        string
		wantType    string
		wantPreview bool
		wantStep    bool
	}{
		{"created event carries the preview url", "created", true, false},
		{"step event skips the preview lookup", "step", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := readStreamEvent(t, sc)
			if ev.Type != tt.wantType {
				t.Fatalf("type = %q, want %q", ev.Type, tt.wantType)
			}
			if got := ev.Deployment.PreviewImageURL != nil; got != tt.wantPreview {
				t.Fatalf("preview set = %v, want %v", got, tt.wantPreview)
			}
			if got := ev.Step != nil; got != tt.wantStep {
				t.Fatalf("step set = %v, want %v", got, tt.wantStep)
			}
		})
	}
}
