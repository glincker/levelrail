package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/datamigrate"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/platformimport"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

type mapResolver map[string]datamigrate.DomainState

func (m mapResolver) Lookup(_ context.Context, d string) datamigrate.DomainState { return m[d] }

func seedImportedApp(t *testing.T, db *store.DB, name string, domains []string, ready bool) {
	t.Helper()
	ctx := context.Background()
	err := db.SaveDesiredService(ctx, store.DesiredService{Name: name, Image: "nginx:1", Port: 80, Domains: domains,
		Labels:  map[string]string{platformimport.LabelSourceID: "coolify:" + name},
		Volumes: []store.ServiceVolume{{Name: "app-" + name + "-data", ContainerPath: "/data"}}})
	if err != nil {
		t.Fatal(err)
	}
	status := reconcile.ConditionFalse
	if ready {
		status = reconcile.ConditionTrue
	}
	if err := db.UpsertConditions(ctx, "application/"+name, []reconcile.Condition{{Type: reconcile.ConditionTypeReady, Status: status, Reason: "Probe"}}); err != nil {
		t.Fatal(err)
	}
}

func TestCutoverReport(t *testing.T) {
	rt, db, cookie := newSafetyRouter(t, stubResolver{digest: "sha256:abc"})
	seedImportedApp(t, db, "shop", []string{"shop.example.com"}, true)
	seedImportedApp(t, db, "blog", []string{"blog.example.com"}, true)
	seedImportedApp(t, db, "broken", []string{"broken.example.com"}, false)
	rt.dnsResolver = mapResolver{
		"shop.example.com":   {Addrs: []string{"198.51.100.7"}, TTL: 60, Authoritative: true},
		"blog.example.com":   {Addrs: []string{"198.51.100.7"}, TTL: 3600, Authoritative: true},
		"broken.example.com": {Addrs: []string{"198.51.100.7"}, TTL: 60, Authoritative: true},
	}

	rec := serve(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/migration/cutover?target_ip=203.0.113.10", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	var rep cutoverReport
	if err := json.Unmarshal(rec.Body.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, d := range rep.Domains {
		got[d.Domain] = d.Verdict
	}
	want := map[string]string{"shop.example.com": "go", "blog.example.com": "wait", "broken.example.com": "no-go"}
	for d, v := range want {
		if got[d] != v {
			t.Errorf("%s verdict = %q, want %q", d, got[d], v)
		}
	}
	if rep.Verdict != "no-go" {
		t.Errorf("overall = %q, want the worst verdict", rep.Verdict)
	}
	if rec := serve(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/migration/cutover?target_ip=nope", "")); rec.Code != http.StatusBadRequest {
		t.Errorf("bad target_ip = %d, want 400", rec.Code)
	}
}

func TestVolumeGuideRoute(t *testing.T) {
	rt, db, cookie := newSafetyRouter(t, stubResolver{digest: "sha256:abc"})
	seedImportedApp(t, db, "shop", nil, true)
	if rec := serve(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/migration/volumes?source="+url.QueryEscape("x;y"), "")); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad source = %d, want 400", rec.Code)
	}
	rec := serve(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/migration/volumes?source="+url.QueryEscape("root@old.example.com"), ""))
	var out volumeGuideResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || len(out.Guides) != 1 {
		t.Fatalf("response = %s, %v", rec.Body, err)
	}
	if !strings.Contains(out.Guides[0].Command, "/var/lib/docker/volumes/data/_data/") {
		t.Errorf("command = %s", out.Guides[0].Command)
	}
}

type copyRuntime struct {
	docker.Runtime
	mu      sync.Mutex
	removed int
}

func (c *copyRuntime) InspectByName(_ context.Context, name string) (*docker.ContainerState, error) {
	return &docker.ContainerState{ID: name, Name: name, Running: true}, nil
}
func (c *copyRuntime) Create(context.Context, docker.ContainerSpec) (string, error) { return "h1", nil }
func (c *copyRuntime) Start(context.Context, string) error                          { return nil }
func (c *copyRuntime) Remove(context.Context, string, bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.removed++
	return nil
}
func (c *copyRuntime) Exec(_ context.Context, _ string, cmd []string) (io.ReadCloser, error) {
	script := cmd[len(cmd)-1]
	switch {
	case strings.Contains(script, "pg_dump"):
		return io.NopCloser(strings.NewReader("CREATE TABLE t();")), nil
	case strings.Contains(script, "xpath"):
		return io.NopCloser(strings.NewReader("public.t|3\n")), nil
	}
	return io.NopCloser(strings.NewReader("")), nil
}
func (c *copyRuntime) ExecWithInput(_ context.Context, _ string, _ []string, in io.Reader) (io.ReadCloser, error) {
	_, _ = io.Copy(io.Discard, in)
	return io.NopCloser(strings.NewReader("")), nil
}

func TestCopyDatabaseDataRoute(t *testing.T) {
	rt, db, cookie := newSafetyRouter(t, stubResolver{digest: "sha256:abc"})
	ctx := context.Background()
	for _, d := range []store.DesiredDatabase{{Name: "main", Engine: "postgres", Version: "16"}, {Name: "olap", Engine: "clickhouse", Version: "24"}} {
		if err := db.SaveDesiredDatabase(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	runtime := &copyRuntime{}
	rt.execRuntime = func(string) (docker.Runtime, error) { return runtime, nil }
	body := `{"host":"old.example.com","user":"u","password":"hunter2","database":"app"}`

	if rec := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/imports/platform/databases/olap/copy", body)); rec.Code != http.StatusBadRequest {
		t.Fatalf("unsupported engine = %d, want 400: %s", rec.Code, rec.Body)
	}
	if rec := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/imports/platform/databases/main/copy", `{"host":"","database":"app"}`)); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty host = %d, want 400", rec.Code)
	}
	rec := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/imports/platform/databases/main/copy", body))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("copy = %d: %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "hunter2") {
		t.Fatal("response echoes the source password")
	}

	deadline := time.Now().Add(5 * time.Second)
	var res dataImportResource
	for time.Now().Before(deadline) {
		get := serve(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/imports/platform/databases/main", ""))
		_ = json.Unmarshal(get.Body.Bytes(), &res)
		if res.Status != datamigrate.StatusCopying {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if res.Status != datamigrate.StatusVerified || res.Checked != 1 || res.Mismatched != 0 {
		t.Fatalf("final status = %+v", res)
	}
	if strings.Contains(res.Reason, "hunter2") {
		t.Fatal("status leaks the password")
	}

	list := serve(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/imports/platform/databases", ""))
	var all []dataImportResource
	_ = json.Unmarshal(list.Body.Bytes(), &all)
	states := map[string]string{}
	for _, r := range all {
		states[r.Database] = r.Status
	}
	if states["main"] != "verified" || states["olap"] != "unsupported" {
		t.Errorf("list states = %v", states)
	}
}
