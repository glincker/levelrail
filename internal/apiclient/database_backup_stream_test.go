package apiclient

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDownloadBackupTo_StreamsAndReportsErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/databases/main/backups/gone/download" {
			http.Error(w, `{"error":"missing"}`, http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte("dump-bytes"))
	}))
	defer srv.Close()
	client := NewClient(srv.URL, "test-token")

	var buf bytes.Buffer
	n, err := client.DownloadBackupTo(context.Background(), "main", "hist-1", &buf)
	if err != nil || n != 10 || buf.String() != "dump-bytes" {
		t.Fatalf("n=%d err=%v body=%q", n, err, buf.String())
	}

	buf.Reset()
	_, err = client.DownloadBackupTo(context.Background(), "main", "gone", &buf)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound || buf.Len() != 0 {
		t.Fatalf("err=%v written=%d", err, buf.Len())
	}
}
