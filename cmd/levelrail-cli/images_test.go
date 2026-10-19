package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_ImagesGC_PreviewByDefault(t *testing.T) {
	var gotBody apiclient.ImageGCRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.ImageGCResource{
			DryRun:     true,
			Candidates: []apiclient.ImageEntry{{ID: "sha256:abcdef1234567890", Tags: []string{"web:v1"}, SizeBytes: 2048}},
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"images", "gc", "--api-url", srv.URL})
	if !gotBody.DryRun {
		t.Error("gc without --apply must send dry_run")
	}
	if !strings.Contains(stdout, "would remove abcdef123456") || !strings.Contains(stdout, "--apply") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRun_ImagesList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/images" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.ImageListResource{
			Images: []apiclient.ImageEntry{{ID: "sha256:aaaaaaaaaaaaaaaa", Tags: []string{"web:v2"}, SizeBytes: 1 << 20, Protected: true, ProtectReason: "live image of an app", Apps: []apiclient.ImageAppUse{{App: "web", Role: "live"}}}},
			Totals: apiclient.ImageTotals{Count: 1, SizeBytes: 1 << 20},
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"images", "list", "--api-url", srv.URL})
	if !strings.Contains(stdout, "web:v2") || !strings.Contains(stdout, "live image of an app") {
		t.Errorf("stdout = %q", stdout)
	}
}
