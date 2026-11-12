package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCloudConnectionCreateAndListCLI(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"name":"oci","provider":"oracle","auth_method":"oidc_federation","preview":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"connections":[{"name":"oci","provider":"oracle","auth_method":"oidc_federation","last_check_class":"unsupported"}]}`))
	}))
	defer srv.Close()
	runCLIExpectOK(t, []string{"cloud", "connection", "create", "oci", "--provider", "oracle", "--auth", "oidc_federation", "--region", "us-ashburn-1", "--api-url", srv.URL})
	if !strings.Contains(gotBody, `"provider":"oracle"`) || !strings.Contains(gotBody, `"name":"oci"`) || !strings.Contains(gotBody, `"region":"us-ashburn-1"`) {
		t.Fatalf("request body = %s", gotBody)
	}
	stdout, _ := runCLIExpectOK(t, []string{"cloud", "connection", "list", "--api-url", srv.URL})
	if !strings.Contains(stdout, "oci\toracle\toidc_federation\tcheck=unsupported") {
		t.Fatalf("list output = %q", stdout)
	}
}
