package store

import (
	"context"
	"testing"
)

func TestAppContextConfigRoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	save := AppContextConfig{AppName: "web", Context: "deploy-preview", Env: map[string]string{"A": "1"}, BuildCommand: "make", PublishDir: "dist", BuildArgs: map[string]string{"N": "20"}}
	if err := db.SaveAppContextConfig(ctx, save); err != nil {
		t.Fatalf("save: %v", err)
	}
	save.Env = map[string]string{"A": "2"}
	if err := db.SaveAppContextConfig(ctx, save); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := db.SaveAppContextConfig(ctx, AppContextConfig{AppName: "web", Context: "branch:dev/*"}); err != nil {
		t.Fatalf("save second: %v", err)
	}

	rows, err := db.ListAppContextConfigs(ctx, "web")
	if err != nil || len(rows) != 2 {
		t.Fatalf("list = %v, %v", rows, err)
	}
	if rows[1].Context != "deploy-preview" || rows[1].Env["A"] != "2" || rows[1].BuildArgs["N"] != "20" || rows[1].PublishDir != "dist" {
		t.Fatalf("row = %+v", rows[1])
	}

	found, err := db.DeleteAppContextConfig(ctx, "web", "deploy-preview")
	if err != nil || !found {
		t.Fatalf("delete = %v, %v", found, err)
	}
	if found, _ := db.DeleteAppContextConfig(ctx, "web", "deploy-preview"); found {
		t.Fatal("second delete should report not found")
	}
}
