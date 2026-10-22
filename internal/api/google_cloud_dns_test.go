package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/libdns/libdns"

	"github.com/GLINCKER/levelrail/internal/dnszones"
	"github.com/GLINCKER/levelrail/internal/dnszones/gcdtest"
)

const testGCDKeyMarker = "TOP-SECRET-PEM"

const testGCDKey = `{"type":"service_account","project_id":"proj-1","client_email":"sa@proj-1.iam.gserviceaccount.com","private_key":"-----BEGIN PRIVATE KEY-----\n` + testGCDKeyMarker + `\n-----END PRIVATE KEY-----\n"}`

type fakeGCDSecrets struct{ values map[string]string }

func (f *fakeGCDSecrets) SetValue(_ context.Context, _, envKey, v string) error {
	f.values[envKey] = v
	return nil
}

func (f *fakeGCDSecrets) Exists(_ context.Context, _, envKey string) (bool, error) {
	_, ok := f.values[envKey]
	return ok, nil
}

func (f *fakeGCDSecrets) DeleteAll(context.Context, string) error {
	f.values = map[string]string{}
	return nil
}

func (f *fakeGCDSecrets) Resolve(_ context.Context, _, envKey string) (string, error) {
	return f.values[envKey], nil
}

func newGCDTestRouter(t *testing.T) (*Router, *http.Cookie, *gcdtest.Server) {
	t.Helper()
	srv := gcdtest.New("proj-1", "example.com.")
	t.Cleanup(srv.Close)
	db := openTestDB(t)
	secrets := &fakeGCDSecrets{values: map[string]string{}}
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithGoogleCloudDNSSecrets(secrets))
	rt.googleCloudDNSProvider = func(string) (dnszones.Provider, error) {
		return &dnszones.GoogleCloud{BaseURL: srv.URL, ProjectID: "proj-1", HTTP: srv.Client()}, nil
	}
	return rt, loginTestSession(t, rt, db), srv
}

func TestGoogleCloudDNSSettingsValidation(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantCode int
	}{
		{"enable without key", `{"enabled":true}`, http.StatusBadRequest},
		{"not json key", `{"enabled":true,"service_account_json":"nope"}`, http.StatusBadRequest},
		{"wrong key type", `{"enabled":true,"service_account_json":"{\"type\":\"authorized_user\"}"}`, http.StatusBadRequest},
		{"disable without key", `{"enabled":false}`, http.StatusOK},
		{"valid key", `{"enabled":true,"service_account_json":` + jsonString(testGCDKey) + `}`, http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt, cookie, _ := newGCDTestRouter(t)
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/google-cloud-dns", tc.body))
			if rec.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tc.wantCode, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), testGCDKeyMarker) {
				t.Fatalf("response leaks the key: %s", rec.Body.String())
			}
		})
	}
}

func TestGoogleCloudDNSNoMasterKey(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/google-cloud-dns", `{"enabled":false}`))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", rec.Code)
	}
}

func TestGoogleCloudDNSSaveTestAndRecordFlow(t *testing.T) {
	rt, cookie, srv := newGCDTestRouter(t)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		return rec
	}

	rec := do(http.MethodPut, "/api/v1/settings/google-cloud-dns", `{"enabled":true,"service_account_json":`+jsonString(testGCDKey)+`}`)
	var res googleCloudDNSResource
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if !res.Enabled || !res.HasKey || res.ProjectID != "proj-1" || res.ClientEmail != "sa@proj-1.iam.gserviceaccount.com" {
		t.Fatalf("resource = %+v", res)
	}

	rec = do(http.MethodGet, "/api/v1/settings/google-cloud-dns", "")
	if strings.Contains(rec.Body.String(), testGCDKeyMarker) || strings.Contains(rec.Body.String(), "private_key") {
		t.Fatalf("GET leaks the key: %s", rec.Body.String())
	}

	rec = do(http.MethodPost, "/api/v1/settings/google-cloud-dns/test", "")
	var tr googleCloudDNSTestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &tr); err != nil {
		t.Fatal(err)
	}
	if !tr.OK || len(tr.Zones) != 1 || tr.Zones[0] != "example.com" {
		t.Fatalf("test response = %+v", tr)
	}

	ctx := context.Background()
	if got := rt.activeDNSProvider(ctx); got != dnsProviderGoogleCloud {
		t.Fatalf("activeDNSProvider = %q", got)
	}
	target, err := rt.resolveDNSTarget(ctx, "app.example.com")
	if err != nil || target == nil || target.Provider != dnszones.ProviderGoogleCloud || target.Zone != "example.com." {
		t.Fatalf("resolveDNSTarget = %+v, %v", target, err)
	}
	if _, err := target.Manager.SetRecords(ctx, target.Zone, []libdns.Record{libdns.RR{Name: "app", Type: "A", TTL: 5 * time.Minute, Data: "203.0.113.7"}}); err != nil {
		t.Fatalf("SetRecords: %v", err)
	}
	found := false
	for _, s := range srv.Sets("example-com") {
		if s.Type == "A" && s.Name == "app.example.com." && len(s.RRDatas) == 1 && s.RRDatas[0] == "203.0.113.7" {
			found = true
		}
	}
	if !found {
		t.Fatalf("A record not written: %+v", srv.Sets("example-com"))
	}
	mgr, provider, zone, err := rt.resolveDNSRecordManager(ctx, "app.example.com")
	if err != nil || mgr == nil || provider != dnszones.ProviderGoogleCloud || zone != "example.com." {
		t.Fatalf("resolveDNSRecordManager = %v %q %q %v", mgr, provider, zone, err)
	}

	rec = do(http.MethodDelete, "/api/v1/settings/google-cloud-dns", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("disconnect status = %d", rec.Code)
	}
	if got := rt.activeDNSProvider(ctx); got != dnsProviderNone {
		t.Fatalf("after disconnect activeDNSProvider = %q", got)
	}
}
