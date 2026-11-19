package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestStaticSiteDomainAttach(t *testing.T) {
	tests := []struct {
		name     string
		domain   string
		wantMode string
		wantZone int
	}{
		{"managed zone creates the record", "site.example.com", siteDomainModeAuto, 1},
		{"unmanaged zone returns the record to copy", "site.other.org", siteDomainModeManual, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newGoLiveFixture(t, "203.0.113.5")
			f.setPolicy(t, `{"auto_dns":true}`)
			seedAppWithDomains(t, f.db)

			rec := f.do(t, http.MethodPost, "/api/v1/static-sites/web/domains", `{"domain":"`+tc.domain+`"}`)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
			}
			var out staticSiteDomainResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if out.Mode != tc.wantMode || len(out.Records) != 1 || out.Records[0].Type != "A" || out.Records[0].Value != "203.0.113.5" {
				t.Fatalf("response = %+v", out)
			}
			if len(f.mgr.recs) != tc.wantZone {
				t.Fatalf("zone records = %d, want %d", len(f.mgr.recs), tc.wantZone)
			}

			st := f.do(t, http.MethodGet, "/api/v1/static-sites/web/domains/"+tc.domain+"/status", "")
			if st.Code != http.StatusOK {
				t.Fatalf("status route = %d body = %s", st.Code, st.Body.String())
			}
			var sv staticSiteDomainResponse
			if err := json.Unmarshal(st.Body.Bytes(), &sv); err != nil {
				t.Fatalf("decode status: %v", err)
			}
			if !sv.Verified || sv.Cert != "issued" {
				t.Fatalf("status = %+v", sv)
			}
		})
	}
}

func TestStaticSiteDomainRejectsBadInput(t *testing.T) {
	f := newGoLiveFixture(t, "203.0.113.5")
	seedAppWithDomains(t, f.db)
	if rec := f.do(t, http.MethodPost, "/api/v1/static-sites/web/domains", `{"domain":"not a domain"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad domain = %d", rec.Code)
	}
	if rec := f.do(t, http.MethodGet, "/api/v1/static-sites/web/domains/other.example.com/status", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unattached status = %d", rec.Code)
	}
}
