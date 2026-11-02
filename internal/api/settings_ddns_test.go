package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/ddns"
)

type ddnsMemSecrets map[string]string

func (m ddnsMemSecrets) SetValue(_ context.Context, s, k, v string) error { m[s+k] = v; return nil }
func (m ddnsMemSecrets) Resolve(_ context.Context, s, k string) (string, error) {
	return m[s+k], nil
}
func (m ddnsMemSecrets) Exists(_ context.Context, s, k string) (bool, error) {
	_, ok := m[s+k]
	return ok, nil
}
func (m ddnsMemSecrets) DeleteValue(_ context.Context, s, k string) error { delete(m, s+k); return nil }

func TestDDNSSettingsNeverReturnTarget(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := map[string]string{ddns.EnvAllowPrivate: "on", ddns.EnvProbeURLs: "https://127.0.0.1:1/ip", ddns.EnvProbeQuorum: "1"}
	rt := &Router{}
	rt.SetDDNS(ddns.NewManager(ctx, ddnsMemSecrets{}, func(k string) string { return env[k] }, nil, ddns.Hooks{}))

	put := httptest.NewRecorder()
	rt.handlePutDDNS(put, httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"enabled":true,"target":"https://127.0.0.1/u?key=sekret&ip={ip}"}`)))
	if put.Code != http.StatusOK || strings.Contains(put.Body.String(), "sekret") {
		t.Fatalf("put = %d %s", put.Code, put.Body)
	}
	bad := httptest.NewRecorder()
	rt.handlePutDDNS(bad, httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"target":"https://127.0.0.1/no-placeholder"}`)))
	if bad.Code != http.StatusBadRequest {
		t.Errorf("bad target = %d", bad.Code)
	}
	st := httptest.NewRecorder()
	rt.handleDDNSStatus(st, httptest.NewRequest(http.MethodGet, "/", nil))
	if st.Code != http.StatusOK || !strings.Contains(st.Body.String(), `"running":true`) {
		t.Errorf("status = %d %s", st.Code, st.Body)
	}
}
