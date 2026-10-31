package dnsrecords

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const purgeTestToken = "tok-secret-value"

type fakeCF struct {
	mu      sync.Mutex
	purges  [][]string
	sslMode string
	failAll bool
}

func (f *fakeCF) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+purgeTestToken {
			t.Errorf("missing bearer token on %s", r.URL.Path)
		}
		reply := func(result any) {
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "result": result})
		}
		switch {
		case r.URL.Path == "/zones":
			reply([]map[string]string{{"id": "z1", "name": "example.com"}})
		case strings.HasSuffix(r.URL.Path, "/purge_cache"):
			if f.failAll {
				_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "errors": []map[string]string{{"message": "Authentication error"}}})
				return
			}
			var body struct {
				Hosts []string `json:"hosts"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.mu.Lock()
			f.purges = append(f.purges, body.Hosts)
			f.mu.Unlock()
			reply(map[string]string{"id": "z1"})
		case strings.HasSuffix(r.URL.Path, "/settings/ssl"):
			reply(map[string]string{"id": "ssl", "value": f.sslMode})
		default:
			http.NotFound(w, r)
		}
	})
}

func newFakeCFAPI(t *testing.T, f *fakeCF) *CloudflareAPI {
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	return &CloudflareAPI{Token: purgeTestToken, BaseURL: srv.URL}
}

func TestPurgeHosts_BatchesAndSendsHosts(t *testing.T) {
	f := &fakeCF{}
	api := newFakeCFAPI(t, f)
	var hosts []string
	for i := range 65 {
		hosts = append(hosts, fmt.Sprintf("h%d.example.com", i))
	}
	if err := api.PurgeHosts(context.Background(), "example.com.", hosts); err != nil {
		t.Fatal(err)
	}
	if len(f.purges) != 3 || len(f.purges[0]) != 30 || len(f.purges[2]) != 5 {
		t.Fatalf("batches = %d", len(f.purges))
	}
}

func TestPurgeHosts_ErrorNeverLeaksToken(t *testing.T) {
	api := newFakeCFAPI(t, &fakeCF{failAll: true})
	err := api.PurgeHosts(context.Background(), "example.com.", []string{"a.example.com"})
	if err == nil || strings.Contains(err.Error(), purgeTestToken) {
		t.Fatalf("err = %v", err)
	}
}

func TestSSLMode(t *testing.T) {
	api := newFakeCFAPI(t, &fakeCF{sslMode: "flexible"})
	got, err := api.SSLMode(context.Background(), "example.com.")
	if err != nil || got != "flexible" {
		t.Fatalf("mode = %q err = %v", got, err)
	}
}
