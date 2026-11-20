package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/domaindoctor"
)

func quietDoctorProbes(ip string) *domaindoctor.Probes {
	called := false
	return &domaindoctor.Probes{DNS: doctorFakeDNS{ips: []string{ip}}, TLS: doctorFakeTLS{called: &called}, HTTP: doctorFakeTLS{called: &called}}
}

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

			f.rt.traffic.doctorProbes = quietDoctorProbes("203.0.113.5")
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
	if rec := f.do(t, http.MethodPost, "/api/v1/static-sites/web/domains", `{"domain":"{{host}}.example.com"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("placeholder domain = %d, want 400", rec.Code)
	}
	if rec := f.do(t, http.MethodGet, "/api/v1/static-sites/web/domains/other.example.com/status", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unattached status = %d", rec.Code)
	}
}

func TestStaticSiteDomainDiagnosis(t *testing.T) {
	f := newGoLiveFixture(t, "203.0.113.5")
	seedAppWithDomains(t, f.db)
	f.do(t, http.MethodPost, "/api/v1/static-sites/web/domains", `{"domain":"site.example.com"}`)
	// Public DNS answers a different address, so the pointing check must fail.
	f.rt.traffic.doctorProbes = quietDoctorProbes("198.51.100.9")

	st := f.do(t, http.MethodGet, "/api/v1/static-sites/web/domains/site.example.com/status", "")
	var sv staticSiteDomainResponse
	if err := json.Unmarshal(st.Body.Bytes(), &sv); err != nil {
		t.Fatalf("decode: %v", err)
	}
	got := map[string]siteDiagnosis{}
	for _, d := range sv.Diagnosis {
		if d.Hint == "" {
			t.Errorf("%s has no hint", d.Code)
		}
		got[d.Code] = d
	}
	if d, ok := got[domaindoctor.IDDNSPointsHere]; !ok || d.OK {
		t.Fatalf("points_here = %+v in %+v", d, sv.Diagnosis)
	}
	if d, ok := got[diagCodePrivate]; !ok || !d.OK {
		t.Fatalf("private row = %+v", d)
	}
	if d, ok := got[diagCodeACME]; !ok || !d.OK {
		t.Fatalf("acme row = %+v", d)
	}
}

func TestShortHint(t *testing.T) {
	long := ""
	for range 50 {
		long += "word "
	}
	if got := shortHint(long); len([]rune(got)) > siteDiagnosisHintMax {
		t.Fatalf("hint len = %d", len([]rune(got)))
	}
}

func TestSiteDiagnosisCache(t *testing.T) {
	var c siteDiagnosisCache
	var runs atomic.Int32
	run := func(context.Context) []siteDiagnosis {
		runs.Add(1)
		return []siteDiagnosis{{Code: "x", OK: true}}
	}
	ctx := context.Background()
	c.get(ctx, "k", false, time.Hour, run)
	c.get(ctx, "k", false, time.Hour, run)
	if runs.Load() != 1 {
		t.Fatalf("cache hit ran doctor %d times", runs.Load())
	}
	c.get(ctx, "k", true, time.Hour, run)
	if runs.Load() != 2 {
		t.Fatalf("force refresh runs = %d", runs.Load())
	}
}

func TestSiteDiagnosisSingleFlight(t *testing.T) {
	var c siteDiagnosisCache
	var runs atomic.Int32
	release := make(chan struct{})
	run := func(context.Context) []siteDiagnosis {
		runs.Add(1)
		<-release
		return []siteDiagnosis{}
	}
	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.get(context.Background(), "k", true, time.Hour, run)
		}()
	}
	time.Sleep(100 * time.Millisecond)
	close(release)
	wg.Wait()
	if runs.Load() != 1 {
		t.Fatalf("concurrent callers ran doctor %d times", runs.Load())
	}
}

func TestWWWSiblingWithoutRecordIsHint(t *testing.T) {
	f := newGoLiveFixture(t, "203.0.113.5")
	d, ok := f.rt.wwwApexDiagnosis(context.Background(), "nodns.test", nil)
	if !ok || !d.OK || d.Hint == "" {
		t.Fatalf("www row = %+v ok=%v", d, ok)
	}
}
