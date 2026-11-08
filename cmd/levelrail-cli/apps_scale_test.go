package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsScale_PreservesFieldsAndSetsReplicas(t *testing.T) {
	var put map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &put)
			_, _ = w.Write(body)
			return
		}
		_, _ = w.Write([]byte(`{"name":"web","image":"traefik/whoami:v1.10","port":80,"replicas":1,"strategy":"blue-green"}`))
	}))
	defer srv.Close()

	runCLIExpectOK(t, []string{"apps", "scale", "web", "--replicas", "3", "--strategy", "rolling", "--api-url", srv.URL})
	if put["replicas"] != float64(3) || put["strategy"] != "rolling" || put["image"] != "traefik/whoami:v1.10" {
		t.Errorf("PUT body = %v, want replicas 3, strategy rolling, image preserved", put)
	}
}

func TestRun_AppsScale_Validation(t *testing.T) {
	for _, args := range [][]string{
		{"apps", "scale", "web"},
		{"apps", "scale", "web", "--strategy", "yolo"},
	} {
		var stdout, stderr strings.Builder
		if got := run("levelrail-cli-test", args, &stdout, &stderr, envMap()); got == exitOK {
			t.Errorf("run(%v) = exitOK, want a validation failure", args)
		}
	}
}

func TestRun_AppsScale_PairsRollBackOnFailure(t *testing.T) {
	var puts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/shop/group":
			_, _ = w.Write([]byte(`{"services":[{"name":"shop-web"},{"name":"shop-worker"}]}`))
		case r.Method == http.MethodGet:
			name := strings.TrimPrefix(r.URL.Path, "/api/v1/apps/")
			_ = json.NewEncoder(w).Encode(map[string]any{"name": name, "image": "x:1", "port": 80, "replicas": 1})
		case r.Method == http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			puts = append(puts, r.URL.Path+" "+string(body))
			if strings.Contains(r.URL.Path, "worker") && strings.Contains(string(body), `"replicas":4`) {
				http.Error(w, `{"error":"nope"}`, http.StatusBadRequest)
				return
			}
			_, _ = w.Write(body)
		}
	}))
	defer srv.Close()
	var out, errb strings.Builder
	got := run("levelrail-cli-test", []string{"apps", "scale", "shop", "web=3", "worker=4", "--api-url", srv.URL, "--token", "t"}, &out, &errb, envMap())
	if got == exitOK {
		t.Fatalf("exit = 0, want failure (stderr %q)", errb.String())
	}
	if !strings.Contains(errb.String(), `rolled back "shop-web" to 1 replica(s)`) {
		t.Errorf("stderr = %q, want a rollback report", errb.String())
	}
	if len(puts) != 3 || !strings.Contains(puts[2], `"replicas":1`) {
		t.Errorf("PUTs = %v, want web=3, worker=4 (rejected), web back to 1", puts)
	}
}

func TestAppsScale_SwapAndCpuset(t *testing.T) {
	var put map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &put)
			_, _ = w.Write(body)
			return
		}
		_, _ = w.Write([]byte(`{"name":"web","image":"x:1","port":80,"replicas":1}`))
	}))
	defer srv.Close()

	runCLIExpectOK(t, []string{"apps", "scale", "web", "--memory", "512Mi", "--swap-memory", "1Gi", "--cpuset-cpus", "0-1", "--api-url", srv.URL})
	res, _ := put["resources"].(map[string]any)
	if res["cpuset_cpus"] != "0-1" || res["swap_memory_bytes"] != float64(1<<30) {
		t.Errorf("PUT resources = %v, want cpuset and swap set", res)
	}
	if len(res) < 3 {
		t.Errorf("PUT resources = %v, want memory, swap and cpuset", res)
	}
}

func TestAppsScale_SwapCpusetValidation(t *testing.T) {
	for _, args := range [][]string{
		{"apps", "scale", "web", "--cpuset-cpus", "a-b"},
		{"apps", "scale", "web", "--swap-memory", "nope"},
		{"apps", "scale", "web", "worker=1", "--cpuset-cpus", "0"},
	} {
		var stdout, stderr strings.Builder
		if got := run("levelrail-cli-test", args, &stdout, &stderr, envMap()); got == exitOK {
			t.Errorf("run(%v) = exitOK, want a validation failure", args)
		}
	}
}
