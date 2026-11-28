package store

import (
	"context"
	"errors"
	"testing"
)

func TestServiceNameGuardRejectsNamespaceNames(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	ctx := context.Background()
	hostile := []string{"deleted-app/web", "git-source/x", "mail-tls-cert/x", "user-totp/x", "pr-env/..", "a\\b", "a b", "a\x00b", "a\nb", "a..b"}
	for _, name := range hostile {
		err := db.SaveDesiredService(ctx, DesiredService{Name: name, Image: "img:1", Port: 80})
		if !errors.Is(err, ErrInvalidServiceName) {
			t.Errorf("SaveDesiredService(%q) = %v, want ErrInvalidServiceName", name, err)
		}
		err = db.SaveSuspendedDesiredService(ctx, DesiredService{Name: name, Image: "img:1", Port: 80})
		if !errors.Is(err, ErrInvalidServiceName) {
			t.Errorf("SaveSuspendedDesiredService(%q) = %v, want ErrInvalidServiceName", name, err)
		}
	}
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web-1", Image: "img:1", Port: 80}); err != nil {
		t.Fatalf("plain name refused: %v", err)
	}
}

func TestServiceNameGuardKeepsExistingOffendersSaving(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	// Simulate an old row written before the guard existed.
	if _, err := db.ExecContext(ctx, `UPDATE desired_services SET name = 'old/offender' WHERE name = 'web'`); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "old/offender", Image: "img:2", Port: 80}); err != nil {
		t.Fatalf("existing offender must keep saving: %v", err)
	}
	bad, err := db.ServiceNamesBreakingNamespaceRule(ctx)
	if err != nil || len(bad) != 1 || bad[0] != "old/offender" {
		t.Fatalf("offenders = %v, %v", bad, err)
	}
}
