package store

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

func seedTeamUser(t *testing.T, db *DB, id, team, role string, abilities []string) {
	t.Helper()
	if err := db.CreateUser(context.Background(), User{ID: id, Email: id + "@x.test", DisplayName: id, Abilities: abilities, CreatedAt: time.Now(), TeamID: team, TeamRole: role}); err != nil {
		t.Fatalf("CreateUser(%s): %v", id, err)
	}
}

func TestCreateUser_JoinsDefaultTeamByAbilities(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	tests := []struct {
		id        string
		abilities []string
		want      string
	}{
		{"root", []string{"root"}, TeamRoleOwner},
		{"deployer", []string{"read", "deploy"}, TeamRoleAdmin},
		{"writer", []string{"read", "write"}, TeamRoleDeveloper},
		{"reader", []string{"read"}, TeamRoleViewer},
	}
	for _, tc := range tests {
		seedTeamUser(t, db, tc.id, "", "", tc.abilities)
		if role, err := db.TeamMemberRole(ctx, DefaultTeamID, tc.id); err != nil || role != tc.want {
			t.Errorf("%s default team role = %q, %v; want %q", tc.id, role, err, tc.want)
		}
		u, _ := db.GetUserByID(ctx, tc.id)
		if !slices.Equal(u.Abilities, tc.abilities) {
			t.Errorf("%s abilities changed on join: %v", tc.id, u.Abilities)
		}
	}
}

func TestTeamMembership_LastOwnerProtection(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveOrganization(ctx, Organization{ID: "org_t", Name: "t", CreatedAt: "2026-10-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	seedTeamUser(t, db, "o1", "org_t", TeamRoleOwner, TeamRoleAbilities(TeamRoleOwner))
	seedTeamUser(t, db, "d1", "org_t", TeamRoleDeveloper, TeamRoleAbilities(TeamRoleDeveloper))

	if err := db.RemoveTeamMember(ctx, "org_t", "o1"); !errors.Is(err, ErrLastTeamOwner) {
		t.Errorf("remove last owner = %v, want ErrLastTeamOwner", err)
	}
	if err := db.SetTeamMemberRole(ctx, "org_t", "o1", TeamRoleAdmin); !errors.Is(err, ErrLastTeamOwner) {
		t.Errorf("demote last owner = %v, want ErrLastTeamOwner", err)
	}
	if err := db.TransferTeamOwnership(ctx, "org_t", "d1", "o1"); !errors.Is(err, ErrTeamMemberNotFound) {
		t.Errorf("transfer from a non-owner = %v, want refused", err)
	}
	if err := db.TransferTeamOwnership(ctx, "org_t", "o1", "d1"); err != nil {
		t.Fatalf("transfer = %v", err)
	}
	if role, _ := db.TeamMemberRole(ctx, "org_t", "d1"); role != TeamRoleOwner {
		t.Errorf("new owner role = %q", role)
	}
	if err := db.RemoveTeamMember(ctx, "org_t", "o1"); err != nil {
		t.Errorf("remove former owner = %v", err)
	}
	u, _ := db.GetUserByID(ctx, "o1")
	if len(u.Abilities) != 0 {
		t.Errorf("tenant with no teams keeps abilities %v, want none", u.Abilities)
	}
}

func TestTeamInvites_ClaimOnce(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now()
	inv := TeamInvite{ID: "i1", TeamID: DefaultTeamID, Role: TeamRoleViewer, TokenHash: "h1", CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	if err := db.CreateTeamInvite(ctx, inv); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateTeamInvite(ctx, TeamInvite{ID: "i2", TeamID: DefaultTeamID, Role: TeamRoleOwner, TokenHash: "h2", CreatedAt: now, ExpiresAt: now.Add(time.Hour)}); !errors.Is(err, ErrInvalidTeamRole) {
		t.Errorf("owner invite = %v, want ErrInvalidTeamRole", err)
	}
	if _, err := db.ClaimTeamInvite(ctx, "h1", "u1", now); err != nil {
		t.Fatalf("first claim = %v", err)
	}
	if _, err := db.ClaimTeamInvite(ctx, "h1", "u2", now); !errors.Is(err, ErrTeamInviteInvalid) {
		t.Errorf("replay = %v, want ErrTeamInviteInvalid", err)
	}
	expired := TeamInvite{ID: "i3", TeamID: DefaultTeamID, Role: TeamRoleViewer, TokenHash: "h3", CreatedAt: now, ExpiresAt: now.Add(time.Minute)}
	if err := db.CreateTeamInvite(ctx, expired); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ClaimTeamInvite(ctx, "h3", "u3", now.Add(2*time.Minute)); !errors.Is(err, ErrTeamInviteInvalid) {
		t.Errorf("expired claim = %v, want ErrTeamInviteInvalid", err)
	}
}

func TestResourceOwner_DefaultsToDefaultTeam(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "loose", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	o, err := db.OwnerOfApp(ctx, "loose")
	if err != nil || o == nil || o.TeamID != DefaultTeamID || o.ProjectID != "" {
		t.Errorf("owner of project-less app = %+v, %v; want default team", o, err)
	}
	if o, _ := db.OwnerOfApp(ctx, "missing"); o != nil {
		t.Errorf("owner of missing app = %+v, want nil", o)
	}
}
