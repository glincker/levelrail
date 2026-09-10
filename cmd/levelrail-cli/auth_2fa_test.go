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

// runTwoFactorCommandOK runs the CLI with args and requires an exitOK
// result, returning stdout for the caller's own assertions. Extracted
// from the identical run-then-check-exitOK boilerplate every
// TestRun_AuthTwoFactor* success case repeated.
func runTwoFactorCommandOK(t *testing.T, args []string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", args, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	return stdout.String()
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

	stdout := runTwoFactorCommandOK(t, []string{
		"auth", "2fa", "enable", "--code", "123456", "--username", "admin", "--password", "x", "--api-url", srv.URL,
	})
	if gotReq.Code != "123456" {
		t.Errorf("request code = %q, want 123456", gotReq.Code)
	}
	if !strings.Contains(stdout, "aaaa-bbbb") || !strings.Contains(stdout, "cccc-dddd") {
		t.Errorf("stdout = %q, want both recovery codes printed", stdout)
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

// TestRun_AuthTwoFactorDisable covers both ways to prove the second
// factor (a live code or a recovery code): same request/response shape,
// differing only in which flag is passed and which twoFactorDisableRequest
// field the server should see set.
func TestRun_AuthTwoFactorDisable(t *testing.T) {
	tests := []struct {
		name         string
		extraArgs    []string
		wantCode     string
		wantRecovery string
	}{
		{name: "with code", extraArgs: []string{"--code", "123456"}, wantCode: "123456"},
		{name: "with recovery code", extraArgs: []string{"--recovery-code", "aaaa-bbbb"}, wantRecovery: "aaaa-bbbb"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotReq twoFactorDisableRequest
			srv := fakeSessionAuthServer(t, "POST", "/api/v1/auth/2fa/disable", func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
					t.Fatalf("decode disable request: %v", err)
				}
				w.WriteHeader(http.StatusNoContent)
			})

			args := append([]string{"auth", "2fa", "disable"}, tt.extraArgs...)
			args = append(args, "--username", "admin", "--password", "x", "--api-url", srv.URL)
			stdout := runTwoFactorCommandOK(t, args)

			if gotReq.Code != tt.wantCode {
				t.Errorf("request code = %q, want %q", gotReq.Code, tt.wantCode)
			}
			if gotReq.RecoveryCode != tt.wantRecovery {
				t.Errorf("request recovery_code = %q, want %q", gotReq.RecoveryCode, tt.wantRecovery)
			}
			if !strings.Contains(stdout, "disabled") {
				t.Errorf("stdout = %q, want a disabled confirmation", stdout)
			}
		})
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

	stdout := runTwoFactorCommandOK(t, []string{
		"auth", "2fa", "recovery-codes", "--code", "654321", "--username", "admin", "--password", "x", "--api-url", srv.URL,
	})
	if gotReq.Code != "654321" {
		t.Errorf("request code = %q, want 654321", gotReq.Code)
	}
	if !strings.Contains(stdout, "eeee-ffff") {
		t.Errorf("stdout = %q, want the new recovery code printed", stdout)
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
