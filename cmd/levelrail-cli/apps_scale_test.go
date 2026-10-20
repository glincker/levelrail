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
