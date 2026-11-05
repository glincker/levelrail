package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/hostprovider"
)

// fakeMetadataDeps routes every metadata GET to a fake server that answers
// only the one path (and header) a single provider owns.
func fakeMetadataDeps(t *testing.T, path, headerKey, headerVal string) hostprovider.Deps {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if path == "" || r.URL.Path != path || (headerKey != "" && r.Header.Get(headerKey) != headerVal) {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("instance-1"))
	}))
	t.Cleanup(srv.Close)
	base, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	return hostprovider.Deps{
		ReadFile: func(string) ([]byte, error) { return nil, errors.New("no dmi") },
		Get: func(ctx context.Context, raw string, h http.Header) (int, []byte, error) {
			u, err := url.Parse(raw)
			if err != nil {
				return 0, nil, err
			}
			u.Scheme, u.Host = base.Scheme, base.Host
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
			if err != nil {
				return 0, nil, err
			}
			req.Header = h
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return 0, nil, err
			}
			defer func() { _ = resp.Body.Close() }()
			b, err := io.ReadAll(resp.Body)
			return resp.StatusCode, b, err
		},
	}
}

func TestDoctorCloudWarnsOnBadProviderOverride(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "override.json")
	if err := os.WriteFile(bad, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(hostprovider.EnvDataFile, bad)
	var out, errb bytes.Buffer
	if code := runDoctorCloud("levelrail-cli", nil, &out, &errb, fakeMetadataDeps(t, "", "", "")); code != exitOK {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if !strings.Contains(errb.String(), "provider data override ignored") || !strings.Contains(errb.String(), bad) {
		t.Fatalf("missing warning: %q", errb.String())
	}
}

func TestDoctorCloudRules(t *testing.T) {
	tests := []struct {
		name, path, hk, hv string
		wantProvider       string
		wantConsole        string
	}{
		{"oracle", "/opc/v2/instance/id", "Authorization", "Bearer Oracle", "oracle", "https://cloud.oracle.com/"},
		{"aws", "/latest/meta-data/instance-id", "", "", "aws", "https://console.aws.amazon.com/"},
		{"gcp", "/computeMetadata/v1/instance/id", "Metadata-Flavor", "Google", "gcp", "https://console.cloud.google.com/"},
		{"azure", "/metadata/instance", "Metadata", "true", "azure", "https://portal.azure.com/"},
		{"digitalocean", "/metadata/v1/id", "", "", "digitalocean", "https://cloud.digitalocean.com/"},
		{"hetzner", "/hetzner/v1/metadata", "", "", "hetzner", "https://console.hetzner.cloud/"},
		{"unknown", "", "", "", "unknown", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			deps := fakeMetadataDeps(t, tc.path, tc.hk, tc.hv)
			var out, errb bytes.Buffer
			if code := runDoctorCloud("levelrail-cli", nil, &out, &errb, deps); code != exitOK {
				t.Fatalf("exit %d: %s", code, errb.String())
			}
			text := out.String()
			if tc.wantProvider == "unknown" {
				if !strings.Contains(text, "Allow inbound TCP 80 and 443") || strings.Contains(text, "Console:") {
					t.Fatalf("generic output wrong: %q", text)
				}
				return
			}
			for _, want := range []string{"levelrail-http", "TCP 80", "levelrail-https", "TCP 443", "Console: " + tc.wantConsole} {
				if !strings.Contains(text, want) {
					t.Fatalf("missing %q in %q", want, text)
				}
			}
			var js bytes.Buffer
			if code := runDoctorCloud("levelrail-cli", []string{"--json"}, &js, &errb, deps); code != exitOK || !strings.Contains(js.String(), `"provider": "`+tc.wantProvider+`"`) {
				t.Fatalf("json wrong (%d): %s", code, js.String())
			}
		})
	}
}
