package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/GLINCKER/levelrail/internal/platformimport"
)

type dokployFixture struct {
	Projects     json.RawMessage            `json:"projects"`
	Applications map[string]json.RawMessage `json:"applications"`
}

// fixtureDokploy serves the project_full.json fixture over the same endpoints a real instance exposes.
func fixtureDokploy(t *testing.T) *httptest.Server {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "internal", "platformimport", "testdata", "dokploy", "project_full.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fx dokployFixture
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/project.all":
			_, _ = w.Write(fx.Projects)
		case "/api/application.one":
			if app, ok := fx.Applications[r.URL.Query().Get("applicationId")]; ok {
				_, _ = w.Write(app)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

type recordingApplier struct {
	mu        sync.Mutex
	apps, dbs []string
}

func (a *recordingApplier) CreateApp(_ context.Context, p platformimport.AppPlan) ([]string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.apps = append(a.apps, p.Name)
	return nil, nil
}

func (a *recordingApplier) CreateDatabase(_ context.Context, p platformimport.DatabasePlan) ([]string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.dbs = append(a.dbs, p.Name)
	return nil, nil
}

func TestMigrateDokployE2E(t *testing.T) {
	srv := fixtureDokploy(t)
	src, err := platformimport.NewDokploy(srv.URL, "e2e-key", platformimport.ClientOptions{
		Policy: platformimport.NetworkPolicy{AllowLoopback: true, AllowPrivate: true},
	})
	if err != nil {
		t.Fatalf("NewDokploy: %v", err)
	}
	disc, err := src.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	full := platformimport.BuildPlan(disc, platformimport.PlanOptions{})
	if got := full.Report.Counts; len(full.Apps) != 2 || len(full.Databases) != 1 {
		t.Fatalf("plan apps=%d dbs=%d counts=%v, want 2 apps and 1 database", len(full.Apps), len(full.Databases), got)
	}
	unsupported := 0
	for _, it := range full.Report.Items {
		if it.Status == platformimport.StatusUnsupported {
			unsupported++
		}
	}
	if unsupported != 1 {
		t.Errorf("unsupported items = %d, want 1 (the compose stack)", unsupported)
	}

	only := platformimport.BuildPlan(disc, platformimport.PlanOptions{Only: []string{"app-web"}})
	if len(only.Apps) != 1 || len(only.Databases) != 0 {
		t.Fatalf("--only plan apps=%d dbs=%d, want 1 app and no database", len(only.Apps), len(only.Databases))
	}
	ap := &recordingApplier{}
	rep := platformimport.Apply(context.Background(), only, ap)
	if rep.Counts[platformimport.StatusCreated] != 1 || len(ap.apps) != 1 || ap.apps[0] != "web" {
		t.Fatalf("apply counts=%v apps=%v, want one created app named web", rep.Counts, ap.apps)
	}

	// A rerun that knows what exists must create nothing.
	again := platformimport.BuildPlan(disc, platformimport.PlanOptions{
		ExistingApps: map[string]string{"dokploy:app-web": "web"},
		TakenApps:    map[string]bool{"web": true},
	})
	if again.Report.Counts[platformimport.StatusAlready] < 1 {
		t.Errorf("rerun counts=%v, want web reported as already imported", again.Report.Counts)
	}
	for _, a := range again.Apps {
		if a.SourceID == "app-web" {
			t.Errorf("rerun planned app-web again")
		}
	}
}
