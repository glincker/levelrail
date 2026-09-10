package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestRun_AuthTwoFactorStatus(t *testing.T) {
	tests := []struct {
		name       string
		enabled    bool
		remaining  int
		wantOutput string
	}{
		{name: "disabled", enabled: false, wantOutput: "disabled"},
		{name: "enabled", enabled: true, remaining: 7, wantOutput: "recovery codes remaining: 7"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := fakeSessionAuthServer(t, "GET", "/api/v1/auth/2fa", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(twoFactorStatusResponse{Enabled: tt.enabled, RecoveryCodesRemaining: tt.remaining})
			})

			var stdout, stderr bytes.Buffer
			got := run("levelrail-cli-test", []string{
				"auth", "2fa", "status", "--username", "admin", "--password", "x", "--api-url", srv.URL,
			}, &stdout, &stderr, envMap())
			if got != exitOK {
				t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
			}
			if !strings.Contains(stdout.String(), tt.wantOutput) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tt.wantOutput)
			}
		})
	}
}

func TestRun_AuthTwoFactorSetup(t *testing.T) {
	srv := fakeSessionAuthServer(t, "POST", "/api/v1/auth/2fa/setup", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(twoFactorSetupResponse{Secret: "JBSWY3DPEHPK3PXP", ProvisioningURI: "otpauth://totp/example"}) //nolint:gosec // fake test fixture, not a real secret
	})

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"auth", "2fa", "setup", "--username", "admin", "--password", "x", "--api-url", srv.URL,
	}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "JBSWY3DPEHPK3PXP") {
		t.Errorf("stdout = %q, want the secret printed", stdout.String())
	}
	if !strings.Contains(stdout.String(), "otpauth://totp/example") {
		t.Errorf("stdout = %q, want the provisioning URI printed", stdout.String())
	}
}

func TestRun_AuthTwoFactorEnable(t *testing.T) {
	var gotReq twoFactorCodeRequest
	srv := fakeSessionAuthServer(t, "POST", "/api/v1/auth/2fa/confirm", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatalf("decode confirm request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(twoFactorRecoveryCodesResponse{RecoveryCodes: []string{"aaaa-bbbb", "cccc-dddd"}})
	})

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"auth", "2fa", "enable", "--code", "123456", "--username", "admin", "--password", "x", "--api-url", srv.URL,
	}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotReq.Code != "123456" {
		t.Errorf("request code = %q, want 123456", gotReq.Code)
	}
	if !strings.Contains(stdout.String(), "aaaa-bbbb") || !strings.Contains(stdout.String(), "cccc-dddd") {
		t.Errorf("stdout = %q, want both recovery codes printed", stdout.String())
	}
}

func TestRun_AuthTwoFactorEnable_MissingCode(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"auth", "2fa", "enable", "--username", "admin", "--password", "x"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitValidation, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "--code is required") {
		t.Errorf("stderr = %q, want a missing-code validation error", stderr.String())
	}
}

func TestRun_AuthTwoFactorDisable(t *testing.T) {
	var gotReq twoFactorDisableRequest
	srv := fakeSessionAuthServer(t, "POST", "/api/v1/auth/2fa/disable", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatalf("decode disable request: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"auth", "2fa", "disable", "--code", "123456", "--username", "admin", "--password", "x", "--api-url", srv.URL,
	}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotReq.Code != "123456" {
		t.Errorf("request code = %q, want 123456", gotReq.Code)
	}
	if !strings.Contains(stdout.String(), "disabled") {
		t.Errorf("stdout = %q, want a disabled confirmation", stdout.String())
	}
}

func TestRun_AuthTwoFactorDisable_WithRecoveryCode(t *testing.T) {
	var gotReq twoFactorDisableRequest
	srv := fakeSessionAuthServer(t, "POST", "/api/v1/auth/2fa/disable", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatalf("decode disable request: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"auth", "2fa", "disable", "--recovery-code", "aaaa-bbbb", "--username", "admin", "--password", "x", "--api-url", srv.URL,
	}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotReq.RecoveryCode != "aaaa-bbbb" {
		t.Errorf("request recovery_code = %q, want aaaa-bbbb", gotReq.RecoveryCode)
	}
}

func TestRun_AuthTwoFactorDisable_MissingCode(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"auth", "2fa", "disable", "--username", "admin", "--password", "x"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitValidation, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "either --code or --recovery-code is required") {
		t.Errorf("stderr = %q, want a missing-code validation error", stderr.String())
	}
}

func TestRun_AuthTwoFactorRecoveryCodes(t *testing.T) {
	var gotReq twoFactorCodeRequest
	srv := fakeSessionAuthServer(t, "POST", "/api/v1/auth/2fa/recovery-codes/regenerate", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatalf("decode regenerate request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(twoFactorRecoveryCodesResponse{RecoveryCodes: []string{"eeee-ffff"}})
	})

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"auth", "2fa", "recovery-codes", "--code", "654321", "--username", "admin", "--password", "x", "--api-url", srv.URL,
	}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotReq.Code != "654321" {
		t.Errorf("request code = %q, want 654321", gotReq.Code)
	}
	if !strings.Contains(stdout.String(), "eeee-ffff") {
		t.Errorf("stdout = %q, want the new recovery code printed", stdout.String())
	}
}

func TestRun_AuthTwoFactorRecoveryCodes_MissingCode(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"auth", "2fa", "recovery-codes", "--username", "admin", "--password", "x"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitValidation, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "--code is required") {
		t.Errorf("stderr = %q, want a missing-code validation error", stderr.String())
	}
}

func TestRun_AuthTwoFactor_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"auth", "2fa", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stdout.String(), "auth 2fa status") {
		t.Errorf("stdout = %q, want usage text", stdout.String())
	}
}

func TestRun_AuthTwoFactor_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"auth", "2fa", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown auth 2fa subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderr.String())
	}
}

func TestRun_AuthTwoFactor_NoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"auth", "2fa"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}
