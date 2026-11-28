package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/apiclient"
	"github.com/glincker/levelrail/internal/placement"
)

func TestRolloutGroupSelectionAndBatchOrder(t *testing.T) {
	mk := func(id, name string, l map[string]string) nodeLabelSet {
		return nodeLabelSet{Node: apiclient.NodeResource{ID: id, Name: name}, Labels: l}
	}
	sets := []nodeLabelSet{
		mk("n3", "pi-c", map[string]string{"fleet": "edge", "site": "a"}),
		mk("n1", "pi-a", map[string]string{"fleet": "edge"}),
		mk("n2", "pi-b", map[string]string{"fleet": "lab"}),
		mk("n4", "pi-d", map[string]string{"fleet": "edge"}),
	}
	apps := []apiclient.AppResource{
		{Name: "sensor-a", NodeID: "n1"}, {Name: "sensor", NodeID: "n3"},
		{Name: "sensor-b", NodeID: "n2"}, {Name: "sensor-d", NodeID: "n4"}, {Name: "other", NodeID: "n1"},
	}
	batches := planRollout(sets, apps, "sensor", placement.Selector{"fleet": "edge"}, 2)
	if len(batches) != 2 || len(batches[0]) != 2 || len(batches[1]) != 1 {
		t.Fatalf("batches = %+v", batches)
	}
	if batches[0][0].Node != "pi-a" || batches[0][1].Node != "pi-c" || batches[1][0].Node != "pi-d" {
		t.Fatalf("order = %+v", batches)
	}
	g := groupsFromLabels(sets)
	if g[0].Selector != "fleet=edge" || len(g[0].Nodes) != 3 {
		t.Fatalf("groups = %+v", g)
	}
}

// rolloutFake serves nodes n1..n3 (n3 offline) and apps app-1..app-3, one per node.
// stuck names apps that never converge; others converge after the deploy call.
type rolloutFake struct {
	mu       sync.Mutex
	deployed []string
	stuck    map[string]bool
	failDep  map[string]bool
}

func (f *rolloutFake) handler() http.Handler {
	mux := http.NewServeMux()
	js := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("/api/v1/nodes", func(w http.ResponseWriter, _ *http.Request) {
		js(w, []apiclient.NodeResource{{ID: "n1", Name: "a", Status: "online"}, {ID: "n2", Name: "b", Status: "online"}, {ID: "n3", Name: "c", Status: "offline"}})
	})
	mux.HandleFunc("/api/v1/nodes/", func(w http.ResponseWriter, _ *http.Request) {
		js(w, apiclient.NodeLabels{Labels: map[string]string{"fleet": "edge"}})
	})
	mux.HandleFunc("/api/v1/apps", func(w http.ResponseWriter, _ *http.Request) {
		js(w, []apiclient.AppResource{{Name: "app-1", NodeID: "n1"}, {Name: "app-2", NodeID: "n2"}, {Name: "app-3", NodeID: "n3"}})
	})
	mux.HandleFunc("/api/v1/apps/", func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/api/v1/apps/")
		f.mu.Lock()
		defer f.mu.Unlock()
		if name, ok := strings.CutSuffix(rest, "/deploys"); ok {
			if f.failDep[name] {
				http.Error(w, `{"error":"boom"}`, http.StatusInternalServerError)
				return
			}
			f.deployed = append(f.deployed, name)
			js(w, apiclient.AppResource{Name: name})
			return
		}
		a := apiclient.AppResource{Name: rest, Image: "old"}
		if !f.stuck[rest] && slices.Contains(f.deployed, rest) {
			a.Image = "new"
		}
		js(w, a)
	})
	return mux
}

func execRollout(t *testing.T, f *rolloutFake, timeout string, extra ...string) (int, []rolloutResult) {
	t.Helper()
	old := rolloutPollEvery
	rolloutPollEvery = time.Millisecond
	t.Cleanup(func() { rolloutPollEvery = old })
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	env := func(k string) (string, bool) {
		if k == rolloutTimeoutEnv && timeout != "" {
			return timeout, true
		}
		return "", false
	}
	args := append([]string{"app", "--image", "new", "--group", "fleet=edge", "--json", "--api-url", srv.URL}, extra...)
	var stdout, stderr bytes.Buffer
	code := runAppsRollout("levelrail-cli-test", args, &stdout, &stderr, env)
	var res []rolloutResult
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		t.Fatalf("output %q (stderr %q): %v", stdout.String(), stderr.String(), err)
	}
	return code, res
}

func statuses(res []rolloutResult) string {
	var parts []string
	for _, r := range res {
		parts = append(parts, r.App+"="+r.Status)
	}
	return strings.Join(parts, ",")
}

func TestRolloutConvergedBatchProceedsAndOfflineIsQueued(t *testing.T) {
	f := &rolloutFake{}
	code, res := execRollout(t, f, "5s", "--max-unavailable", "1")
	if code != exitOK {
		t.Fatalf("exit = %d, results %s", code, statuses(res))
	}
	if got := statuses(res); got != "app-1=ok,app-2=ok,app-3=queued" {
		t.Fatalf("statuses = %s", got)
	}
	if !slices.Equal(f.deployed, []string{"app-1", "app-2", "app-3"}) {
		t.Fatalf("deploy order = %v", f.deployed)
	}
}

func TestRolloutFailedBatchHaltsAndSkipsRest(t *testing.T) {
	f := &rolloutFake{stuck: map[string]bool{"app-1": true}}
	code, res := execRollout(t, f, "30ms", "--max-unavailable", "1")
	if code == exitOK {
		t.Fatalf("exit = %d, want non-zero", code)
	}
	if got := statuses(res); got != "app-1=failed,app-2=skipped,app-3=skipped" {
		t.Fatalf("statuses = %s", got)
	}
	if !slices.Equal(f.deployed, []string{"app-1"}) {
		t.Fatalf("later batches were deployed: %v", f.deployed)
	}
}

func TestRolloutDeployErrorHalts(t *testing.T) {
	f := &rolloutFake{failDep: map[string]bool{"app-2": true}}
	code, res := execRollout(t, f, "5s", "--max-unavailable", "1")
	if code == exitOK {
		t.Fatalf("exit = %d, want non-zero", code)
	}
	if got := statuses(res); got != "app-1=ok,app-2=failed,app-3=skipped" {
		t.Fatalf("statuses = %s", got)
	}
}

func TestRolloutBatchTimeoutEnvIsHonored(t *testing.T) {
	f := &rolloutFake{stuck: map[string]bool{"app-1": true}}
	start := time.Now()
	code, res := execRollout(t, f, "20ms", "--max-unavailable", "1")
	if code == exitOK || res[0].Status != "failed" || !strings.Contains(res[0].Detail, "20ms") {
		t.Fatalf("exit %d results %+v", code, res)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatalf("timeout not honored, took %s", time.Since(start))
	}
}

func TestRolloutOfflineNodeNotWaitedOn(t *testing.T) {
	f := &rolloutFake{stuck: map[string]bool{"app-3": true}}
	code, res := execRollout(t, f, "20ms", "--max-unavailable", "3")
	if code != exitOK || statuses(res) != "app-1=ok,app-2=ok,app-3=queued" {
		t.Fatalf("exit %d statuses %s", code, statuses(res))
	}
}
