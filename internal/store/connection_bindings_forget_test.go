package store

import (
	"context"
	"errors"
	"testing"
)

func TestBindingRowsFollowTheirAppOrgAndConnection(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "nginx:1", Port: 80}))
	must(db.SaveOrganization(ctx, Organization{ID: "org_x", Name: "x", CreatedAt: "2026-10-01T00:00:00Z"}))
	must(db.SaveRegistryCredential(ctx, RegistryCredential{ID: "rc1", Name: "hub", RegistryHost: "r.example.com", CreatedAt: "2026-01-01T00:00:00Z"}))
	for _, b := range []ConnectionBinding{
		{Ref: "registry:rc1", TargetType: BindingTargetApp, TargetID: "web", Role: "registry_pull"},
		{Ref: "registry:rc2", TargetType: BindingTargetOrg, TargetID: "org_x", Role: "registry_pull"},
		{Ref: "registry:rc1", TargetType: BindingTargetApp, TargetID: "api", Role: "registry_pull"},
	} {
		_, err := db.CreateConnectionBinding(ctx, b)
		must(err)
	}
	must(db.UpsertConnectionMeta(ctx, ConnectionMeta{Ref: "secret:s1", ScopeType: ConnectionScopeApp, ScopeID: "web"}))
	must(db.UpsertConnectionMeta(ctx, ConnectionMeta{Ref: "secret:s2", ScopeType: ConnectionScopeOrg, ScopeID: "org_x"}))
	must(db.UpsertConnectionMeta(ctx, ConnectionMeta{Ref: "registry:rc1", ScopeType: ConnectionScopeInstance}))

	must(db.DeleteDesiredService(ctx, "web"))
	if rows, _ := db.ListConnectionBindingsByTarget(ctx, BindingTargetApp, "web"); len(rows) != 0 {
		t.Errorf("app bindings after app delete = %d, want 0", len(rows))
	}
	if m, err := db.GetConnectionMeta(ctx, "secret:s1"); err != nil || m.ScopeType != ConnectionScopeApp || m.ScopeID != "" {
		t.Errorf("app scoped meta after app delete = %+v, %v, want app scope with no id", m, err)
	}

	must(db.DeleteOrganization(ctx, "org_x"))
	if rows, _ := db.ListConnectionBindingsByTarget(ctx, BindingTargetOrg, "org_x"); len(rows) != 0 {
		t.Errorf("org bindings after org delete = %d, want 0", len(rows))
	}
	if m, err := db.GetConnectionMeta(ctx, "secret:s2"); err != nil || m.ScopeID != "" {
		t.Errorf("org scoped meta after org delete = %+v, %v, want no id", m, err)
	}

	must(db.DeleteRegistryCredential(ctx, "rc1"))
	if rows, _ := db.ListConnectionBindingsByRef(ctx, "registry:rc1"); len(rows) != 0 {
		t.Errorf("bindings after connection delete = %d, want 0", len(rows))
	}
	if _, err := db.GetConnectionMeta(ctx, "registry:rc1"); !errors.Is(err, ErrConnectionMetaNotFound) {
		t.Errorf("meta after connection delete err = %v, want not found", err)
	}
	if err := db.DeleteDesiredService(ctx, "web"); !errors.Is(err, ErrServiceNotFound) {
		t.Errorf("second delete err = %v, want not found", err)
	}
}
