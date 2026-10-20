package store

import (
	"context"
	"errors"
	"slices"
	"testing"
)

func seedUser(t *testing.T, db *DB, id string) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(),
		`INSERT INTO users (id, email, display_name, created_at) VALUES (?, ?, ?, '2026-10-01T00:00:00Z')`,
		id, id+"@example.com", id); err != nil {
		t.Fatalf("seed user: %v", err)
	}
}

func TestRoles_BuiltinsListFirstAndAreProtected(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.CreateRole(ctx, Role{ID: "role_qa", Name: "qa", Abilities: []string{"read", "deploy"}, Visibility: RoleVisibilityAll}); err != nil {
		t.Fatalf("CreateRole() error = %v", err)
	}
	roles, err := db.ListRoles(ctx)
	if err != nil {
		t.Fatalf("ListRoles() error = %v", err)
	}
	if len(roles) != 5 || !roles[0].Builtin || roles[4].ID != "role_qa" || roles[4].Builtin {
		t.Fatalf("ListRoles() = %+v, want 4 built-ins then the custom role", roles)
	}

	if err := db.CreateRole(ctx, Role{ID: "role_qa2", Name: "qa", Abilities: []string{"read"}, Visibility: RoleVisibilityAll}); !errors.Is(err, ErrRoleNameTaken) {
		t.Errorf("duplicate name error = %v, want ErrRoleNameTaken", err)
	}
	if err := db.UpdateRole(ctx, Role{ID: "role_guest", Name: "guest", Abilities: []string{"root"}, Visibility: RoleVisibilityAll}); !errors.Is(err, ErrRoleBuiltin) {
		t.Errorf("update built-in error = %v, want ErrRoleBuiltin", err)
	}
	if err := db.DeleteRole(ctx, "role_admin"); !errors.Is(err, ErrRoleBuiltin) {
		t.Errorf("delete built-in error = %v, want ErrRoleBuiltin", err)
	}
	if err := db.DeleteRole(ctx, "role_missing"); !errors.Is(err, ErrRoleNotFound) {
		t.Errorf("delete missing error = %v, want ErrRoleNotFound", err)
	}
}

func TestRoles_AssigningARoleCopiesItsAbilitiesAndBlocksDeletion(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedUser(t, db, "u1")
	if err := db.CreateRole(ctx, Role{ID: "role_qa", Name: "qa", Abilities: []string{"read", "deploy"}, Visibility: RoleVisibilityAll}); err != nil {
		t.Fatal(err)
	}

	if err := db.SetUserRole(ctx, "u1", "role_qa"); err != nil {
		t.Fatalf("SetUserRole() error = %v", err)
	}
	u, err := db.GetUserByID(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(u.Abilities, []string{"read", "deploy"}) {
		t.Errorf("user abilities = %v, want the role's abilities copied", u.Abilities)
	}
	if got, _ := db.UserRoleID(ctx, "u1"); got != "role_qa" {
		t.Errorf("UserRoleID() = %q, want role_qa", got)
	}
	if err := db.DeleteRole(ctx, "role_qa"); !errors.Is(err, ErrRoleInUse) {
		t.Errorf("delete in-use role error = %v, want ErrRoleInUse", err)
	}
	if err := db.SetUserRole(ctx, "nobody", "role_qa"); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("unknown user error = %v, want ErrUserNotFound", err)
	}
	if err := db.SetUserRole(ctx, "u1", "role_missing"); !errors.Is(err, ErrRoleNotFound) {
		t.Errorf("unknown role error = %v, want ErrRoleNotFound", err)
	}
}

func TestVisibility_GuestSeesOnlyGrantedEnvironments(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedUser(t, db, "guest")
	seedUser(t, db, "ops")
	if err := db.SetUserRole(ctx, "guest", "role_guest"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetUserRole(ctx, "ops", "role_operator"); err != nil {
		t.Fatal(err)
	}

	v, err := db.UserVisibility(ctx, "guest")
	if err != nil || !v.Restricted || v.Allows("env_dev") {
		t.Fatalf("guest with no grants: %+v (err %v), want restricted and seeing nothing", v, err)
	}
	if err := db.SetUserEnvironmentGrants(ctx, "guest", []string{"env_dev", "env_dev", "env_test"}); err != nil {
		t.Fatalf("SetUserEnvironmentGrants() error = %v", err)
	}
	v, _ = db.UserVisibility(ctx, "guest")
	if !v.Allows("env_dev") || !v.Allows("env_test") || v.Allows("env_production") || v.Allows("") {
		t.Errorf("guest visibility = %+v, want only the granted environments and never untagged resources", v)
	}
	if err := db.SetUserEnvironmentGrants(ctx, "guest", []string{"env_uat"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.ListUserEnvironmentGrants(ctx, "guest"); !slices.Equal(got, []string{"env_uat"}) {
		t.Errorf("grants after replace = %v, want only env_uat", got)
	}
	if err := db.SetUserEnvironmentGrants(ctx, "guest", []string{"env_nope"}); err == nil {
		t.Error("granting an unknown environment must fail")
	}
	if got, _ := db.ListUserEnvironmentGrants(ctx, "guest"); !slices.Equal(got, []string{"env_uat"}) {
		t.Errorf("a failed replace must leave the old grants, got %v", got)
	}

	ov, err := db.UserVisibility(ctx, "ops")
	if err != nil || ov.Restricted || !ov.Allows("anything") || !ov.Allows("") {
		t.Errorf("operator visibility = %+v (err %v), want unrestricted", ov, err)
	}
}

func TestEnvironmentOf_ResolvesTaggedServicesAndDatabases(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	if ref, err := db.EnvironmentOfApp(ctx, "web"); err != nil || ref != nil {
		t.Fatalf("untagged app = %+v (err %v), want nil", ref, err)
	}
	if err := db.SetServiceEnvironment(ctx, "web", "env_production"); err != nil {
		t.Fatal(err)
	}
	ref, err := db.EnvironmentOfApp(ctx, "web")
	if err != nil || ref == nil || ref.Kind != "production" || !ref.Protected || ref.Scope != "global" {
		t.Errorf("tagged app = %+v (err %v), want the protected global production environment", ref, err)
	}
	if ref, _ := db.EnvironmentOfApp(ctx, "missing"); ref != nil {
		t.Errorf("unknown app = %+v, want nil", ref)
	}

	if _, err := db.ExecContext(ctx, `INSERT INTO desired_databases (name, engine, version, updated_at, environment_id) VALUES ('pg', 'postgres', '16', '2026-10-01T00:00:00Z', 'env_dev')`); err != nil {
		t.Fatalf("seed database: %v", err)
	}
	if ref, err := db.EnvironmentOfDatabase(ctx, "pg"); err != nil || ref == nil || ref.Kind != "dev" {
		t.Errorf("tagged database = %+v (err %v), want the dev environment", ref, err)
	}
}
