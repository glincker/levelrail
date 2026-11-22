package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/macosrt"
)

func TestLaunchdPrintAgent(t *testing.T) {
	var out, errb bytes.Buffer
	code := runLaunchd("cli", []string{"print", "agent", "--binary", "/usr/local/bin/levelrail-agent", "--env", "APP_CONTROL_PLANE_ADDR=h:1"}, &out, &errb, vmEnvMap(nil))
	if code != exitOK || !strings.Contains(out.String(), "com.levelrail.agent") || !strings.Contains(out.String(), "h:1") {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errb.String())
	}
}

func TestLaunchdRefusesSecretsAndBadRole(t *testing.T) {
	var out, errb bytes.Buffer
	if code := runLaunchd("cli", []string{"print", "agent", "--binary", "/b", "--env", "APP_JOIN_TOKEN=x"}, &out, &errb, vmEnvMap(nil)); code != exitUsage {
		t.Fatalf("secret env accepted: %d", code)
	}
	if code := runLaunchd("cli", []string{"print", "nope", "--binary", "/b"}, &out, &errb, vmEnvMap(nil)); code != exitUsage {
		t.Fatalf("bad role accepted: %d", code)
	}
}

func TestMacDoctorReportFailsOnMissingLima(t *testing.T) {
	d := macosrt.Deps{GOOS: "darwin", GOARCH: "arm64", MinFreeGiB: 1, MinMacOSMajor: 13}
	d.LookPath = func(string) (string, error) { return "", errors.New("missing") }
	d.Sysctl = func(string) (string, error) { return "1", nil }
	d.Version = func() (string, error) { return "15.0", nil }
	d.FreeBytes = func(string) (int64, error) { return 50 << 30, nil }
	d.PortFree = func(int) bool { return true }
	if rep := macDoctorReport(d); rep.OK {
		t.Fatal("expected not ok without limactl")
	}
}

func TestVMLogsTailsGuestUnit(t *testing.T) {
	e, f := runningVM(t)
	code, _, errs := runVMCmd(e, nil, "logs", "--lines", "5")
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errs)
	}
	found := false
	for _, c := range f.calls {
		if strings.Contains(c, "journalctl -u "+vmGuestUnit+" -n 5") {
			found = true
		}
	}
	if !found {
		t.Fatalf("calls %v", f.calls)
	}
}
