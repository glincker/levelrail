package store

import (
	"context"
	"testing"
)

func TestPatchServiceEnvKeepsOtherFields(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "x:1", Port: 80, Env: map[string]string{"A": "1", "B": "2"}, Domains: []string{"a.com"}}); err != nil {
		t.Fatal(err)
	}
	if err := db.PatchServiceEnv(ctx, "web", map[string]string{"B": "3"}); err != nil {
		t.Fatal(err)
	}
	svc, _ := db.GetDesiredService(ctx, "web")
	if svc.Env["A"] != "1" || svc.Env["B"] != "3" || len(svc.Domains) != 1 || svc.Image != "x:1" {
		t.Fatalf("stored = %+v", svc)
	}
	if err := db.PatchServiceEnv(ctx, "missing", nil); err == nil {
		t.Fatal("missing service must error")
	}
}
