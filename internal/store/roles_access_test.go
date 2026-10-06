package store

import (
	"context"
	"slices"
	"testing"
)

func TestRoleAccessHelpers(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedUser(t, db, "u_root")
	seedUser(t, db, "u_qa")
	if err := db.CreateRole(ctx, Role{ID: "role_qa", Name: "qa", Abilities: []string{"read"}, Visibility: RoleVisibilityAll}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetUserRole(ctx, "u_root", "role_admin"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetUserRole(ctx, "u_qa", "role_qa"); err != nil {
		t.Fatal(err)
	}

	if n, err := db.CountRootUsersExcept(ctx, "u_root"); err != nil || n != 0 {
		t.Errorf("CountRootUsersExcept(u_root) = %d, %v, want 0", n, err)
	}
	if n, err := db.CountRootUsersExcept(ctx, "u_qa"); err != nil || n != 1 {
		t.Errorf("CountRootUsersExcept(u_qa) = %d, %v, want 1", n, err)
	}
	if n, err := db.CountRootUsersOutsideRole(ctx, "role_admin"); err != nil || n != 0 {
		t.Errorf("CountRootUsersOutsideRole(admin) = %d, %v, want 0", n, err)
	}
	if n, err := db.CountRootUsersOutsideRole(ctx, "role_qa"); err != nil || n != 1 {
		t.Errorf("CountRootUsersOutsideRole(qa) = %d, %v, want 1", n, err)
	}

	if err := db.UpdateRole(ctx, Role{ID: "role_qa", Name: "qa", Abilities: []string{"read", "deploy"}, Visibility: RoleVisibilityAll}); err != nil {
		t.Fatal(err)
	}
	if err := db.SyncRoleAbilities(ctx, "role_qa"); err != nil {
		t.Fatal(err)
	}
	u, err := db.GetUserByID(ctx, "u_qa")
	if err != nil || !slices.Equal(u.Abilities, []string{"read", "deploy"}) {
		t.Errorf("abilities after sync = %v, %v", u.Abilities, err)
	}
	r, err := db.GetUserByID(ctx, "u_root")
	if err != nil || !slices.Equal(r.Abilities, []string{"root"}) {
		t.Errorf("unrelated user changed: %v, %v", r.Abilities, err)
	}
}
