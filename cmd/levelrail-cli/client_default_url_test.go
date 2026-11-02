package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHintIfNotLevelrail(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
		hint    bool
	}{
		{"levelrail", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"status":"ok"}`)) }, false},
		{"html page", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`<html></html>`)) }, true},
		{"not found", func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()
			var buf bytes.Buffer
			hintIfNotLevelrail(srv.URL, http.DefaultTransport, &buf)
			if got := strings.Contains(buf.String(), envLevelrailAPIURL); got != tc.hint {
				t.Fatalf("hint=%v want %v: %q", got, tc.hint, buf.String())
			}
		})
	}
	var buf bytes.Buffer
	hintIfNotLevelrail("http://127.0.0.1:1", http.DefaultTransport, &buf)
	if buf.Len() != 0 {
		t.Fatalf("unreachable URL must stay silent: %q", buf.String())
	}
}
