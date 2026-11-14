package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// fakeOracleAPI stands in for the control plane; it only records what the CLI sent.
func fakeOracleAPI(t *testing.T, calls *atomic.Int32, gotKey *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/settings/cloud-connections/oracle" {
			http.NotFound(w, r)
			return
		}
		calls.Add(1)
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		*gotKey = body["private_key_pem"]
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"name":"` + body["name"] + `","provider":"oracle","auth_method":"static","fingerprint_prefix":"` + body["fingerprint"][:11] + `"}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func apiSinkDeps(t *testing.T, r *fakeOCIRunner, srvURL string, out *bytes.Buffer, env map[string]string) ociConnectDeps {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	d := ociTestDeps(t, r, &fakeOCISink{}, env)
	d.sink = func(string, string) ociCredentialSink {
		return apiOCISink{prog: "cli", apiURL: srvURL, lookupEnv: d.lookup, out: out}
	}
	return d
}

func TestCloudConnectOCIYesStoresThroughAPI(t *testing.T) {
	var calls atomic.Int32
	var gotKey string
	srv := fakeOracleAPI(t, &calls, &gotKey)
	k := connectTestKey(t)
	var sinkOut bytes.Buffer
	r := &fakeOCIRunner{t: t}
	code, out, errOut := runConnect(t, apiSinkDeps(t, r, srv.URL, &sinkOut, map[string]string{"APP_API_TOKEN": "test-token"}), "--region", "us-phoenix-1", "--yes")
	if code != exitOK || calls.Load() != 1 {
		t.Fatalf("code %d calls %d: %s", code, calls.Load(), errOut)
	}
	if !strings.Contains(gotKey, keyBody(k)) {
		t.Fatal("server did not receive the generated key")
	}
	all := out + errOut + sinkOut.String()
	if strings.Contains(all, keyBody(k)) || !strings.Contains(sinkOut.String(), fpOf(t, &k.PublicKey)[:11]) {
		t.Fatalf("output wrong or leaks the key: %s", sinkOut.String())
	}
}

func TestCloudConnectOCIPasteStoresThroughAPI(t *testing.T) {
	var calls atomic.Int32
	var gotKey string
	srv := fakeOracleAPI(t, &calls, &gotKey)
	k := connectTestKey(t)
	env := map[string]string{"APP_API_TOKEN": "test-token", "OCI_KEY": privPEMOf(k)}
	var sinkOut bytes.Buffer
	args := []string{"--paste", "--name", "oci-paste", "--tenancy", tTenancy, "--region", "us-phoenix-1", "--user-ocid", tUser,
		"--fingerprint", fpOf(t, &k.PublicKey), "--key-env", "OCI_KEY", "--compartment-ocid", tComp}
	code, _, errOut := runConnect(t, apiSinkDeps(t, &fakeOCIRunner{t: t}, srv.URL, &sinkOut, env), args...)
	if code != exitOK || calls.Load() != 0 {
		t.Fatalf("dry run called the server: code %d calls %d: %s", code, calls.Load(), errOut)
	}
	code, _, errOut = runConnect(t, apiSinkDeps(t, &fakeOCIRunner{t: t}, srv.URL, &sinkOut, env), append(args, "--yes")...)
	if code != exitOK || calls.Load() != 1 || !strings.Contains(gotKey, keyBody(k)) || !strings.Contains(sinkOut.String(), "oci-paste") {
		t.Fatalf("paste --yes: code %d calls %d: %s %s", code, calls.Load(), errOut, sinkOut.String())
	}
}

func TestCloudConnectOCIDryRunNeverCallsAPI(t *testing.T) {
	var calls atomic.Int32
	var gotKey string
	srv := fakeOracleAPI(t, &calls, &gotKey)
	var sinkOut bytes.Buffer
	code, _, errOut := runConnect(t, apiSinkDeps(t, &fakeOCIRunner{t: t}, srv.URL, &sinkOut, map[string]string{"APP_API_TOKEN": "test-token"}), "--region", "us-phoenix-1")
	if code != exitOK || calls.Load() != 0 {
		t.Fatalf("code %d calls %d: %s", code, calls.Load(), errOut)
	}
}
