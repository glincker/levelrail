package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_FixesListAndApply(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"fixes":[{"id":"disk.reclaim","kind":"auto","state":"offer","risk":"safe","title":"Clean up","changes":[],"attempts":[]}]}`))
		default:
			_, _ = w.Write([]byte(`{"fix_id":"disk.reclaim","outcome":"fixed","detail":"check passes"}`))
		}
	}))
	defer srv.Close()

	out, _ := runCLIExpectOK(t, []string{"fixes", "list", "--api-url", srv.URL})
	if !strings.Contains(out, "disk.reclaim") {
		t.Errorf("list output %q", out)
	}
	out, _ = runCLIExpectOK(t, []string{"fixes", "apply", "disk.reclaim", "--yes", "--api-url", srv.URL})
	if !strings.Contains(out, "fixed") || paths[len(paths)-1] != "POST /api/v1/fixes/disk.reclaim/apply" {
		t.Errorf("apply output %q paths %v", out, paths)
	}
}
