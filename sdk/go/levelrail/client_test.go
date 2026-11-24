package levelrail

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Core methods name operation ids by string, so the generated table must hold them with the expected shape.
func TestCoreOperationsExist(t *testing.T) {
	want := map[string][2]string{
		"handleListApps":         {"GET", "/api/v1/apps"},
		"handleGetApp":           {"GET", "/api/v1/apps/{name}"},
		"handleCreateApp":        {"POST", "/api/v1/apps"},
		"handleTriggerDeploy":    {"POST", "/api/v1/apps/{name}/deploys"},
		"handleRollbackToDeploy": {"POST", "/api/v1/apps/{name}/deploys/{deployId}/rollback"},
		"handleDeployHistory":    {"GET", "/api/v1/apps/{name}/deploys"},
		"handleListBoundEnv":     {"GET", "/api/v1/apps/{name}/env/bound"},
		"handleUpdateApp":        {"PUT", "/api/v1/apps/{name}"},
		"handleHealthz":          {"GET", "/healthz"},
		"handleChangesStream":    {"GET", "/api/v1/events/changes"},
	}
	for id, w := range want {
		op, ok := Operations[id]
		if !ok || op.Method != w[0] || op.Path != w[1] {
			t.Errorf("%s = %+v, want %v", id, op, w)
		}
	}
	if !Operations["handleChangesStream"].Stream {
		t.Error("changes stream not flagged as stream")
	}
}

func TestAppsPaginationAndAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Query().Get("cursor") {
		case "":
			w.Header().Set(nextCursorHeader, "c2")
			_, _ = io.WriteString(w, `[{"name":"a"},{"name":"b"}]`)
		case "c2":
			_, _ = io.WriteString(w, `[{"name":"c"}]`)
		}
	}))
	defer srv.Close()
	c := New(srv.URL, "tok")
	var names []string
	for app, err := range c.AllApps(context.Background(), ListOptions{Limit: 2}) {
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, app["name"].(string))
	}
	if strings.Join(names, ",") != "a,b,c" {
		t.Fatalf("names = %v", names)
	}
	if _, _, err := New(srv.URL, "bad").ListApps(context.Background(), ListOptions{}); err == nil {
		t.Fatal("want auth error")
	}
}

func TestSetEnvBodyAndErrorHidesValues(t *testing.T) {
	var got map[string]map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v1/apps/my%20app" && r.URL.EscapedPath() != "/api/v1/apps/my%20app" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.EscapedPath())
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"invalid"}`)
	}))
	defer srv.Close()
	_, err := New(srv.URL, "tok").SetEnv(context.Background(), "my app", map[string]string{"K": "s3cret"})
	if got["env"]["K"] != "s3cret" {
		t.Fatalf("body = %v", got)
	}
	if err == nil || strings.Contains(err.Error(), "s3cret") {
		t.Fatalf("err = %v", err)
	}
}

func TestChangesStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, ": connected\n\nevent: ping\ndata: {\"heartbeat_ms\":20000}\n\ndata: {\"topics\":[\"apps\",\"nodes\"]}\n\n")
	}))
	defer srv.Close()
	var evs []ChangeEvent
	for ev, err := range New(srv.URL, "tok").Changes(context.Background()) {
		if err != nil {
			t.Fatal(err)
		}
		evs = append(evs, ev)
	}
	if len(evs) != 2 || !evs[0].Heartbeat || evs[0].HeartbeatMS != 20000 || strings.Join(evs[1].Topics, ",") != "apps,nodes" {
		t.Fatalf("events = %+v", evs)
	}
}
