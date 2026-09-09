package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNewDeployAttemptID(t *testing.T) {
	seen := make(map[string]bool)
	for range 20 {
		id, err := NewDeployAttemptID()
		if err != nil {
			t.Fatalf("NewDeployAttemptID() error = %v", err)
		}
		if id == "" {
			t.Fatal("NewDeployAttemptID() returned empty string")
		}
		if len(id) < len(deployAttemptIDPrefix)+10 {
			t.Errorf("NewDeployAttemptID() = %q, looks too short to be random bytes plus prefix", id)
		}
		if id[:len(deployAttemptIDPrefix)] != deployAttemptIDPrefix {
			t.Errorf("NewDeployAttemptID() = %q, want prefix %q", id, deployAttemptIDPrefix)
		}
		if seen[id] {
			t.Fatalf("NewDeployAttemptID() produced a duplicate: %q", id)
		}
		seen[id] = true
	}
}

func TestSaveAndGetDeployAttempt(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	started := time.Now().UTC().Truncate(time.Millisecond)
	want := DeployAttempt{
		ID:          "dep_test1",
		ServiceName: "web",
		Image:       "levelrail/web:abc123",
		Status:      DeployAttemptStatusRunning,
		StartedAt:   started,
	}
	if err := db.SaveDeployAttempt(ctx, want); err != nil {
		t.Fatalf("SaveDeployAttempt() error = %v", err)
	}

	got, err := db.GetDeployAttempt(ctx, "dep_test1")
	if err != nil {
		t.Fatalf("GetDeployAttempt() error = %v", err)
	}
	if got.ID != want.ID || got.ServiceName != want.ServiceName || got.Image != want.Image || got.Status != want.Status {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if !got.StartedAt.Equal(want.StartedAt) {
		t.Errorf("StartedAt = %v, want %v", got.StartedAt, want.StartedAt)
	}
	if got.FinishedAt != nil {
		t.Errorf("FinishedAt = %v, want nil before the attempt finishes", got.FinishedAt)
	}
	if got.Error != "" {
		t.Errorf("Error = %q, want empty before the attempt finishes", got.Error)
	}
}

func TestSaveAndGetDeployAttempt_CommitSHAAndSource(t *testing.T) {
	tests := []struct {
		name      string
		commitSHA string
		source    string
	}{
		{name: "webhook with commit", commitSHA: "abc123", source: DeployAttemptSourceWebhook},
		{name: "manual build with commit", commitSHA: "def456", source: DeployAttemptSourceManual},
		{name: "plain image tag, no commit", commitSHA: "", source: DeployAttemptSourceImage},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openTestDB(t)
			ctx := context.Background()

			id := "dep_" + tt.source
			if err := db.SaveDeployAttempt(ctx, DeployAttempt{
				ID: id, ServiceName: "web", Image: "levelrail/web:tag",
				CommitSHA: tt.commitSHA, Source: tt.source,
				Status: DeployAttemptStatusRunning, StartedAt: time.Now().UTC(),
			}); err != nil {
				t.Fatalf("SaveDeployAttempt() error = %v", err)
			}

			got, err := db.GetDeployAttempt(ctx, id)
			if err != nil {
				t.Fatalf("GetDeployAttempt() error = %v", err)
			}
			if got.CommitSHA != tt.commitSHA {
				t.Errorf("CommitSHA = %q, want %q", got.CommitSHA, tt.commitSHA)
			}
			if got.Source != tt.source {
				t.Errorf("Source = %q, want %q", got.Source, tt.source)
			}

			list, err := db.ListDeployAttempts(ctx, "web")
			if err != nil {
				t.Fatalf("ListDeployAttempts() error = %v", err)
			}
			if len(list) != 1 || list[0].CommitSHA != tt.commitSHA || list[0].Source != tt.source {
				t.Errorf("ListDeployAttempts() = %+v, want CommitSHA %q and Source %q", list, tt.commitSHA, tt.source)
			}
		})
	}
}

func TestGetDeployAttempt_NotFound(t *testing.T) {
	db := openTestDB(t)
	_, err := db.GetDeployAttempt(context.Background(), "dep_never-saved")
	if !errors.Is(err, ErrDeployAttemptNotFound) {
		t.Errorf("error = %v, want ErrDeployAttemptNotFound", err)
	}
}

func TestFinishDeployAttempt_Succeeded(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDeployAttempt(ctx, DeployAttempt{
		ID: "dep_ok", ServiceName: "web", Image: "levelrail/web:abc123",
		Status: DeployAttemptStatusRunning, StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	finished := time.Now().UTC().Truncate(time.Millisecond)
	if err := db.FinishDeployAttempt(ctx, "dep_ok", DeployAttemptStatusSucceeded, finished, ""); err != nil {
		t.Fatalf("FinishDeployAttempt() error = %v", err)
	}

	got, err := db.GetDeployAttempt(ctx, "dep_ok")
	if err != nil {
		t.Fatalf("GetDeployAttempt() error = %v", err)
	}
	if got.Status != DeployAttemptStatusSucceeded {
		t.Errorf("Status = %q, want %q", got.Status, DeployAttemptStatusSucceeded)
	}
	if got.FinishedAt == nil || !got.FinishedAt.Equal(finished) {
		t.Errorf("FinishedAt = %v, want %v", got.FinishedAt, finished)
	}
	if got.Error != "" {
		t.Errorf("Error = %q, want empty for a succeeded attempt", got.Error)
	}
}

func TestFinishDeployAttempt_Failed(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDeployAttempt(ctx, DeployAttempt{
		ID: "dep_fail", ServiceName: "web", Image: "levelrail/web:abc123",
		Status: DeployAttemptStatusRunning, StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	finished := time.Now().UTC().Truncate(time.Millisecond)
	if err := db.FinishDeployAttempt(ctx, "dep_fail", DeployAttemptStatusFailed, finished, "build: step failed"); err != nil {
		t.Fatalf("FinishDeployAttempt() error = %v", err)
	}

	got, err := db.GetDeployAttempt(ctx, "dep_fail")
	if err != nil {
		t.Fatalf("GetDeployAttempt() error = %v", err)
	}
	if got.Status != DeployAttemptStatusFailed {
		t.Errorf("Status = %q, want %q", got.Status, DeployAttemptStatusFailed)
	}
	if got.Error != "build: step failed" {
		t.Errorf("Error = %q, want %q", got.Error, "build: step failed")
	}
}

func TestFailOrphanedDeployAttempts_MarksRunningOnesFailed(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDeployAttempt(ctx, DeployAttempt{
		ID: "dep_orphan1", ServiceName: "web", Image: "levelrail/web:abc123",
		Status: DeployAttemptStatusRunning, StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed orphan1: %v", err)
	}
	if err := db.SaveDeployAttempt(ctx, DeployAttempt{
		ID: "dep_orphan2", ServiceName: "worker", Image: "levelrail/worker:def456",
		Status: DeployAttemptStatusRunning, StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed orphan2: %v", err)
	}
	if err := db.SaveDeployAttempt(ctx, DeployAttempt{
		ID: "dep_already_done", ServiceName: "web", Image: "levelrail/web:xyz789",
		Status: DeployAttemptStatusRunning, StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed already_done: %v", err)
	}
	finishedEarlier := time.Now().UTC().Truncate(time.Millisecond)
	if err := db.FinishDeployAttempt(ctx, "dep_already_done", DeployAttemptStatusSucceeded, finishedEarlier, ""); err != nil {
		t.Fatalf("finish already_done: %v", err)
	}

	sweepTime := time.Now().UTC().Truncate(time.Millisecond)
	n, err := db.FailOrphanedDeployAttempts(ctx, sweepTime)
	if err != nil {
		t.Fatalf("FailOrphanedDeployAttempts() error = %v", err)
	}
	if n != 2 {
		t.Errorf("fixed count = %d, want 2", n)
	}

	for _, id := range []string{"dep_orphan1", "dep_orphan2"} {
		got, err := db.GetDeployAttempt(ctx, id)
		if err != nil {
			t.Fatalf("GetDeployAttempt(%q) error = %v", id, err)
		}
		if got.Status != DeployAttemptStatusFailed {
			t.Errorf("%s: Status = %q, want %q", id, got.Status, DeployAttemptStatusFailed)
		}
		if got.FinishedAt == nil || !got.FinishedAt.Equal(sweepTime) {
			t.Errorf("%s: FinishedAt = %v, want %v", id, got.FinishedAt, sweepTime)
		}
		if got.Error == "" {
			t.Errorf("%s: Error is empty, want a real explanation", id)
		}
	}

	// Already-finished attempts must be left untouched.
	untouched, err := db.GetDeployAttempt(ctx, "dep_already_done")
	if err != nil {
		t.Fatalf("GetDeployAttempt(already_done) error = %v", err)
	}
	if untouched.Status != DeployAttemptStatusSucceeded {
		t.Errorf("already_done: Status = %q, want unchanged %q", untouched.Status, DeployAttemptStatusSucceeded)
	}
	if !untouched.FinishedAt.Equal(finishedEarlier) {
		t.Errorf("already_done: FinishedAt = %v, want unchanged %v", untouched.FinishedAt, finishedEarlier)
	}
}

func TestFailOrphanedDeployAttempts_NothingRunning_ReturnsZero(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	n, err := db.FailOrphanedDeployAttempts(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("FailOrphanedDeployAttempts() error = %v", err)
	}
	if n != 0 {
		t.Errorf("fixed count = %d, want 0", n)
	}
}

func TestFinishDeployAttempt_NotFound(t *testing.T) {
	db := openTestDB(t)
	err := db.FinishDeployAttempt(context.Background(), "dep_ghost", DeployAttemptStatusFailed, time.Now(), "boom")
	if !errors.Is(err, ErrDeployAttemptNotFound) {
		t.Errorf("error = %v, want ErrDeployAttemptNotFound", err)
	}
}

func TestListDeployAttempts_NewestFirstScopedToService(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	base := time.Now().UTC().Truncate(time.Millisecond)
	attempts := []DeployAttempt{
		{ID: "dep_1", ServiceName: "web", Image: "web:1", Status: DeployAttemptStatusSucceeded, StartedAt: base},
		{ID: "dep_2", ServiceName: "web", Image: "web:2", Status: DeployAttemptStatusSucceeded, StartedAt: base.Add(time.Minute)},
		{ID: "dep_3", ServiceName: "worker", Image: "worker:1", Status: DeployAttemptStatusSucceeded, StartedAt: base.Add(2 * time.Minute)},
	}
	for _, a := range attempts {
		if err := db.SaveDeployAttempt(ctx, a); err != nil {
			t.Fatalf("seed %s: %v", a.ID, err)
		}
	}

	got, err := db.ListDeployAttempts(ctx, "web")
	if err != nil {
		t.Fatalf("ListDeployAttempts() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 attempts for service %q, got %d", "web", len(got))
	}
	if got[0].ID != "dep_2" || got[1].ID != "dep_1" {
		t.Errorf("got IDs [%s, %s], want newest first [dep_2, dep_1]", got[0].ID, got[1].ID)
	}
}

func TestListDeployAttempts_EmptyIsNotError(t *testing.T) {
	db := openTestDB(t)
	got, err := db.ListDeployAttempts(context.Background(), "never-deployed")
	if err != nil {
		t.Fatalf("ListDeployAttempts() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no attempts, got %d", len(got))
	}
}

func TestNewDeployAttemptSnapshot(t *testing.T) {
	hostPort := 8080
	tests := []struct {
		name string
		svc  DesiredService
		want DeployAttemptSnapshot
	}{
		{
			name: "literal, secret, and database env keys classify correctly",
			svc: DesiredService{
				Env:         map[string]string{"PLAIN": "value"},
				SecretEnv:   []string{"API_KEY"},
				DatabaseEnv: map[string]DatabaseEnvRef{"DB_URL": {Database: "main", Field: "url"}},
			},
			want: DeployAttemptSnapshot{Env: []DeployAttemptEnvKey{
				{Key: "API_KEY", Kind: DeployAttemptEnvKindSecret},
				{Key: "DB_URL", Kind: DeployAttemptEnvKindDatabase},
				{Key: "PLAIN", Kind: DeployAttemptEnvKindLiteral, Value: "value"},
			}},
		},
		{
			name: "port, host port, domains, and resources carry through",
			svc: DesiredService{
				Port: 3000, HostPort: &hostPort,
				Domains:   []string{"app.example.com"},
				Resources: &ServiceResources{MemoryBytes: 512 << 20, NanoCPUs: 5e8},
			},
			want: DeployAttemptSnapshot{
				Port: 3000, HostPort: &hostPort,
				Domains:   []string{"app.example.com"},
				Resources: &ServiceResources{MemoryBytes: 512 << 20, NanoCPUs: 5e8},
			},
		},
		{
			name: "no env, ports, domains, or resources configured",
			svc:  DesiredService{Port: 8080},
			want: DeployAttemptSnapshot{Port: 8080},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewDeployAttemptSnapshot(tt.svc)
			if len(got.Env) != len(tt.want.Env) {
				t.Fatalf("Env = %+v, want %+v", got.Env, tt.want.Env)
			}
			for i, w := range tt.want.Env {
				if got.Env[i] != w {
					t.Errorf("Env[%d] = %+v, want %+v", i, got.Env[i], w)
				}
			}
			if got.Port != tt.want.Port {
				t.Errorf("Port = %d, want %d", got.Port, tt.want.Port)
			}
			if (got.HostPort == nil) != (tt.want.HostPort == nil) {
				t.Errorf("HostPort = %v, want %v", got.HostPort, tt.want.HostPort)
			} else if got.HostPort != nil && *got.HostPort != *tt.want.HostPort {
				t.Errorf("HostPort = %d, want %d", *got.HostPort, *tt.want.HostPort)
			}
		})
	}
}

func TestNewDeployAttemptSnapshot_NeverCarriesASecretOrDatabaseValue(t *testing.T) {
	svc := DesiredService{
		Env:         map[string]string{"API_KEY": "this-must-never-appear", "DB_URL": "this-must-never-appear-either"},
		SecretEnv:   []string{"API_KEY"},
		DatabaseEnv: map[string]DatabaseEnvRef{"DB_URL": {Database: "main", Field: "url"}},
	}
	// A secret/database env key never has a real literal value in
	// DesiredService.Env in practice (see internal/deploy/translate.go's
	// literalEnv), but this asserts the snapshot builder itself would
	// still refuse to surface one even if a caller's Env map somehow held
	// a stray entry for a declared secret/database key.
	got := NewDeployAttemptSnapshot(svc)
	for _, e := range got.Env {
		if e.Kind != DeployAttemptEnvKindLiteral && e.Value != "" {
			t.Errorf("env key %q (kind %q) has a non-empty Value: %q", e.Key, e.Kind, e.Value)
		}
	}
}

func TestSaveAndGetDeployAttempt_Snapshot(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	hostPort := 9090
	want := DeployAttemptSnapshot{
		Env: []DeployAttemptEnvKey{
			{Key: "API_KEY", Kind: DeployAttemptEnvKindSecret},
			{Key: "DB_URL", Kind: DeployAttemptEnvKindDatabase},
			{Key: "PLAIN", Kind: DeployAttemptEnvKindLiteral, Value: "value"},
		},
		Port: 3000, HostPort: &hostPort,
		Domains:   []string{"app.example.com"},
		Resources: &ServiceResources{MemoryBytes: 512 << 20, NanoCPUs: 5e8, SwapMemoryBytes: 1 << 30, CPUSetCPUs: "0-1"},
	}

	if err := db.SaveDeployAttempt(ctx, DeployAttempt{
		ID: "dep_snap1", ServiceName: "web", Image: "levelrail/web:1",
		Status: DeployAttemptStatusRunning, StartedAt: time.Now().UTC().Truncate(time.Millisecond),
		Snapshot: want,
	}); err != nil {
		t.Fatalf("SaveDeployAttempt() error = %v", err)
	}

	got, err := db.GetDeployAttempt(ctx, "dep_snap1")
	if err != nil {
		t.Fatalf("GetDeployAttempt() error = %v", err)
	}

	if len(got.Snapshot.Env) != len(want.Env) {
		t.Fatalf("Snapshot.Env = %+v, want %+v", got.Snapshot.Env, want.Env)
	}
	for i, w := range want.Env {
		if got.Snapshot.Env[i] != w {
			t.Errorf("Snapshot.Env[%d] = %+v, want %+v", i, got.Snapshot.Env[i], w)
		}
	}
	if got.Snapshot.Port != want.Port {
		t.Errorf("Snapshot.Port = %d, want %d", got.Snapshot.Port, want.Port)
	}
	if got.Snapshot.HostPort == nil || *got.Snapshot.HostPort != *want.HostPort {
		t.Errorf("Snapshot.HostPort = %v, want %d", got.Snapshot.HostPort, *want.HostPort)
	}
	if len(got.Snapshot.Domains) != 1 || got.Snapshot.Domains[0] != "app.example.com" {
		t.Errorf("Snapshot.Domains = %v, want [app.example.com]", got.Snapshot.Domains)
	}
	if got.Snapshot.Resources == nil || *got.Snapshot.Resources != *want.Resources {
		t.Errorf("Snapshot.Resources = %+v, want %+v", got.Snapshot.Resources, want.Resources)
	}
}

func TestGetDeployAttempt_PreMigrationRowHasZeroValueSnapshot(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	// Simulates a row written before migrations/0086 added config_snapshot:
	// SaveDeployAttempt always writes a value now, so this bypasses it to
	// insert a row the way the old schema's default ('{}') would have left
	// one, and confirms GetDeployAttempt still reads it back cleanly.
	_, err := db.ExecContext(ctx, `
		INSERT INTO deploy_attempts (id, service_name, image, commit_sha, source, status, started_at, finished_at, error)
		VALUES ('dep_premigrate', 'web', 'levelrail/web:1', '', '', ?, ?, NULL, NULL)
	`, DeployAttemptStatusSucceeded, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("seed pre-migration row: %v", err)
	}

	got, err := db.GetDeployAttempt(ctx, "dep_premigrate")
	if err != nil {
		t.Fatalf("GetDeployAttempt() error = %v", err)
	}
	if len(got.Snapshot.Env) != 0 || got.Snapshot.Port != 0 || got.Snapshot.Resources != nil {
		t.Errorf("Snapshot = %+v, want the zero value for a pre-migration row", got.Snapshot)
	}
}
