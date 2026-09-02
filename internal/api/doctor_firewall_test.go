package api

import (
	"context"
	"errors"
	"os/exec"
	"testing"
)

func TestDoctorCheckFirewall_NotInstalled(t *testing.T) {
	lookPath := func(string) (string, error) { return "", errors.New("not found") }
	run := func(context.Context, string, ...string) ([]byte, error) {
		t.Fatal("run should not be called when ufw isn't on PATH")
		return nil, nil
	}
	got := doctorCheckFirewall(context.Background(), lookPath, run)
	if got.Status != doctorStatusUnknown {
		t.Errorf("Status = %q, want %q", got.Status, doctorStatusUnknown)
	}
}

func TestDoctorCheckFirewall_ActiveDenyIncoming(t *testing.T) {
	lookPath := func(string) (string, error) { return "/usr/sbin/ufw", nil }
	run := func(context.Context, string, ...string) ([]byte, error) {
		return []byte("Status: active\nLogging: on (low)\nDefault: deny (incoming), allow (outgoing), disabled (routed)\n"), nil
	}
	got := doctorCheckFirewall(context.Background(), lookPath, run)
	if got.Status != doctorStatusOK {
		t.Errorf("Status = %q, want %q, message = %q", got.Status, doctorStatusOK, got.Message)
	}
}

func TestDoctorCheckFirewall_ActiveAllowIncoming(t *testing.T) {
	lookPath := func(string) (string, error) { return "/usr/sbin/ufw", nil }
	run := func(context.Context, string, ...string) ([]byte, error) {
		return []byte("Status: active\nDefault: allow (incoming), allow (outgoing), disabled (routed)\n"), nil
	}
	got := doctorCheckFirewall(context.Background(), lookPath, run)
	if got.Status != doctorStatusWarn {
		t.Errorf("Status = %q, want %q (allow incoming is a real warning), message = %q", got.Status, doctorStatusWarn, got.Message)
	}
}

func TestDoctorCheckFirewall_InstalledButInactive(t *testing.T) {
	lookPath := func(string) (string, error) { return "/usr/sbin/ufw", nil }
	run := func(context.Context, string, ...string) ([]byte, error) {
		return []byte("Status: inactive\n"), nil
	}
	got := doctorCheckFirewall(context.Background(), lookPath, run)
	if got.Status != doctorStatusWarn {
		t.Errorf("Status = %q, want %q, message = %q", got.Status, doctorStatusWarn, got.Message)
	}
}

func TestDoctorCheckFirewall_CommandFails(t *testing.T) {
	lookPath := func(string) (string, error) { return "/usr/sbin/ufw", nil }
	run := func(context.Context, string, ...string) ([]byte, error) {
		return nil, &exec.ExitError{}
	}
	got := doctorCheckFirewall(context.Background(), lookPath, run)
	if got.Status != doctorStatusUnknown {
		t.Errorf("Status = %q, want %q", got.Status, doctorStatusUnknown)
	}
}

func TestDoctorCheckFirewallCtx_RealDependencies(t *testing.T) {
	// Smoke test only: proves the real exec.LookPath/exec.CommandContext
	// wiring doesn't panic or hang, whatever ufw's actual state on the
	// machine running this test happens to be (CI runners typically
	// don't have ufw installed at all, which is itself a valid,
	// non-failing outcome per doctorCheckFirewall's own doc comment).
	got := doctorCheckFirewallCtx(context.Background())
	if got.Code != "firewall" {
		t.Errorf("Code = %q, want %q", got.Code, "firewall")
	}
	if got.Status == doctorStatusFail {
		t.Errorf("Status = %q, want never fail (informational check only)", got.Status)
	}
}
