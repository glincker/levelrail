package store

import (
	"context"
	"testing"
)

func TestAppProtection_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if p, err := db.GetAppProtection(ctx, "web"); err != nil || p != nil {
		t.Fatalf("unprotected app = %v, %v, want nil, nil", p, err)
	}
	if err := db.SetAppProtection(ctx, "web", "first", "alice"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := db.SetAppProtection(ctx, "web", "second", "bob"); err != nil {
		t.Fatalf("set twice: %v", err)
	}
	p, err := db.GetAppProtection(ctx, "web")
	if err != nil || p == nil {
		t.Fatalf("get = %v, %v", p, err)
	}
	if p.Reason != "first" || p.ProtectedBy != "alice" || p.ProtectedAt == "" {
		t.Errorf("protection = %+v, want first reason and owner kept", p)
	}
	if cleared, err := db.ClearAppProtection(ctx, "web"); err != nil || !cleared {
		t.Errorf("clear = %v, %v, want true", cleared, err)
	}
	if cleared, _ := db.ClearAppProtection(ctx, "web"); cleared {
		t.Error("second clear reported a row")
	}
}

func TestPreviewSettings_HostTemplateRoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	s := DefaultPreviewAppSettings("web")
	s.HostTemplate = "pr-{number}.{domain}"
	s.WildcardDomain = "preview.example.com"
	if err := db.SavePreviewAppSettings(ctx, s); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := db.GetPreviewAppSettings(ctx, "web")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.HostTemplate != s.HostTemplate || got.WildcardDomain != s.WildcardDomain {
		t.Errorf("settings = %+v, want host template and wildcard domain kept", got)
	}
}
