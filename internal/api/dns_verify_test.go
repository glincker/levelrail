package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/dnszones"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

const verifyTestToken = "cf-token-must-never-leak-0123456789" //nolint:gosec // fake token to prove it never leaks

// verifyFake is a Provider whose list calls return canned results.
type verifyFake struct {
	dnszones.Provider
	zones      []dnszones.Zone
	zonesErr   error
	recordsErr error
}

func (f *verifyFake) Name() string { return dnszones.ProviderCloudflare }
func (f *verifyFake) ListZones(context.Context) ([]dnszones.Zone, error) {
	return f.zones, f.zonesErr
}

func (f *verifyFake) ListRecordSets(context.Context, dnszones.Zone) ([]dnszones.RecordSet, error) {
	return nil, f.recordsErr
}

func putVerifyToken(t *testing.T, fake *verifyFake, setup func(*Router)) (*httptest.ResponseRecorder, string) {
	t.Helper()
	var logs bytes.Buffer
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	rt := NewRouter(logger, testBrand(), db, WithCloudflareDNSSecrets(newFakeCloudflareDNSSecrets()))
	rt.dnsVerifyProvider = func(string) dnszones.Provider { return fake }
	if setup != nil {
		setup(rt)
	}
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	body := `{"enabled":true,"token":"` + verifyTestToken + `"}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/cloudflare-dns", body))
	return rec, logs.String()
}

func decodeVerification(t *testing.T, rec *httptest.ResponseRecorder) cloudflareDNSResource {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var res cloudflareDNSResource
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if !res.HasToken || !res.Enabled {
		t.Fatalf("the credential must be saved whatever the verdict: %+v", res)
	}
	return res
}

func TestDNSVerifyOnSave(t *testing.T) {
	zone := []dnszones.Zone{{ID: "z1", Name: "example.com"}}
	cases := []struct {
		name        string
		fake        *verifyFake
		wantOK      bool
		wantMissing []string
		wantZones   int
	}{
		{"ok", &verifyFake{zones: zone}, true, []string{}, 1},
		{"missing zone read", &verifyFake{zonesErr: &dnszones.CloudflareError{Status: 403, Messages: []string{"9109 Unauthorized"}}}, false, []string{"Zone:Zone:Read"}, 0},
		{"missing dns edit", &verifyFake{zones: zone, recordsErr: &dnszones.CloudflareError{Status: 403}}, false, []string{"Zone:DNS:Edit"}, 1},
		{"invalid token", &verifyFake{zonesErr: &dnszones.CloudflareError{Status: 401}}, false, []string{}, 0},
		{"invalid format", &verifyFake{zonesErr: &dnszones.CloudflareError{Status: 400, Messages: []string{"6003 Invalid request headers"}}}, false, []string{}, 0},
		{"unreachable", &verifyFake{zonesErr: context.DeadlineExceeded}, false, []string{}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec, logs := putVerifyToken(t, c.fake, nil)
			res := decodeVerification(t, rec)
			v := res.Verification
			if v == nil {
				t.Fatal("verification missing")
			}
			if v.OK != c.wantOK || v.ZonesVisible != c.wantZones || !slices.Equal(v.MissingScopes, c.wantMissing) || v.Message == "" {
				t.Fatalf("verification = %+v", v)
			}
			if strings.Contains(rec.Body.String(), verifyTestToken) || strings.Contains(logs, verifyTestToken) {
				t.Fatal("the token leaked into the response or the logs")
			}
			if !strings.Contains(rec.Body.String(), `"missing_scopes":[`) {
				t.Fatal("missing_scopes must always be an array")
			}
		})
	}
	if dnsVerifyTimeout <= 0 {
		t.Fatal("dnsVerifyTimeout must be a positive named constant")
	}
}

func TestDNSVerifyOnSaveSettingOffAndLive(t *testing.T) {
	rec, _ := putVerifyToken(t, &verifyFake{}, func(r *Router) {
		if _, err := r.runtimeSettings.Set(context.Background(), runtimesettings.KeyDNSVerifyTokenOnSave, json.RawMessage("false"), "test"); err != nil {
			t.Fatal(err)
		}
	})
	if res := decodeVerification(t, rec); res.Verification != nil {
		t.Fatalf("setting off must skip verification: %+v", res.Verification)
	}
}

func TestDNSVerifyOnSaveKeepsTokenOptional(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(slog.New(slog.NewTextHandler(discardWriter{}, nil)), testBrand(), db, WithCloudflareDNSSecrets(newFakeCloudflareDNSSecrets()))
	called := false
	rt.dnsVerifyProvider = func(string) dnszones.Provider { called = true; return &verifyFake{} }
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/cloudflare-dns", `{"enabled":false}`))
	if rec.Code != http.StatusOK || called || strings.Contains(rec.Body.String(), "verification") {
		t.Fatalf("no new token means no verification: %d %s", rec.Code, rec.Body.String())
	}
}
