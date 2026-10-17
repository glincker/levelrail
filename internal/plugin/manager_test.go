package plugin

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestAbilityApprovalGating(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	if _, err := h.m.Install(ctx, h.request(manifestYAML("[read]")), "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.m.Enable(ctx, "example", "admin"); !errors.Is(err, ErrNotApproved) {
		t.Fatalf("Enable before approval = %v, want ErrNotApproved", err)
	}
	if _, err := h.m.Approve(ctx, "example", []string{AbilityReadSensitive}, "admin"); err == nil {
		t.Fatal("approved an ability the manifest never requested")
	}
	if _, err := h.m.Approve(ctx, "example", []string{"root"}, "admin"); err == nil {
		t.Fatal("approved root")
	}
	if _, err := h.m.Approve(ctx, "example", nil, "admin"); err == nil {
		t.Fatal("empty approval accepted")
	}
	if _, err := h.m.Enable(ctx, "example", "admin"); !errors.Is(err, ErrNotApproved) {
		t.Fatalf("Enable after only refused approvals = %v, want ErrNotApproved", err)
	}
	if _, err := h.m.Approve(ctx, "example", []string{AbilityRead}, "admin"); err != nil {
		t.Fatal(err)
	}
	rec, err := h.m.Enable(ctx, "example", "admin")
	if err != nil || rec.State != StateEnabled {
		t.Fatalf("Enable after approval = %+v, %v", rec, err)
	}
}

func TestLifecycleAuditEntries(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.installApprovedEnabled("[read]", []string{AbilityRead})
	if _, err := h.m.Disable(ctx, "example", "admin"); err != nil {
		t.Fatal(err)
	}
	if err := h.m.Remove(ctx, "example", "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.m.TrustAdd(ctx, "second", b64(mustPub(t)), "admin"); err != nil {
		t.Fatal(err)
	}
	keys, _ := h.m.TrustKeys(ctx)
	if err := h.m.TrustRemove(ctx, keys[len(keys)-1].KeyID, "admin"); err != nil {
		t.Fatal(err)
	}
	want := []string{AuditInstall, AuditApprove, AuditEnable, AuditDisable, AuditRemove, AuditTrustAdd, AuditTrustRemove}
	if got := h.audit.actions(); !reflect.DeepEqual(got, want) {
		t.Fatalf("audit actions = %v, want %v", got, want)
	}
	for _, e := range h.audit.events {
		for _, v := range e.Detail {
			if v == "" {
				t.Fatalf("audit detail has an empty value: %+v", e)
			}
		}
	}
}

func TestInstallRejectsDuplicateAndBadConfig(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	req := h.request(manifestYAML("[read]"))
	if _, err := h.m.Install(ctx, req, "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.m.Install(ctx, req, "admin"); !errors.Is(err, ErrExists) {
		t.Fatalf("duplicate install = %v, want ErrExists", err)
	}
	h2 := newHarness(t)
	req2 := h2.request(manifestYAML("[read]"))
	req2.ConfigJSON = `["not","an","object"]`
	if _, err := h2.m.Install(ctx, req2, "admin"); err == nil {
		t.Fatal("non-object config accepted")
	}
	req2.ConfigJSON = `{"pad":"` + string(make([]byte, 20<<10)) + `"}`
	if _, err := h2.m.Install(ctx, req2, "admin"); err == nil {
		t.Fatal("oversized config accepted")
	}
}
