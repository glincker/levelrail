package selfupgrade

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseListing(t *testing.T) {
	full := `{"tag":"v1.5.8","published_at":"2026-11-18T10:00:00Z","beta":{"tag":"v1.6.0-beta.2"},"edge":{"tag":"main-abc1234"}}`
	tests := []struct {
		name, raw, channel, wantTag, wantTime string
		wantErr                               bool
	}{
		{name: "stable with time", raw: full, channel: "stable", wantTag: "v1.5.8", wantTime: "2026-11-18T10:00:00Z"},
		{name: "stable tag only", raw: `{"tag":"v1.5.9"}`, channel: "stable", wantTag: "v1.5.9"},
		{name: "bad time dropped", raw: `{"tag":"v1.5.9","published_at":"yesterday"}`, channel: "stable", wantTag: "v1.5.9"},
		{name: "beta", raw: full, channel: "beta", wantTag: "v1.6.0-beta.2"},
		{name: "edge", raw: full, channel: "edge", wantTag: "main-abc1234"},
		{name: "stable refuses prerelease", raw: `{"tag":"v1.6.0-beta.1"}`, channel: "stable", wantErr: true},
		{name: "stable refuses path tag", raw: `{"tag":"../v1.5.8"}`, channel: "stable", wantErr: true},
		{name: "beta refuses stable tag", raw: `{"beta":{"tag":"v1.5.8"}}`, channel: "beta", wantErr: true},
		{name: "beta missing", raw: `{"tag":"v1.5.8"}`, channel: "beta", wantErr: true},
		{name: "edge refuses odd tag", raw: `{"edge":{"tag":"main-xyz"}}`, channel: "edge", wantErr: true},
		{name: "not json", raw: `<html>`, channel: "stable", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseListing([]byte(tc.raw), tc.channel)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err == nil && (got.Tag != tc.wantTag || got.PublishedAt != tc.wantTime) {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestFromMirror(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		base    string
		channel string
		wantTag string
		wantErr bool
	}{
		{name: "stable", handler: func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"tag":"v1.5.8"}`)) }, channel: "stable", wantTag: "v1.5.8"},
		{name: "beta", handler: func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"tag":"v1.5.8","beta":{"tag":"v1.6.0-beta.1"}}`))
		}, channel: "beta", wantTag: "v1.6.0-beta.1"},
		{name: "404", handler: func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) }, channel: "stable", wantErr: true},
		{name: "too large", handler: func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"tag":"v1.5.8","pad":"` + strings.Repeat("a", listingMaxBytes) + `"}`))
		}, channel: "stable", wantErr: true},
		{name: "redirect off host refused", handler: func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "http://other.invalid/latest.json", http.StatusFound)
		}, channel: "stable", wantErr: true},
		{name: "plain http base refused", handler: func(http.ResponseWriter, *http.Request) {}, base: "http://example.test", channel: "stable", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()
			base, d := srv.URL, Discoverer{AllowHTTP: true}
			if tc.base != "" {
				base, d = tc.base, Discoverer{}
			}
			got, err := d.FromMirror(context.Background(), base, tc.channel)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err == nil && (got.Tag != tc.wantTag || got.Source != "mirror") {
				t.Fatalf("got %+v", got)
			}
		})
	}
}
