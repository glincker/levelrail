package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSecurityPolicyRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	if _, ok, err := db.GetSecurityPolicy(ctx); err != nil || ok {
		t.Fatalf("fresh policy ok=%v err=%v, want no row", ok, err)
	}
	scope, days := "all_methods", 90
	if err := db.SaveSecurityPolicy(ctx, SecurityPolicy{ApprovalScope: &scope, MaxTokenLifetimeDays: &days, UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	p, ok, err := db.GetSecurityPolicy(ctx)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if p.ApprovalScope == nil || *p.ApprovalScope != scope || p.MaxTokenLifetimeDays == nil || *p.MaxTokenLifetimeDays != 90 {
		t.Fatalf("policy = %+v", p)
	}
	if p.WarnUnusedDays != nil || p.DisableUnusedDays != nil {
		t.Fatalf("unset fields must stay nil: %+v", p)
	}
}

func TestUserSecuritySettings(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	now := time.Now()
	tests := []struct {
		name string
		do   func() error
		want UserSecuritySettings
	}{
		{"unset is zero", func() error { return nil }, UserSecuritySettings{UserID: "u1"}},
		{"require approval", func() error { return db.SetRequireNewDeviceApproval(ctx, "u1", true, now) }, UserSecuritySettings{UserID: "u1", RequireNewDeviceApproval: true}},
		{"flag keeps switch", func() error { return db.FlagUserForReset(ctx, "u1", "not_me", now) }, UserSecuritySettings{UserID: "u1", RequireNewDeviceApproval: true, ResetFlagReason: "not_me"}},
		{"clear flag", func() error { return db.ClearUserResetFlag(ctx, "u1", now) }, UserSecuritySettings{UserID: "u1", RequireNewDeviceApproval: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.do(); err != nil {
				t.Fatal(err)
			}
			got, err := db.GetUserSecuritySettings(ctx, "u1")
			if err != nil {
				t.Fatal(err)
			}
			if got.RequireNewDeviceApproval != tt.want.RequireNewDeviceApproval || got.ResetFlagReason != tt.want.ResetFlagReason ||
				(tt.want.ResetFlagReason != "") == got.ResetFlaggedAt.IsZero() {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestSignInAlertTokenSingleUse(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	now := time.Now()
	for _, tok := range []SignInAlertToken{
		{ID: "live", UserID: "u1", SessionID: "s1", CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
		{ID: "old", UserID: "u1", CreatedAt: now.Add(-2 * time.Hour), ExpiresAt: now.Add(-time.Hour)},
	} {
		if err := db.CreateSignInAlertToken(ctx, tok); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"first use", "live", false},
		{"replay", "live", true},
		{"expired", "old", true},
		{"unknown", "nope", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := db.ConsumeSignInAlertToken(ctx, tt.id, now)
			if tt.wantErr {
				if !errors.Is(err, ErrSignInAlertTokenUsed) {
					t.Fatalf("err = %v, want used", err)
				}
				return
			}
			if err != nil || got.SessionID != "s1" {
				t.Fatalf("got %+v, %v", got, err)
			}
		})
	}
}

func TestTokenHygieneNotices(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	now := time.Now()
	if err := db.SaveTokenHygieneNotice(ctx, TokenHygieneNotice{TokenID: "t1", NoticedAt: now.Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveTokenHygieneNotice(ctx, TokenHygieneNotice{TokenID: "t1", NoticedAt: now}); err != nil {
		t.Fatal(err)
	}
	list, err := db.ListTokenHygieneNotices(ctx)
	if err != nil || len(list) != 1 || !list[0].NoticedAt.Before(now) {
		t.Fatalf("a repeat notice must keep the first time: %+v, %v", list, err)
	}
	if err := db.MarkTokenHygieneDisabled(ctx, "t1", now); err != nil {
		t.Fatal(err)
	}
	if list, _ = db.ListTokenHygieneNotices(ctx); list[0].DisabledAt.IsZero() {
		t.Fatal("disabled_at not set")
	}
	if err := db.DeleteTokenHygieneNotice(ctx, "t1"); err != nil {
		t.Fatal(err)
	}
	if list, _ = db.ListTokenHygieneNotices(ctx); len(list) != 0 {
		t.Fatalf("notice not deleted: %+v", list)
	}
}

func TestRememberBrowser(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	now := time.Now()
	tests := []struct {
		name, user, fp   string
		wantNew, wantAny bool
	}{
		{"first browser of an account", "u1", "a", true, false},
		{"same browser again", "u1", "a", false, true},
		{"second browser", "u1", "b", true, true},
		{"other account starts fresh", "u2", "a", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isNew, hadAny, err := db.RememberBrowser(ctx, tt.user, tt.fp, now)
			if err != nil || isNew != tt.wantNew || hadAny != tt.wantAny {
				t.Fatalf("got new=%v any=%v err=%v", isNew, hadAny, err)
			}
		})
	}
	if err := db.ForgetBrowser(ctx, "u1", "b"); err != nil {
		t.Fatal(err)
	}
	if isNew, _, _ := db.RememberBrowser(ctx, "u1", "b", now); !isNew {
		t.Fatal("a forgotten browser must read as new")
	}
}
