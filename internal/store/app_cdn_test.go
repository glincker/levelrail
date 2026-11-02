package store

import (
	"context"
	"testing"
)

func TestAppDNSProxiedOverride(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	if _, set, err := db.GetAppDNSProxied(ctx, "web"); err != nil || set {
		t.Fatalf("fresh app: set=%v err=%v", set, err)
	}
	on, off := true, false
	if err := db.SetAppDNSProxied(ctx, "web", &on, "admin"); err != nil {
		t.Fatal(err)
	}
	if v, set, _ := db.GetAppDNSProxied(ctx, "web"); !set || !v {
		t.Fatalf("after on: v=%v set=%v", v, set)
	}
	if err := db.SetAppDNSProxied(ctx, "web", &off, "admin"); err != nil {
		t.Fatal(err)
	}
	if v, set, _ := db.GetAppDNSProxied(ctx, "web"); !set || v {
		t.Fatalf("after off: v=%v set=%v", v, set)
	}
	if err := db.SetAppDNSProxied(ctx, "web", nil, "admin"); err != nil {
		t.Fatal(err)
	}
	if _, set, _ := db.GetAppDNSProxied(ctx, "web"); set {
		t.Fatal("override survived a clear")
	}
}
