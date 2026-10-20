package store

import (
	"context"
	"testing"
)

func TestServiceTemplateDefaults_SaveGetClearAndCascade(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "oc", Image: "img:1", Port: 80}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	d := ServiceTemplateDefaults{ServiceName: "oc", TemplateID: "openclaw", Strategy: "recreate", StrategyReason: "lock", MemoryBytes: 1 << 30}
	if err := db.SaveServiceTemplateDefaults(ctx, d); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := db.GetServiceTemplateDefaults(ctx, "oc")
	if err != nil || got == nil || *got != d {
		t.Fatalf("Get = %+v, %v, want %+v", got, err, d)
	}

	if err := db.SaveServiceTemplateDefaults(ctx, ServiceTemplateDefaults{ServiceName: "oc"}); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if got, _ := db.GetServiceTemplateDefaults(ctx, "oc"); got != nil {
		t.Errorf("row survived an empty save: %+v", got)
	}

	if err := db.SaveServiceTemplateDefaults(ctx, d); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := db.DeleteDesiredService(ctx, "oc"); err != nil {
		t.Fatalf("delete service: %v", err)
	}
	if got, _ := db.GetServiceTemplateDefaults(ctx, "oc"); got != nil {
		t.Errorf("row survived its service: %+v", got)
	}
}
