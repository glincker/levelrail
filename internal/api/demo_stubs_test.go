package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDemoProbeStubs(t *testing.T) {
	rt := demoRouter(t, DemoConfig{})
	rec := postDemo(rt, "/api/v1/demo/session", `{}`)
	var cookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("no session cookie")
	}
	get := func(target string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.AddCookie(cookie)
		out := httptest.NewRecorder()
		rt.Handler().ServeHTTP(out, req)
		return out
	}

	doctor := get("/api/v1/system/doctor")
	if doctor.Code != http.StatusOK {
		t.Fatalf("doctor status = %d, body = %s", doctor.Code, doctor.Body.String())
	}
	var report systemDoctorResponse
	if err := json.Unmarshal(doctor.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode doctor: %v", err)
	}
	var diskWarn bool
	for _, c := range report.Checks {
		if c.Status == doctorStatusFail || c.Status == doctorStatusUnknown {
			t.Errorf("check %s = %s, want no fail or unknown in the demo", c.Code, c.Status)
		}
		if c.Code == "disk_space" && c.Status == doctorStatusWarn {
			diskWarn = true
		}
	}
	if !report.OK || !diskWarn {
		t.Errorf("ok = %v, diskWarn = %v, want an ok report with the disk warning", report.OK, diskWarn)
	}

	for _, target := range []string{"/api/v1/updates", "/api/v1/dns/health-checks"} {
		if out := get(target); out.Code != http.StatusOK || strings.Contains(out.Body.String(), "demo_disabled") {
			t.Errorf("GET %s = %d %s, want a stubbed 200", target, out.Code, out.Body.String())
		}
	}
}
