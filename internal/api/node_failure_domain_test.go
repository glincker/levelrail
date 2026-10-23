package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHandleSetNodeFailureDomain(t *testing.T) {
	tests := []struct {
		name         string
		bodies       []string
		wantStatus   int
		wantRegion   string
		wantProvider string
		wantZone     string
	}{
		{"all three", []string{`{"region":"eu","provider":"hetzner","zone":"fsn1"}`}, http.StatusOK, "eu", "hetzner", "fsn1"},
		{"region only leaves others empty", []string{`{"region":"eu"}`}, http.StatusOK, "eu", "", ""},
		{"omitted fields keep stored values", []string{`{"region":"eu","provider":"aws","zone":"a"}`, `{"region":"us"}`}, http.StatusOK, "us", "aws", "a"},
		{"empty string clears", []string{`{"region":"eu","zone":"a"}`, `{"region":"eu","zone":""}`}, http.StatusOK, "eu", "", ""},
		{"too long zone rejected", []string{`{"region":"eu","zone":"` + strings.Repeat("z", 65) + `"}`}, http.StatusBadRequest, "", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt, db := newTestRouter(t)
			cookie := loginTestSession(t, rt, db)
			seedNode(t, db, "node_a", "alpha")

			var rec *httptest.ResponseRecorder
			for _, body := range tc.bodies {
				rec = httptest.NewRecorder()
				rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/nodes/node_a/region", body))
			}
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.wantStatus != http.StatusOK {
				return
			}
			var got nodeResource
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if got.Region != tc.wantRegion || got.Provider != tc.wantProvider || got.Zone != tc.wantZone {
				t.Fatalf("got region=%q provider=%q zone=%q", got.Region, got.Provider, got.Zone)
			}
		})
	}
}

func TestWithDomainLabels(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		region string
		zone   string
		want   map[string]string
	}{
		{"no node fields keeps labels", map[string]string{"gpu": "yes"}, "", "", map[string]string{"gpu": "yes"}},
		{"node fields fill gaps", nil, "eu", "a", map[string]string{"region": "eu", "zone": "a"}},
		{"explicit label wins", map[string]string{"zone": "x"}, "eu", "a", map[string]string{"zone": "x", "region": "eu"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := withDomainLabels(tc.labels, store.Node{Region: tc.region, Zone: tc.zone})
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}
