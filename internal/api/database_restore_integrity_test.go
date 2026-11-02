package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/GLINCKER/levelrail/internal/quota"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestBranchAndRestoreAsNew_RefuseBackupOfDeletedSameNameDatabase(t *testing.T) {
	t.Parallel()
	rt, db, _ := newBranchRouter(t, nil)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatal(err)
	}
	seedSucceededBackupForCloneRestore(t, db, "main")
	if err := db.DeleteDesiredDatabase(ctx, "main"); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatal(err)
	}

	code, body := quotaDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/branches", `{"name":"b1","backup_id":"bkh_1"}`)
	if code != http.StatusConflict || !strings.Contains(body, "earlier database") {
		t.Errorf("branch = %d %s, want 409 naming the earlier database", code, body)
	}
	code, body = quotaDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/restore-as-new", `{"backup_id":"bkh_1","new_name":"r1"}`)
	if code != http.StatusConflict {
		t.Errorf("restore-as-new = %d %s, want 409", code, body)
	}
	for _, n := range []string{"b1", "r1"} {
		if _, err := db.GetDesiredDatabase(ctx, n); err == nil {
			t.Errorf("database %q was created despite the refusal", n)
		}
	}
}

func TestBranch_SameInstanceBackupStillAllowed(t *testing.T) {
	t.Parallel()
	rt, db, runner := newBranchRouter(t, nil)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatal(err)
	}
	seedSucceededBackupForCloneRestore(t, db, "main")
	if code, body := quotaDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/branches", `{"name":"b1","backup_id":"bkh_1"}`); code != http.StatusAccepted {
		t.Fatalf("branch = %d %s, want 202", code, body)
	}
	runner.awaitCall(t)
}

func TestBranch_LegacyBackupWithoutInstanceMatchesByCreationTime(t *testing.T) {
	t.Parallel()
	rt, db, runner := newBranchRouter(t, nil)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	// Backups written before the database exists carry no instance id, like rows from before the migration.
	seedSucceededBackupForCloneRestore(t, db, "main")
	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatal(err)
	}
	// The database was created after the 2026-08-14 backup, so the backup is not its own.
	if code, body := quotaDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/branches", `{"name":"b1","backup_id":"bkh_1"}`); code != http.StatusConflict {
		t.Fatalf("legacy backup older than database = %d %s, want 409", code, body)
	}
	// A legacy row at or after the database's creation is accepted.
	if _, err := db.ExecContext(ctx, `UPDATE desired_databases SET created_at = '2026-01-01T00:00:00Z' WHERE name = 'main'`); err != nil {
		t.Fatal(err)
	}
	if code, body := quotaDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/branches", `{"name":"b2","backup_id":"bkh_1"}`); code != http.StatusAccepted {
		t.Fatalf("legacy backup newer than database = %d %s, want 202", code, body)
	}
	runner.awaitCall(t)
}

func TestBranchAtTime_IgnoresBaseBackupsOfDeletedSameNameDatabase(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openTestDB(t)
	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "m", Engine: store.EngineMySQL, Version: "8"}); err != nil {
		t.Fatal(err)
	}
	seedBaseBackup(t, db, "bkt_x", "m", "base_old", "2026-08-14T00:00:00Z")
	if err := db.DeleteDesiredDatabase(ctx, "m"); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "m", Engine: store.EngineMySQL, Version: "8"}); err != nil {
		t.Fatal(err)
	}
	cur, _ := db.GetDesiredDatabase(ctx, "m")
	bases, _ := db.ListBaseBackupHistory(ctx, "m")
	if len(bases) != 1 || bases[0].InstanceID == "" || bases[0].InstanceID == cur.InstanceID {
		t.Fatalf("base row instance = %+v, current = %q, want a different non-empty id", bases, cur.InstanceID)
	}
	if cur.OwnsBackup(bases[0].InstanceID, bases[0].StartedAt) {
		t.Error("current database owns the deleted database's base backup")
	}
}

func quotaMemoryProject(t *testing.T, db *store.DB, memory int64) string {
	t.Helper()
	const proj = "proj_dbq"
	if err := db.SaveProject(context.Background(), store.Project{ID: proj, Name: "dbq", CreatedAt: "2026-10-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetQuota(context.Background(), quota.ScopeProject, proj, quota.Amounts{MemoryBytes: memory}); err != nil {
		t.Fatal(err)
	}
	return proj
}

func TestBranchAndRestoreAsNew_ResourcesRunQuotaCheck(t *testing.T) {
	t.Parallel()
	rt, db, _ := newBranchRouter(t, nil)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	proj := quotaMemoryProject(t, db, 1<<30)
	// The source already sits inside the limit; a copy of it would double the memory in use.
	big := &store.ServiceResources{MemoryBytes: 768 << 20}
	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16", Resources: big}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateDatabaseProject(ctx, "main", proj); err != nil {
		t.Fatal(err)
	}
	seedSucceededBackupForCloneRestore(t, db, "main")

	if code, body := quotaDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/branches", `{"name":"b1","backup_id":"bkh_1"}`); code != http.StatusConflict {
		t.Errorf("branch over quota = %d %s, want 409", code, body)
	}
	restoreBody := fmt.Sprintf(`{"backup_id":"bkh_1","new_name":"r1","project_id":%q,"resources":{"memory_bytes":%d}}`, proj, 768<<20)
	if code, body := quotaDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/restore-as-new", restoreBody); code != http.StatusConflict {
		t.Errorf("restore-as-new over quota = %d %s, want 409", code, body)
	}
	for _, n := range []string{"b1", "r1"} {
		if _, err := db.GetDesiredDatabase(ctx, n); err == nil {
			t.Errorf("database %q exists after a quota refusal", n)
		}
	}
}

func TestCreateDatabase_ConcurrentSameNameOnlyOneWins(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	const n = 8
	codes := make([]int, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes[i], _ = quotaDo(t, rt, cookie, http.MethodPost, "/api/v1/databases", `{"name":"race","engine":"postgres","version":"16"}`)
		}()
	}
	wg.Wait()
	created, conflicts := 0, 0
	for _, c := range codes {
		switch c {
		case http.StatusCreated:
			created++
		case http.StatusConflict:
			conflicts++
		}
	}
	if created != 1 || conflicts != n-1 {
		t.Errorf("codes = %v, want exactly one 201 and %d 409", codes, n-1)
	}
}
