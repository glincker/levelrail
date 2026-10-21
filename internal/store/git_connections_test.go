package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func newTestConn(t *testing.T, db *DB, provider, host, account, clientID string) GitConnection {
	t.Helper()
	id, err := NewGitConnectionID()
	if err != nil {
		t.Fatal(err)
	}
	c := GitConnection{
		ID: id, Provider: provider, Label: host, Host: host, InstanceURL: "https://" + host, ClientID: clientID,
		SecretsKey: GitConnectionSecretsKey(id), AccountID: account, AccountName: account, CreatedAt: "2026-10-20T10:00:00Z",
	}
	if err := db.CreateGitConnection(t.Context(), c); err != nil {
		t.Fatalf("create: %v", err)
	}
	return c
}

func TestGitConnections_AddsRowsInsteadOfReplacing(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	a := newTestConn(t, db, GitProviderGitLab, "gitlab.com", "1", "app-a")
	b := newTestConn(t, db, GitProviderGitLab, "git.corp.example", "2", "app-b")
	got, err := db.ListGitConnections(ctx, GitProviderGitLab)
	if err != nil || len(got) != 2 {
		t.Fatalf("list = %d rows, err %v; want 2", len(got), err)
	}
	if !got[0].IsDefault || got[1].IsDefault {
		t.Fatalf("first connection must be the default, got %+v", got)
	}
	if other, _ := db.ListGitConnections(ctx, GitProviderGitea); len(other) != 0 {
		t.Fatalf("gitea list should be empty, got %d", len(other))
	}
	if all, _ := db.ListGitConnections(ctx, ""); len(all) != 2 {
		t.Fatalf("all providers = %d, want 2", len(all))
	}
	_ = a
	_ = b
}

func TestGitConnections_FindByAccountMatchesHostAccountAndApp(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	a := newTestConn(t, db, GitProviderGitLab, "gitlab.com", "42", "app-a")
	tests := []struct {
		name                                         string
		provider, host, account, clientID, excludeID string
		want                                         bool
	}{
		{"same account same app", GitProviderGitLab, "GitLab.com", "42", "app-a", "", true},
		{"different account", GitProviderGitLab, "gitlab.com", "43", "app-a", "", false},
		{"different host", GitProviderGitLab, "git.corp.example", "42", "app-a", "", false},
		{"different app keeps accounts apart", GitProviderGitLab, "gitlab.com", "42", "app-b", "", false},
		{"excludes itself", GitProviderGitLab, "gitlab.com", "42", "app-a", a.ID, false},
		{"unknown account never matches", GitProviderGitLab, "gitlab.com", "", "app-a", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, ok, err := db.FindGitConnectionByAccount(ctx, tc.provider, tc.host, tc.account, tc.clientID, tc.excludeID)
			if err != nil || ok != tc.want {
				t.Fatalf("found = %v, err %v; want %v", ok, err, tc.want)
			}
		})
	}
}

func TestGitConnections_DeleteLeavesOthersAndPromotesDefault(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	a := newTestConn(t, db, GitProviderGitea, "git.one.example", "1", "x")
	b := newTestConn(t, db, GitProviderGitea, "git.two.example", "2", "y")
	if err := db.DeleteGitConnection(ctx, a.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	got, err := db.GetGitConnection(ctx, b.ID)
	if err != nil || !got.IsDefault {
		t.Fatalf("remaining connection = %+v, err %v; want promoted default", got, err)
	}
	if _, err := db.GetGitConnection(ctx, a.ID); !errors.Is(err, ErrGitConnectionNotFound) {
		t.Fatalf("get deleted = %v, want ErrGitConnectionNotFound", err)
	}
	if err := db.DeleteGitConnection(ctx, a.ID); !errors.Is(err, ErrGitConnectionNotFound) {
		t.Fatalf("second delete = %v, want ErrGitConnectionNotFound", err)
	}
}

func TestGitConnections_SetDefaultRenameAndAccount(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	a := newTestConn(t, db, GitProviderBitbucket, "bitbucket.org", "u1", "k1")
	b := newTestConn(t, db, GitProviderBitbucket, "bitbucket.org", "u2", "k2")
	if err := db.SetDefaultGitConnection(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.RenameGitConnection(ctx, b.ID, "Work"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetGitConnectionAccount(ctx, b.ID, "u9", "nine", "repository"); err != nil {
		t.Fatal(err)
	}
	gotA, _ := db.GetGitConnection(ctx, a.ID)
	gotB, _ := db.GetGitConnection(ctx, b.ID)
	if gotA.IsDefault || !gotB.IsDefault || gotB.Label != "Work" || gotB.AccountID != "u9" || gotB.Scopes != "repository" {
		t.Fatalf("a = %+v, b = %+v", gotA, gotB)
	}
	if err := db.RenameGitConnection(ctx, "missing", "x"); !errors.Is(err, ErrGitConnectionNotFound) {
		t.Fatalf("rename missing = %v", err)
	}
}

func TestGitSources_RelinkOrphansOnlyOnMatchingHost(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	live := newTestConn(t, db, GitProviderGitLab, "gitlab.com", "1", "a")
	for name, repo := range map[string]string{
		"orphan-same-host":  "https://gitlab.com/g/a.git",
		"orphan-other-host": "https://git.other.example/g/b.git",
		"attached":          "https://gitlab.com/g/c.git",
		"pasted":            "https://gitlab.com/g/d.git",
	} {
		if _, err := db.ExecContext(ctx, `INSERT INTO service_git_sources (service_name, repo_url, branch, build_type) VALUES (?, ?, 'main', 'dockerfile')`, name, repo); err != nil {
			t.Fatal(err)
		}
	}
	for name, conn := range map[string]string{"orphan-same-host": "gc_gone", "orphan-other-host": "gc_gone", "attached": live.ID} {
		if err := db.SetGitSourceConnection(ctx, name, conn); err != nil {
			t.Fatal(err)
		}
	}
	n, err := db.RelinkOrphanedGitSources(ctx, "gitlab.com", live.ID)
	if err != nil || n != 1 {
		t.Fatalf("relinked = %d, err %v; want 1", n, err)
	}
	want := map[string]string{"orphan-same-host": live.ID, "orphan-other-host": "gc_gone", "attached": live.ID, "pasted": ""}
	for name, conn := range want {
		gs, err := db.GetGitSource(ctx, name)
		if err != nil || gs.ConnectionID != conn {
			t.Errorf("%s connection = %q, err %v; want %q", name, gs.ConnectionID, err, conn)
		}
	}
	names, _ := db.ListGitSourcesByConnection(ctx, live.ID)
	if len(names) != 2 {
		t.Errorf("sources on live connection = %v, want 2", names)
	}
}

func TestMigration2430_FoldsSingleRowsIntoConnections(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	old, err := Open(ctx, path, WithMigrationCeiling(2412))
	if err != nil {
		t.Fatalf("open at 2412: %v", err)
	}
	for _, q := range []string{
		`INSERT INTO gitlab_app_connections (id, instance_url, client_id, created_at) VALUES (1, 'https://GitLab.Example.com:8443/base', 'glid', '2026-01-01T00:00:00Z')`,
		`INSERT INTO gitea_app_connections (id, instance_url, client_id, created_at) VALUES (1, 'https://git.home.lan', 'giid', '2026-01-02T00:00:00Z')`,
		`INSERT INTO bitbucket_app_connections (id, oauth_key, created_at) VALUES (1, 'bbkey', '2026-01-03T00:00:00Z')`,
		`INSERT INTO service_git_sources (service_name, repo_url, branch, build_type) VALUES ('on-gitlab', 'https://oauth2:tok@gitlab.example.com:8443/g/p.git', 'main', 'dockerfile')`,
		`INSERT INTO service_git_sources (service_name, repo_url, branch, build_type) VALUES ('on-gitea', 'https://git.home.lan/o/r', 'main', 'dockerfile')`,
		`INSERT INTO service_git_sources (service_name, repo_url, branch, build_type) VALUES ('on-bitbucket', 'https://bitbucket.org/w/r.git', 'main', 'dockerfile')`,
		`INSERT INTO service_git_sources (service_name, repo_url, branch, build_type) VALUES ('on-github', 'https://github.com/o/r.git', 'main', 'dockerfile')`,
		`INSERT INTO git_connection_status (provider, status, detail, next_check_at, updated_at) VALUES ('gitlab', 'needs_reauthorize', 'x', '', '2026-01-01T00:00:00Z')`,
		`INSERT INTO git_connection_status (provider, status, detail, next_check_at, updated_at) VALUES ('github', 'revoked', 'y', '', '2026-01-01T00:00:00Z')`,
		`INSERT INTO git_repo_list_cache (cache_key, provider, payload, fetched_at) VALUES ('gitlab', 'gitlab', '{}', '2026-01-01T00:00:00Z')`,
		`INSERT INTO git_repo_list_cache (cache_key, provider, payload, fetched_at) VALUES ('github:1', 'github', '{}', '2026-01-01T00:00:00Z')`,
	} {
		if _, err := old.ExecContext(ctx, q); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	cur, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	defer func() { _ = cur.Close() }()

	conns, err := cur.ListGitConnections(ctx, "")
	if err != nil || len(conns) != 3 {
		t.Fatalf("connections = %d, err %v; want 3", len(conns), err)
	}
	byProvider := map[string]GitConnection{}
	for _, c := range conns {
		byProvider[c.Provider] = c
	}
	gl, gi, bb := byProvider[GitProviderGitLab], byProvider[GitProviderGitea], byProvider[GitProviderBitbucket]
	if gl.Host != "gitlab.example.com" || gl.ClientID != "glid" || gl.SecretsKey != "gitlab-app/connection" || !gl.IsDefault || gl.InstanceURL != "https://GitLab.Example.com:8443/base" {
		t.Errorf("gitlab connection = %+v", gl)
	}
	if gi.Host != "git.home.lan" || gi.SecretsKey != "gitea-app/connection" {
		t.Errorf("gitea connection = %+v", gi)
	}
	if bb.Host != "bitbucket.org" || bb.ClientID != "bbkey" || bb.SecretsKey != "bitbucket-app/connection" {
		t.Errorf("bitbucket connection = %+v", bb)
	}
	wantConn := map[string]string{"on-gitlab": gl.ID, "on-gitea": gi.ID, "on-bitbucket": bb.ID, "on-github": ""}
	for name, want := range wantConn {
		gs, err := cur.GetGitSource(ctx, name)
		if err != nil || gs.ConnectionID != want {
			t.Errorf("%s connection_id = %q, err %v; want %q", name, gs.ConnectionID, err, want)
		}
	}
	if s, ok, _ := cur.GetGitConnectionStatus(ctx, gl.ID); !ok || s.Status != GitConnNeedsReauthorize {
		t.Errorf("gitlab health was not re-keyed to the connection id: %+v ok=%v", s, ok)
	}
	if _, ok, _ := cur.GetGitConnectionStatus(ctx, "gitlab"); ok {
		t.Error("old provider-keyed health row survived")
	}
	if _, ok, _ := cur.GetGitConnectionStatus(ctx, "github"); !ok {
		t.Error("github health row must be untouched")
	}
	if _, err := cur.GetGitRepoListCache(ctx, "gitlab"); err == nil {
		t.Error("stale provider-keyed repo cache row survived")
	}
	if _, err := cur.GetGitRepoListCache(ctx, "github:1"); err != nil {
		t.Errorf("github cache row lost: %v", err)
	}
}
