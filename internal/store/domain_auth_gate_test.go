package store

import (
	"context"
	"reflect"
	"testing"
)

func TestDomainAuthGate_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{
		Name: "web", Image: "img:v1", Port: 80, Domains: []string{"app.example.com"},
	}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	if _, found, err := db.GetDomainAuthGate(ctx, "app.example.com"); err != nil || found {
		t.Fatalf("GetDomainAuthGate() before set = found %v, err %v; want not found", found, err)
	}

	want := DomainAuthGate{
		Domain: "app.example.com", Enabled: true, AuthURL: "http://auth:9091", CheckPath: "/check",
		CopyHeaders: []string{"X-User", "X-Email"}, TimeoutSeconds: 4,
	}
	if err := db.SetDomainAuthGate(ctx, want); err != nil {
		t.Fatalf("SetDomainAuthGate() error = %v", err)
	}
	got, found, err := db.GetDomainAuthGate(ctx, "app.example.com")
	if err != nil || !found {
		t.Fatalf("GetDomainAuthGate() = found %v, err %v", found, err)
	}
	if got.UpdatedAt.IsZero() {
		t.Error("UpdatedAt is zero")
	}
	got.UpdatedAt = want.UpdatedAt
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GetDomainAuthGate() = %+v, want %+v", got, want)
	}

	want.Enabled, want.CopyHeaders = false, nil
	if err := db.SetDomainAuthGate(ctx, want); err != nil {
		t.Fatalf("SetDomainAuthGate() update error = %v", err)
	}
	list, err := db.ListDomainAuthGates(ctx)
	if err != nil || len(list) != 1 || list[0].Enabled || len(list[0].CopyHeaders) != 0 {
		t.Fatalf("ListDomainAuthGates() = %+v, %v; want one disabled row with no headers", list, err)
	}

	if err := db.DeleteDomainAuthGate(ctx, "app.example.com"); err != nil {
		t.Fatalf("DeleteDomainAuthGate() error = %v", err)
	}
	if err := db.DeleteDomainAuthGate(ctx, "app.example.com"); err != nil {
		t.Fatalf("DeleteDomainAuthGate() second call error = %v", err)
	}
	if _, found, _ := db.GetDomainAuthGate(ctx, "app.example.com"); found {
		t.Error("found after delete")
	}
}

func TestDomainAuthGate_RequiresOwnerAndIsReleased(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SetDomainAuthGate(ctx, DomainAuthGate{Domain: "nobody.example.com", AuthURL: "http://auth"}); err == nil {
		t.Fatal("SetDomainAuthGate() accepted a domain with no owner")
	}
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"app.example.com"}}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetDomainAuthGate(ctx, DomainAuthGate{Domain: "app.example.com", Enabled: true, AuthURL: "http://auth", CheckPath: "/"}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := db.GetDomainAuthGate(ctx, "app.example.com"); found {
		t.Error("auth gate outlived the domain's last owner")
	}
}
