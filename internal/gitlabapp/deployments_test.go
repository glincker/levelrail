package gitlabapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type recordedCall struct {
	Method string
	Path   string
	Query  string
	Body   map[string]any
}

func deploymentServer(t *testing.T, calls *[]recordedCall, handle func(r *http.Request, w http.ResponseWriter) bool) (*Client, string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := recordedCall{Method: r.Method, Path: r.URL.EscapedPath(), Query: r.URL.RawQuery}
		_ = json.NewDecoder(r.Body).Decode(&c.Body)
		*calls = append(*calls, c)
		if !handle(r, w) {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return &Client{HTTP: srv.Client()}, srv.URL
}

func TestEnsureEnvironmentCreatesOnceThenReuses(t *testing.T) {
	var calls []recordedCall
	exists := false
	c, srvURL := deploymentServer(t, &calls, func(r *http.Request, w http.ResponseWriter) bool {
		switch r.Method {
		case http.MethodGet:
			if exists {
				_ = json.NewEncoder(w).Encode([]environmentResponse{{ID: 5, Name: "web/production"}})
			} else {
				_, _ = w.Write([]byte("[]"))
			}
			return true
		case http.MethodPost:
			exists = true
			_ = json.NewEncoder(w).Encode(environmentResponse{ID: 5, Name: "web/production"})
			return true
		}
		return false
	})
	first, err := c.EnsureEnvironment(context.Background(), srvURL, "tok", "grp/proj", "web/production", "https://app.test", "production")
	if err != nil || first.ID != 5 {
		t.Fatalf("first = %+v, %v", first, err)
	}
	post := calls[1]
	if post.Method != http.MethodPost || post.Path != "/api/v4/projects/grp%2Fproj/environments" || post.Body["name"] != "web/production" || post.Body["tier"] != "production" || post.Body["external_url"] != "https://app.test" {
		t.Fatalf("create call = %+v", post)
	}
	n := len(calls)
	again, err := c.EnsureEnvironment(context.Background(), srvURL, "tok", "grp/proj", "web/production", "https://app.test", "production")
	if err != nil || again.ID != 5 {
		t.Fatalf("again = %+v, %v", again, err)
	}
	if len(calls) != n+1 || calls[n].Method != http.MethodGet {
		t.Fatalf("a repeat call must only look the environment up: %+v", calls[n:])
	}
}

func TestEnsureEnvironmentFallsBackWhenCreateLosesARace(t *testing.T) {
	var calls []recordedCall
	lookups := 0
	c, srvURL := deploymentServer(t, &calls, func(r *http.Request, w http.ResponseWriter) bool {
		if r.Method == http.MethodGet {
			lookups++
			if lookups == 1 {
				_, _ = w.Write([]byte("[]"))
			} else {
				_ = json.NewEncoder(w).Encode([]environmentResponse{{ID: 9, Name: "e"}})
			}
			return true
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":{"name":["has already been taken"]}}`))
		return true
	})
	got, err := c.EnsureEnvironment(context.Background(), srvURL, "tok", "grp/proj", "e", "", "development")
	if err != nil || got.ID != 9 {
		t.Fatalf("got = %+v, %v", got, err)
	}
}

func TestCreateAndUpdateDeployment(t *testing.T) {
	var calls []recordedCall
	c, srvURL := deploymentServer(t, &calls, func(r *http.Request, w http.ResponseWriter) bool {
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"id":77}`))
		}
		return true
	})
	u := srvURL
	id, err := c.CreateDeployment(context.Background(), u, "tok", "grp/proj", "web/production", "abc", "main", false, DeploymentRunning)
	if err != nil || id != 77 {
		t.Fatalf("id = %d, %v", id, err)
	}
	b := calls[0].Body
	if calls[0].Path != "/api/v4/projects/grp%2Fproj/deployments" || b["environment"] != "web/production" || b["sha"] != "abc" || b["ref"] != "main" || b["status"] != "running" || b["tag"] != false {
		t.Fatalf("create = %+v", calls[0])
	}
	for _, st := range []DeploymentStatus{DeploymentSuccess, DeploymentFailed, DeploymentCanceled} {
		if err := c.UpdateDeploymentStatus(context.Background(), u, "tok", "grp/proj", 77, st); err != nil {
			t.Fatalf("update %s: %v", st, err)
		}
		last := calls[len(calls)-1]
		if last.Method != http.MethodPut || last.Path != "/api/v4/projects/grp%2Fproj/deployments/77" || last.Body["status"] != string(st) {
			t.Fatalf("update call = %+v", last)
		}
	}
}

func TestStopEnvironment(t *testing.T) {
	var calls []recordedCall
	found := true
	c, srvURL := deploymentServer(t, &calls, func(r *http.Request, w http.ResponseWriter) bool {
		if r.Method == http.MethodGet {
			if found {
				_ = json.NewEncoder(w).Encode([]environmentResponse{{ID: 3, Name: "web/preview-7"}})
			} else {
				_, _ = w.Write([]byte("[]"))
			}
		}
		return true
	})
	u := srvURL
	if err := c.StopEnvironment(context.Background(), u, "tok", "grp/proj", "web/preview-7"); err != nil {
		t.Fatal(err)
	}
	if last := calls[len(calls)-1]; last.Method != http.MethodPost || last.Path != "/api/v4/projects/grp%2Fproj/environments/3/stop" {
		t.Fatalf("stop call = %+v", last)
	}
	found = false
	n := len(calls)
	if err := c.StopEnvironment(context.Background(), u, "tok", "grp/proj", "web/preview-7"); err != nil {
		t.Fatalf("a missing environment must not fail: %v", err)
	}
	if len(calls) != n+1 {
		t.Fatalf("a missing environment must only be looked up: %+v", calls[n:])
	}
}

func TestDeploymentCallsSurfaceAPIErrors(t *testing.T) {
	var calls []recordedCall
	c, srvURL := deploymentServer(t, &calls, func(_ *http.Request, w http.ResponseWriter) bool {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"403 Forbidden"}`))
		return true
	})
	if _, err := c.CreateDeployment(context.Background(), srvURL, "tok", "grp/proj", "e", "abc", "abc", false, DeploymentRunning); err == nil {
		t.Fatal("a 403 must be an error")
	}
}
