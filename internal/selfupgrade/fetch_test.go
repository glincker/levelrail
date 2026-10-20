package selfupgrade

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFetcher(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/dl/v1.2.3/levelrail-linux-amd64", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("BIN")) })
	mux.HandleFunc("/dl/v1.2.3/checksums.txt", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("sums")) })
	srv := httptest.NewServer(mux)
	defer srv.Close()

	f := Fetcher{BaseURL: srv.URL + "/dl", AllowHTTP: true}
	dir := filepath.Join(t.TempDir(), "w")
	got, err := f.Fetch(context.Background(), "v1.2.3", "levelrail-linux-amd64", dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.BundlePath != "" {
		t.Fatalf("bundle path %q for a release without a signature", got.BundlePath)
	}
	if b, _ := os.ReadFile(got.BinaryPath); string(b) != "BIN" {
		t.Fatalf("binary = %q", b)
	}

	t.Run("plain http refused by default", func(t *testing.T) {
		_, err := Fetcher{BaseURL: srv.URL + "/dl"}.Fetch(context.Background(), "v1.2.3", "levelrail-linux-amd64", dir)
		if err == nil || !strings.Contains(err.Error(), "https") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("unsafe tag", func(t *testing.T) {
		if _, err := f.Fetch(context.Background(), "../etc", "levelrail-linux-amd64", dir); err == nil {
			t.Fatal("unsafe tag accepted")
		}
	})
	t.Run("size cap", func(t *testing.T) {
		small := Fetcher{BaseURL: srv.URL + "/dl", AllowHTTP: true, MaxBytes: 2}
		if _, err := small.Fetch(context.Background(), "v1.2.3", "levelrail-linux-amd64", dir); err == nil {
			t.Fatal("oversized download accepted")
		}
	})
	t.Run("missing asset", func(t *testing.T) {
		if _, err := f.Fetch(context.Background(), "v9.9.9", "levelrail-linux-amd64", dir); err == nil {
			t.Fatal("404 accepted")
		}
	})
}
