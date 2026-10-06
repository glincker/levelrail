package store

import (
	"context"
	"slices"
	"testing"
)

func TestUpdateRoleAndSync_RollsBackTheRoleWhenUsersCannotBeSynced(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedUser(t, db, "u1")
	if err := db.CreateRole(ctx, Role{ID: "role_qa", Name: "qa", Abilities: []string{"read", "deploy"}, Visibility: RoleVisibilityAll}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetUserRole(ctx, "u1", "role_qa"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TRIGGER block_user_updates BEFORE UPDATE ON users BEGIN SELECT RAISE(ABORT, 'boom'); END`); err != nil {
		t.Fatal(err)
	}

	err := db.UpdateRoleAndSync(ctx, Role{ID: "role_qa", Name: "qa", Abilities: []string{"read"}, Visibility: RoleVisibilityAll})
	if err == nil {
		t.Fatal("UpdateRoleAndSync() succeeded although users could not be updated")
	}
	role, _ := db.GetRole(ctx, "role_qa")
	if !slices.Equal(role.Abilities, []string{"read", "deploy"}) {
		t.Errorf("role abilities after a failed sync = %v, want the old set restored (rollback)", role.Abilities)
	}
}

func TestUpdateRoleAndSync_DowngradeReachesEveryUser(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedUser(t, db, "u1")
	seedUser(t, db, "u2")
	if err := db.CreateRole(ctx, Role{ID: "role_qa", Name: "qa", Abilities: []string{"read", "deploy"}, Visibility: RoleVisibilityAll}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"u1", "u2"} {
		if err := db.SetUserRole(ctx, id, "role_qa"); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.UpdateRoleAndSync(ctx, Role{ID: "role_qa", Name: "qa", Abilities: []string{"read"}, Visibility: RoleVisibilityAll}); err != nil {
		t.Fatalf("UpdateRoleAndSync() error = %v", err)
	}
	for _, id := range []string{"u1", "u2"} {
		u, _ := db.GetUserByID(ctx, id)
		if !slices.Equal(u.Abilities, []string{"read"}) {
			t.Errorf("user %s abilities = %v, want the downgraded set", id, u.Abilities)
		}
	}
	if err := db.UpdateRoleAndSync(ctx, Role{ID: "role_admin", Name: "admin", Abilities: []string{"read"}, Visibility: RoleVisibilityAll}); err != ErrRoleBuiltin {
		t.Errorf("built-in update error = %v, want ErrRoleBuiltin", err)
	}
}

func TestUpdateUserAbilities_ClearsTheRoleLabel(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedUser(t, db, "u1")
	if err := db.SetUserRole(ctx, "u1", "role_admin"); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateUserAbilities(ctx, "u1", []string{"read"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.UserRoleID(ctx, "u1"); got != "" {
		t.Errorf("role after hand-editing abilities = %q, want none: the abilities no longer match it", got)
	}
}
