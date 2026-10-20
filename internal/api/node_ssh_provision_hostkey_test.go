package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/sshprovision"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHandleCreateSSHNodeProvisionHostKeyPolicy(t *testing.T) {
	base := `"host":"192.0.2.10","username":"root","auth":{"type":"password","password":"s3cret"},"name":"ssh-hk","control_plane_addr":"cp.example:9443"`
	tests := []struct {
		name         string
		extra        string
		wantStatus   int
		wantFP       string
		wantInsecure bool
	}{
		{"missing fingerprint is refused", ``, http.StatusBadRequest, "", false},
		{"malformed fingerprint", `,"host_key_fingerprint":"MD5:aa:bb"`, http.StatusBadRequest, "", false},
		{"both fingerprint and insecure", `,"host_key_fingerprint":"` + testHostKeyFingerprint + `","insecure_skip_host_key_check":true`, http.StatusBadRequest, "", false},
		{"pinned fingerprint", `,"host_key_fingerprint":"` + testHostKeyFingerprint + `"`, http.StatusCreated, testHostKeyFingerprint, false},
		{"pinned without prefix", `,"host_key_fingerprint":"` + strings.TrimPrefix(testHostKeyFingerprint, "SHA256:") + `"`, http.StatusCreated, testHostKeyFingerprint, false},
		{"explicit insecure skip", `,"insecure_skip_host_key_check":true`, http.StatusCreated, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeSSHProvisioner{events: []sshprovision.Event{{Step: sshprovision.StepDone, Message: "done"}}}
			rt, db := newTestRouterWithSSHProvisioning(t, fake)
			cookie := loginTestSession(t, rt, db)
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/nodes/ssh-provision", "{"+base+tc.extra+"}"))
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.wantStatus != http.StatusCreated {
				return
			}
			var created sshNodeProvisionResource
			if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if created.HostKeyFingerprint != tc.wantFP || created.HostKeyVerified == tc.wantInsecure {
				t.Fatalf("created = %+v", created)
			}
			if tc.wantInsecure != (len(created.Warnings) == 1) {
				t.Fatalf("warnings = %v, insecure = %v", created.Warnings, tc.wantInsecure)
			}
			awaitSSHProvisionStatus(t, db, created.ID, store.SSHNodeProvisionStatusEnrolling)
			if fake.lastCreds.HostKeyFingerprint != tc.wantFP || fake.lastCreds.InsecureSkipHostKeyCheck != tc.wantInsecure {
				t.Fatalf("provisioner creds host key policy = %q/%v", fake.lastCreds.HostKeyFingerprint, fake.lastCreds.InsecureSkipHostKeyCheck)
			}
			entries, err := db.ListAuditEntries(context.Background(), 50, nil, store.AuditEntryFilter{Action: store.AuditActionSSHProvisionHostKeySkipped})
			if err != nil {
				t.Fatalf("list audit: %v", err)
			}
			if tc.wantInsecure != (len(entries) == 1) {
				t.Fatalf("host key skipped audit rows = %d, insecure = %v", len(entries), tc.wantInsecure)
			}
		})
	}
}

func TestHandleSSHProvisionPreflight(t *testing.T) {
	keys := []sshprovision.HostKey{{Type: "ssh-ed25519", Fingerprint: testHostKeyFingerprint}}
	tests := []struct {
		name       string
		body       string
		scanErr    error
		wantStatus int
		wantScan   bool
	}{
		{"returns observed keys", `{"host":"192.0.2.10","port":2222}`, nil, http.StatusOK, true},
		{"default port", `{"host":"192.0.2.10"}`, nil, http.StatusOK, true},
		{"missing host", `{"port":22}`, nil, http.StatusBadRequest, false},
		{"bad port", `{"host":"192.0.2.10","port":70000}`, nil, http.StatusBadRequest, false},
		{"unreachable", `{"host":"192.0.2.10"}`, context.DeadlineExceeded, http.StatusBadGateway, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeSSHProvisioner{scanKeys: keys, scanErr: tc.scanErr}
			rt, db := newTestRouterWithSSHProvisioning(t, fake)
			cookie := loginTestSession(t, rt, db)
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/nodes/ssh-provision/preflight", tc.body))
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if (fake.scanCalls > 0) != tc.wantScan {
				t.Fatalf("scan calls = %d, want scan %v", fake.scanCalls, tc.wantScan)
			}
			if tc.wantStatus != http.StatusOK {
				return
			}
			var out sshProvisionPreflightResource
			if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if len(out.HostKeys) != 1 || out.HostKeys[0].Fingerprint != testHostKeyFingerprint || out.HostKeys[0].Type != "ssh-ed25519" {
				t.Fatalf("preflight = %+v", out)
			}
		})
	}
}

func TestHandleSSHProvisionPreflightRequiresAuth(t *testing.T) {
	fake := &fakeSSHProvisioner{}
	rt, _ := newTestRouterWithSSHProvisioning(t, fake)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/ssh-provision/preflight", strings.NewReader(`{"host":"192.0.2.10"}`))
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want 401", rec.Code)
	}
	if fake.scanCalls != 0 {
		t.Fatalf("unauthenticated request reached the scanner")
	}
}

func TestHandleSSHProvisionPreflightRequiresRoot(t *testing.T) {
	fake := &fakeSSHProvisioner{}
	rt, db := newTestRouterWithSSHProvisioning(t, fake)
	for _, ability := range []string{AbilityRead, AbilityDeploy, AbilityWriteSensitive} {
		t.Run(ability, func(t *testing.T) {
			plain := seedMatrixToken(t, db, "tok-"+ability, []string{ability})
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/ssh-provision/preflight", strings.NewReader(`{"host":"192.0.2.10"}`))
			req.Header.Set("Authorization", "Bearer "+plain)
			rt.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403", rec.Code)
			}
		})
	}
	if fake.scanCalls != 0 {
		t.Fatalf("non-root token reached the scanner")
	}
}
