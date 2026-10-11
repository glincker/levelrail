package store

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestMigration0442_BackfillsDefaultTeam replays 0442's data statements over
// a pre-teams shaped database: every project lands in the default team and
// every existing user joins with a role matching their abilities.
func TestMigration0442_BackfillsDefaultTeam(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	for _, u := range []User{
		{ID: "legacy_root", Email: "r@x.test", DisplayName: "r", Abilities: []string{"root"}},
		{ID: "legacy_op", Email: "o@x.test", DisplayName: "o", Abilities: []string{"read", "write", "deploy"}},
		{ID: "legacy_ro", Email: "v@x.test", DisplayName: "v", Abilities: []string{"read", "read:sensitive"}},
	} {
		u.CreatedAt = time.Now()
		if err := db.CreateUser(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SaveProject(ctx, Project{ID: "legacy_proj", Name: "p", CreatedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`DELETE FROM team_members`, `UPDATE projects SET org_id = NULL`} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	src, err := migrationsFS.ReadFile("migrations/0442_teams.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range strings.Split(string(src), ";") {
		var kept []string
		for _, line := range strings.Split(stmt, "\n") {
			if !strings.HasPrefix(strings.TrimSpace(line), "--") {
				kept = append(kept, line)
			}
		}
		body := strings.TrimSpace(strings.Join(kept, "\n"))
		if body == "" || strings.HasPrefix(body, "CREATE") {
			continue
		}
		if _, err := db.ExecContext(ctx, body); err != nil {
			t.Fatalf("replay %q: %v", body, err)
		}
	}
	if team, err := db.TeamOfProject(ctx, "legacy_proj"); err != nil || team != DefaultTeamID {
		t.Errorf("legacy project team = %q, %v", team, err)
	}
	for id, want := range map[string]string{"legacy_root": TeamRoleOwner, "legacy_op": TeamRoleAdmin, "legacy_ro": TeamRoleViewer} {
		if role, err := db.TeamMemberRole(ctx, DefaultTeamID, id); err != nil || role != want {
			t.Errorf("%s role = %q, %v; want %q", id, role, err, want)
		}
	}
}
