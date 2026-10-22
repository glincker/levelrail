package docker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const batchListBody = `[
 {"Id":"aaa","Names":["/web-1"],"Image":"img","State":"running","Ports":[{"PrivatePort":80,"PublicPort":30001,"IP":"127.0.0.1","Type":"tcp"}]},
 {"Id":"bbb","Names":["/api-1"],"Image":"img","State":"running"}
]`

func batchTestClient(t *testing.T, lists *atomic.Int32, nameFilters *[]string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/_ping"):
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			lists.Add(1)
			*nameFilters = append(*nameFilters, r.URL.Query().Get("filters"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(batchListBody))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DOCKER_HOST", "tcp://"+strings.TrimPrefix(srv.URL, "http://"))
	c, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestInspectManyByName_OneCallCoversManyNamesAndMarksAbsentOnes(t *testing.T) {
	var lists atomic.Int32
	var filters []string
	c := batchTestClient(t, &lists, &filters)

	got, err := c.InspectManyByName(context.Background(), []string{"web-1", "api-1", "gone-1"})
	if err != nil {
		t.Fatalf("InspectManyByName() error = %v", err)
	}
	if lists.Load() != 1 {
		t.Errorf("daemon calls = %d, want 1", lists.Load())
	}
	for _, name := range []string{"^/web-1$", "^/api-1$", "^/gone-1$"} {
		if !strings.Contains(filters[0], name) {
			t.Errorf("filters %q missing the anchored name %s", filters[0], name)
		}
	}
	if got["web-1"] == nil || !got["web-1"].Running || len(got["web-1"].Ports) != 1 || got["web-1"].Ports[0].HostPort != 30001 {
		t.Errorf("web-1 = %+v, want running with its published port", got["web-1"])
	}
	if got["api-1"] == nil {
		t.Error("api-1 missing")
	}
	if state, ok := got["gone-1"]; !ok || state != nil {
		t.Errorf("gone-1 = %v (present=%v), want a covered nil: absent containers are a known answer", state, ok)
	}
}

func TestInspectManyByName_SplitsLargeBatches(t *testing.T) {
	var lists atomic.Int32
	var filters []string
	c := batchTestClient(t, &lists, &filters)

	names := make([]string, inspectBatchSize*2+1)
	for i := range names {
		names[i] = "svc-" + string(rune('a'+i%26)) + "-" + strings.Repeat("x", i%7) + string(rune('0'+i%10)) + string(rune('A'+i/26))
	}
	if _, err := c.InspectManyByName(context.Background(), names); err != nil {
		t.Fatalf("InspectManyByName() error = %v", err)
	}
	if lists.Load() != 3 {
		t.Errorf("daemon calls for %d names = %d, want 3", len(names), lists.Load())
	}
}

func TestFetchInspectSet(t *testing.T) {
	var lists atomic.Int32
	var filters []string
	c := batchTestClient(t, &lists, &filters)

	set := FetchInspectSet(context.Background(), c, []string{"web-1", "gone-1"})
	if set == nil {
		t.Fatal("FetchInspectSet returned nil for a batching runtime")
	}
	state, covered := set.Lookup("web-1")
	if !covered || state == nil || state.ID != "aaa" {
		t.Fatalf("web-1 = %+v covered=%v", state, covered)
	}
	state.ID = "mutated"
	if again, _ := set.Lookup("web-1"); again.ID != "aaa" {
		t.Error("Lookup must hand out a private copy")
	}
	if st, covered := set.Lookup("gone-1"); !covered || st != nil {
		t.Errorf("gone-1 = %v covered=%v, want covered nil", st, covered)
	}
	if _, covered := set.Lookup("never-asked"); covered {
		t.Error("a name the batch never asked about must fall through to the runtime")
	}
}

type plainRuntime struct{ Runtime }

func TestFetchInspectSet_NilWhenRuntimeCannotBatch(t *testing.T) {
	if set := FetchInspectSet(context.Background(), plainRuntime{}, []string{"a"}); set != nil {
		t.Error("want nil for a runtime without BatchInspector")
	}
	var nilSet *InspectSet
	if _, covered := nilSet.Lookup("a"); covered {
		t.Error("a nil set covers nothing")
	}
	if set := FetchInspectSet(context.Background(), plainRuntime{}, nil); set != nil {
		t.Error("want nil for no names")
	}
}
