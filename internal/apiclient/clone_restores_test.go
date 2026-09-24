package apiclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_CloneRestoresStatusAndFailed(t *testing.T) {
	var gotMethod, gotURI string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotURI = r.Method, r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"x1","status":"succeeded"}]`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "t")
	ctx := context.Background()

	tests := []struct {
		name    string
		call    func() error
		wantURI string
	}{
		{"db clone restores", func() error { _, err := c.ListCloneRestores(ctx, "db one"); return err }, "/api/v1/databases/db%20one/clone-restores"},
		{"volume clone restores", func() error { _, err := c.ListVolumeCloneRestores(ctx, "web", "data"); return err }, "/api/v1/apps/web/volumes/data/clone-restores"},
		{"database status", func() error { _, err := c.GetDatabaseStatus(ctx, "main"); return err }, "/api/v1/databases/main/status"},
		{"failed deploys", func() error { _, err := c.ListFailedDeploys(ctx, "6h"); return err }, "/api/v1/deploys/failed?since=6h"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err != nil {
				t.Fatalf("call: %v", err)
			}
			if gotMethod != http.MethodGet || gotURI != tt.wantURI {
				t.Errorf("request = %s %s, want GET %s", gotMethod, gotURI, tt.wantURI)
			}
		})
	}
}

func TestClient_StreamDeploySteps(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps/web/deploys/d1/steps" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(": connected\n\ndata: {\"step\":\"building\",\"status\":\"running\",\"timestamp\":\"t1\"}\n\ndata: not json\n\ndata: {\"step\":\"deploying\",\"status\":\"done\",\"timestamp\":\"t2\"}\n\n"))
	}))
	defer srv.Close()

	stop := errors.New("stop")
	var got []DeployStepEvent
	err := NewClient(srv.URL, "t").StreamDeploySteps(context.Background(), "web", "d1", func(ev DeployStepEvent) error {
		got = append(got, ev)
		if ev.Step == "deploying" {
			return stop
		}
		return nil
	})
	if !errors.Is(err, stop) {
		t.Fatalf("err = %v, want stop", err)
	}
	if len(got) != 2 || got[0].Step != "building" || got[1].Status != "done" {
		t.Errorf("events = %+v", got)
	}
}
