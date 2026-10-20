package store

import (
	"context"
	"testing"
)

func TestDomainSearchVisibility(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{
		Name: "web", Image: "img:v1", Port: 80, Domains: []string{"console.example.com", "www.example.com"},
	}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	if hidden, err := db.IsDomainHidden(ctx, "console.example.com"); err != nil || hidden {
		t.Fatalf("IsDomainHidden() = %v, %v, want false (the default is visible)", hidden, err)
	}
	for i := 0; i < 2; i++ {
		if err := db.SetDomainHidden(ctx, "console.example.com", true); err != nil {
			t.Fatalf("SetDomainHidden(true) pass %d error = %v", i, err)
		}
	}
	list, err := db.ListHiddenDomains(ctx)
	if err != nil || len(list) != 1 || list[0] != "console.example.com" {
		t.Fatalf("ListHiddenDomains() = %v, %v, want [console.example.com]", list, err)
	}
	if err := db.SetDomainHidden(ctx, "console.example.com", false); err != nil {
		t.Fatalf("SetDomainHidden(false) error = %v", err)
	}
	if hidden, _ := db.IsDomainHidden(ctx, "console.example.com"); hidden {
		t.Error("domain still hidden after SetDomainHidden(false)")
	}

	if err := db.SetDomainHidden(ctx, "www.example.com", true); err != nil {
		t.Fatalf("SetDomainHidden() error = %v", err)
	}
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v2", Port: 80}); err != nil {
		t.Fatalf("SaveDesiredService() (drop domains) error = %v", err)
	}
	if list, _ := db.ListHiddenDomains(ctx); len(list) != 0 {
		t.Errorf("ListHiddenDomains() = %v, want none: rows cascade with their domain", list)
	}
}
