package store

import (
	"context"
	"testing"
)

// TestGetIngressSettings_Default proves migrations/0023's own seeded row
// round-trips into the Go zero-value shape a fresh, never-configured
// control plane should see: ACME off, nothing set. This is the
// regression this whole feature exists to never break: an operator who
// never visits the settings page must see identical behavior to before
// this table existed.
func TestGetIngressSettings_Default(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	got, err := db.GetIngressSettings(ctx)
	if err != nil {
		t.Fatalf("GetIngressSettings() error = %v", err)
	}
	want := IngressSettings{}
	if got != want {
		t.Errorf("GetIngressSettings() = %+v, want zero value %+v", got, want)
	}
}

func TestUpdateAndGetIngressSettings_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := IngressSettings{
		PrimaryDomain:    "dashboard.example.com",
		ACMEEnabled:      true,
		ACMEEmail:        "ops@example.com",
		ACMEDirectoryURL: "https://acme-staging-v02.api.letsencrypt.org/directory",
	}
	if err := db.UpdateIngressSettings(ctx, want); err != nil {
		t.Fatalf("UpdateIngressSettings() error = %v", err)
	}

	got, err := db.GetIngressSettings(ctx)
	if err != nil {
		t.Fatalf("GetIngressSettings() error = %v", err)
	}
	if got != want {
		t.Errorf("GetIngressSettings() = %+v, want %+v", got, want)
	}
}

// TestUpdateIngressSettings_ClearsFields proves an update can move
// settings back toward the empty/disabled state, not just set them: a
// row that once had a primary domain and ACME enabled can have both
// cleared by a subsequent update, and the cleared fields must read back
// as empty/false, not as stale leftover values.
func TestUpdateIngressSettings_ClearsFields(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.UpdateIngressSettings(ctx, IngressSettings{
		PrimaryDomain: "dashboard.example.com",
		ACMEEnabled:   true,
		ACMEEmail:     "ops@example.com",
	}); err != nil {
		t.Fatalf("UpdateIngressSettings(set) error = %v", err)
	}

	if err := db.UpdateIngressSettings(ctx, IngressSettings{}); err != nil {
		t.Fatalf("UpdateIngressSettings(clear) error = %v", err)
	}

	got, err := db.GetIngressSettings(ctx)
	if err != nil {
		t.Fatalf("GetIngressSettings() error = %v", err)
	}
	if got != (IngressSettings{}) {
		t.Errorf("GetIngressSettings() after clear = %+v, want zero value", got)
	}
}

func TestUpdateIngressSettings_MultipleUpdatesStaySingleRow(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.UpdateIngressSettings(ctx, IngressSettings{PrimaryDomain: "one.example.com"}); err != nil {
		t.Fatalf("UpdateIngressSettings(1) error = %v", err)
	}
	if err := db.UpdateIngressSettings(ctx, IngressSettings{PrimaryDomain: "two.example.com"}); err != nil {
		t.Fatalf("UpdateIngressSettings(2) error = %v", err)
	}

	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM ingress_settings`).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("ingress_settings row count = %d, want 1 (singleton table)", count)
	}

	got, err := db.GetIngressSettings(ctx)
	if err != nil {
		t.Fatalf("GetIngressSettings() error = %v", err)
	}
	if got.PrimaryDomain != "two.example.com" {
		t.Errorf("PrimaryDomain = %q, want the latest update to win", got.PrimaryDomain)
	}
}

func TestListServiceDomains(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Domains: []string{"web.example.com", "www.example.com"},
	}); err != nil {
		t.Fatalf("SaveDesiredService(web) error = %v", err)
	}
	if err := db.SaveDesiredService(ctx, DesiredService{
		Name: "api", Image: "img:v2", Port: 8080,
		Domains: []string{"api.example.com"},
	}); err != nil {
		t.Fatalf("SaveDesiredService(api) error = %v", err)
	}

	got, err := db.ListServiceDomains(ctx)
	if err != nil {
		t.Fatalf("ListServiceDomains() error = %v", err)
	}
	want := []ServiceDomain{
		{Domain: "api.example.com", ServiceName: "api"},
		{Domain: "web.example.com", ServiceName: "web"},
		{Domain: "www.example.com", ServiceName: "web"},
	}
	if len(got) != len(want) {
		t.Fatalf("ListServiceDomains() = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ListServiceDomains()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestListServiceDomains_Empty(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	got, err := db.ListServiceDomains(ctx)
	if err != nil {
		t.Fatalf("ListServiceDomains() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ListServiceDomains() = %+v, want empty", got)
	}
}
