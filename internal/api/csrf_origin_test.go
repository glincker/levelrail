package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCSRFCrossOrigin(t *testing.T) {
	tests := []struct {
		name   string
		method string
		hdr    map[string]string
		cookie bool
		want   bool
	}{
		{"get never blocked", http.MethodGet, map[string]string{"Origin": "https://evil.example"}, true, false},
		{"bearer exempt", http.MethodPost, map[string]string{"Origin": "https://evil.example", "Authorization": "Bearer x"}, true, false},
		{"no cookie exempt", http.MethodPost, map[string]string{"Origin": "https://evil.example"}, false, false},
		{"cookie no origin (cli)", http.MethodPost, nil, true, false},
		{"same origin", http.MethodPost, map[string]string{"Origin": "https://dash.example"}, true, false},
		{"cross origin", http.MethodPost, map[string]string{"Origin": "https://evil.example"}, true, true},
		{"null origin", http.MethodDelete, map[string]string{"Origin": "null"}, true, true},
		{"referer only cross-site", http.MethodPost, map[string]string{"Referer": "https://evil.example/x"}, true, true},
		{"referer only same host", http.MethodPost, map[string]string{"Referer": "https://dash.example/app"}, true, false},
		{"referer subdomain trick", http.MethodPost, map[string]string{"Referer": "https://dash.example.evil.example/"}, true, true},
		{"referer userinfo trick", http.MethodPost, map[string]string{"Referer": "https://dash.example@evil.example/"}, true, true},
		{"fetch metadata cross-site", http.MethodPost, map[string]string{"Sec-Fetch-Site": "cross-site"}, true, true},
		{"fetch metadata same-site", http.MethodPost, map[string]string{"Sec-Fetch-Site": "same-site"}, true, true},
		{"fetch metadata same-origin", http.MethodPost, map[string]string{"Sec-Fetch-Site": "same-origin"}, true, false},
		{"origin wins over lying fetch metadata", http.MethodPost, map[string]string{"Origin": "https://evil.example", "Sec-Fetch-Site": "same-origin"}, true, true},
		{"origin with opaque scheme", http.MethodPost, map[string]string{"Origin": "file://"}, true, true},
		{"port mismatch", http.MethodPut, map[string]string{"Origin": "https://dash.example:8443"}, true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, "https://dash.example/api/v1/x", nil)
			for k, v := range tc.hdr {
				r.Header.Set(k, v)
			}
			if tc.cookie {
				r.Header.Set("Cookie", sessionCookieName+"=t")
			}
			if got := csrfCrossOrigin(r); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
