package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStackTrust_AddListRemove(t *testing.T) {
	var calls []string
	var added map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &added)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"cst_1","stack":"shop","repository":"acme/shop","ref_pattern":"refs/heads/main","environment":"production","verbs":["apply"]}`))
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"trusts":[{"id":"cst_1","stack":"shop","repository":"acme/shop","ref_pattern":"refs/heads/main","environment":"production","verbs":["apply"]}]}`))
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()

	code, out, e := runIaC([]string{"stack", "trust", "add", "shop", "--repo", "acme/shop", "--ref", "refs/heads/main", "--environment", "production", "--verbs", "apply", "--api-url", srv.URL}, nil)
	if code != exitOK || !strings.Contains(out, "cst_1") {
		t.Fatalf("add = %d %s %s", code, out, e)
	}
	if added["repository"] != "acme/shop" || added["environment"] != "production" {
		t.Fatalf("request = %v", added)
	}
	if code, out, _ = runIaC([]string{"stack", "trust", "list", "shop", "--api-url", srv.URL}, nil); code != exitOK || !strings.Contains(out, "refs/heads/main") {
		t.Fatalf("list = %d %s", code, out)
	}
	if code, out, _ = runIaC([]string{"stack", "trust", "remove", "shop", "cst_1", "--api-url", srv.URL}, nil); code != exitOK || !strings.Contains(out, "removed trust cst_1") {
		t.Fatalf("remove = %d %s", code, out)
	}
	if code, _, _ = runIaC([]string{"stack", "trust", "add", "shop", "--api-url", srv.URL}, nil); code == exitOK {
		t.Fatal("add without --repo and --ref succeeded")
	}
	want := []string{"POST /api/v1/stacks/shop/oidc-trusts", "GET /api/v1/stacks/shop/oidc-trusts", "DELETE /api/v1/stacks/shop/oidc-trusts/cst_1"}
	if strings.Join(calls, "|") != strings.Join(want, "|") {
		t.Fatalf("calls = %v", calls)
	}
}
