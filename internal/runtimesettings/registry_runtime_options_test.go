package runtimesettings

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/spec"
)

func TestRuntimeOptionBoundsFollowSettings(t *testing.T) {
	t.Cleanup(func() { spec.SetRuntimeLimitSource(nil) })
	svc := NewWithDefinitions(newFakeStore(), envOf(map[string]string{"APP_RUNTIME_DNS_MAX": "2", "APP_RUNTIME_PIDS_LIMIT_MAX": "0"}), Definitions())
	BindRuntimeLimits(svc)
	if b := spec.CurrentRuntimeBounds(); b.DNSMax != 2 || b.PidsLimitMax != 32768 || b.TmpfsMax != 16 {
		t.Fatalf("seed bounds = %+v, want dns 2, pids default, tmpfs default", b)
	}
	if _, err := svc.Set(context.Background(), KeyRuntimeDNSMax, raw("5"), "tester"); err != nil {
		t.Fatal(err)
	}
	if got := spec.CurrentRuntimeBounds().DNSMax; got != 5 {
		t.Fatalf("dns max after save = %d, want 5 without restart", got)
	}
	if _, err := svc.Set(context.Background(), KeyRuntimeDNSMax, raw("99"), "tester"); err == nil {
		t.Fatal("value above the ceiling was accepted")
	}
}
