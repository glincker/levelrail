package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

var protoV6Factories = map[string]func() (tfprotov6.ProviderServer, error){
	TypeName: providerserver.NewProtocol6WithError(New("test")()),
}

func TestClientDo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/v1/apps/missing":
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "app not found"})
		case "/api/v1/projects":
			var in map[string]string
			_ = json.NewDecoder(r.Body).Decode(&in)
			_ = json.NewEncoder(w).Encode(wireProject{ID: "p1", Name: in["name"]})
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL+"/", "tok")
	var p wireProject
	if err := c.do(context.Background(), http.MethodPost, "/api/v1/projects", map[string]string{"name": "x"}, &p); err != nil {
		t.Fatal(err)
	}
	if p.ID != "p1" || p.Name != "x" {
		t.Fatalf("unexpected project %+v", p)
	}

	err := c.do(context.Background(), http.MethodGet, "/api/v1/apps/missing", nil, nil)
	if !isNotFound(err) || err.Error() != "404 app not found" {
		t.Fatalf("want a 404 with the server message, got %v", err)
	}

	bad := NewClient(srv.URL, "wrong")
	if err := bad.do(context.Background(), http.MethodGet, "/api/v1/apps", nil, nil); err == nil {
		t.Fatal("expected an auth error")
	}
}

func TestEnvPath(t *testing.T) {
	tests := []struct {
		scope, id, want string
		wantErr         bool
	}{
		{"project", "p 1", "/api/v1/projects/p%201/env", false},
		{"environment", "e1", "/api/v1/environments/e1/env", false},
		{"app", "a", "", true},
	}
	for _, tc := range tests {
		got, err := envPath(tc.scope, tc.id)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("envPath(%q,%q) = %q, %v", tc.scope, tc.id, got, err)
		}
	}
}

// TestProviderValidates runs terraform validate against the provider schema.
// It needs a terraform or tofu binary, so it only runs with TF_ACC set.
func TestProviderValidates(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6Factories,
		Steps: []resource.TestStep{{
			Config:   `provider "` + TypeName + "\" {\n  url   = \"http://127.0.0.1:1\"\n  token = \"x\"\n}\n",
			PlanOnly: true,
		}},
	})
}
