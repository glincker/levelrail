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

	"github.com/GLINCKER/levelrail/internal/store"
)

func seedV2Apps(t *testing.T, db *store.DB) {
	t.Helper()
	ctx := context.Background()
	for _, svc := range []store.DesiredService{
		{Name: "web", Image: "nginx:1", Port: 80, Labels: map[string]string{"tier": "web"}, Env: map[string]string{"API_KEY": "hunter2"}},
		{Name: "worker", Image: "busybox", Port: 81, Labels: map[string]string{"tier": "jobs"}},
		{Name: "secret", Image: "vault", Port: 82, Labels: map[string]string{"tier": "web"}},
	} {
		if err := db.SaveDesiredService(ctx, svc); err != nil {
			t.Fatal(err)
		}
	}
}

func TestV2MapperGolden(t *testing.T) {
	svc := store.DesiredService{Name: "web", Image: "nginx:1", Port: 80, ProjectID: "proj_1", Labels: map[string]string{"tier": "web"}, Strategy: "rolling", Replicas: 2, BindAddress: "private"}
	res := toAppResourceRaw(svc)
	obj := v2AppObject(v2APIVersion("example.com/sub"), res, svc, []string{"prod"}, &appStatusSummary{Label: "Healthy", Variant: "success"}, 7)
	got, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"apiVersion":"example.com/v2"`, `"kind":"App"`,
		`"metadata":{"name":"web","namespace":"proj_1","labels":{"tier":"web"},"annotations":{"example.com/tags":"prod"},"resourceVersion":"7"}`,
		`"spec":{"name":"web","image":"nginx:1"`, `"status":{"label":"Healthy","variant":"success"}`,
	} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("mapped object\n%s\nmissing %s", got, want)
		}
	}
}

func TestV2ListSelectorPagingAndGet(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedV2Apps(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v2/apps?labelSelector=tier%3Dweb&limit=1", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	var list v2List
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if list.Kind != "AppList" || list.Metadata.ResourceVersion == "0" || len(list.Items) != 1 || list.Items[0].Metadata.Name != "secret" || list.Metadata.Continue == "" {
		t.Fatalf("first page = %+v", list)
	}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v2/apps?labelSelector=tier%3Dweb&limit=1&continue="+list.Metadata.Continue, ""))
	var next v2List
	_ = json.Unmarshal(rec.Body.Bytes(), &next)
	if len(next.Items) != 1 || next.Items[0].Metadata.Name != "web" || next.Items[0].Metadata.ResourceVersion == "" {
		t.Fatalf("second page = %+v", next)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v2/apps/worker", ""))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"name":"worker"`) {
		t.Fatalf("get: %d %s", rec.Code, rec.Body.String())
	}
}

func TestV2WatchGoneAfterCompaction(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedV2Apps(t, db)
	if _, err := db.CompactResourceChanges(context.Background(), store.ResourceRetention{Retain: 1, MaxAge: time.Hour}, time.Now()); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v2/apps?watch=true&resourceVersion=0", ""))
	if rec.Code != http.StatusGone || !strings.Contains(rec.Body.String(), `"reason":"Expired"`) || !strings.Contains(rec.Body.String(), "list again") {
		t.Fatalf("watch from compacted version: %d %s", rec.Code, rec.Body.String())
	}
}

// TestV2WatchResumeFiltersPerObject proves a token denied one app never sees it
// in a list or a watch, and that no env value leaks into watch events.
func TestV2WatchResumeFiltersPerObject(t *testing.T) {
	t.Setenv(envWatchPollInterval, "20ms")
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedV2Apps(t, db)
	tok := mintTokenAs(t, rt, cookie, "viewer")
	doc := `{"Statement":[{"Effect":"Deny","Action":["read"],"Resource":["app:secret"]}]}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/iam/policies", `{"name":"hide-secret","document":`+doc+`}`))
	var pol policyResource
	_ = json.Unmarshal(rec.Body.Bytes(), &pol)
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/iam/policies/"+pol.ID+"/attachments", `{"principal_type":"token","principal_id":"`+tok.ID+`"}`))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("attach: %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, bearerRequest(http.MethodGet, "/api/v2/apps", "", tok.Token))
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), `"name":"secret"`) || !strings.Contains(rec.Body.String(), `"name":"web"`) {
		t.Fatalf("viewer list: %d %s", rec.Code, rec.Body.String())
	}

	srv := httptest.NewServer(rt.Handler())
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/v2/apps?watch=1&resourceVersion=0", nil)
	req.Header.Set("Authorization", "Bearer "+tok.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("watch status %d", resp.StatusCode)
	}
	if err := db.UpdateServiceSuspended(context.Background(), "worker", true); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateServiceSuspended(context.Background(), "secret", true); err != nil {
		t.Fatal(err)
	}
	var seen []string
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}
		if strings.Contains(data, "hunter2") {
			t.Fatalf("watch event leaks an env value: %s", data)
		}
		var ev struct {
			Type   string   `json:"type"`
			Object v2Object `json:"object"`
		}
		_ = json.Unmarshal([]byte(data), &ev)
		seen = append(seen, ev.Type+":"+ev.Object.Metadata.Name)
		if ev.Object.Metadata.Name == "secret" {
			t.Fatalf("viewer saw denied app in watch: %v", seen)
		}
		if ev.Type == store.ResourceOpModified && ev.Object.Metadata.Name == "worker" {
			break
		}
	}
	want := "[ADDED:web ADDED:worker MODIFIED:worker]"
	if got := strings.Join(strings.Fields(strings.Trim(strings.Join(seen, " "), " ")), " "); "["+got+"]" != want {
		t.Fatalf("events = [%s], want %s", got, want)
	}
}
