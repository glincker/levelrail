package backup

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// A damaged dump must fail the restore and leave the existing data in place.
func TestContainerRestorer_Restore_Postgres_DamagedDumpKeepsData_Live(t *testing.T) {
	rt := liveRuntime(t)
	ctx := context.Background()

	const name = "levelrail-test-restore-damaged"
	removeContainerIfExists(ctx, t, rt, name)
	t.Cleanup(func() { removeContainerIfExists(context.Background(), t, rt, name) })

	id, err := rt.Create(ctx, docker.ContainerSpec{
		Name:  name,
		Image: "postgres:16",
		Env:   map[string]string{"POSTGRES_USER": "leveltest", "POSTGRES_PASSWORD": "leveltestpass"},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := rt.Start(ctx, id); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	waitReady(ctx, t, rt, name, []string{"psql", "-U", "leveltest", "-d", "leveltest", "-c", "SELECT 1"}, 30*time.Second)

	run := func(sql string) string {
		out, err := rt.Exec(ctx, name, []string{"sh", "-c", `psql -U leveltest -d leveltest -At -c "` + sql + `"`})
		if err != nil {
			t.Fatalf("Exec(%q) error = %v", sql, err)
		}
		defer func() { _ = out.Close() }()
		b, _ := io.ReadAll(out)
		return string(b)
	}
	run("CREATE TABLE keepme (val text); INSERT INTO keepme VALUES ('still-here');")

	r := &ContainerRestorer{Runtime: rt}
	damaged := "CREATE TABLE half (i int);\nINSERT INTO half VALUES (1);\nSELECT 1/0;\nCREATE TABLE never (i int);\n"
	if err := r.Restore(ctx, store.EnginePostgres, name, strings.NewReader(damaged)); err == nil {
		t.Fatal("Restore() of a damaged dump returned nil, want an error")
	}
	if got := run("SELECT val FROM keepme"); !strings.Contains(got, "still-here") {
		t.Errorf("existing data after failed restore = %q, want it untouched", got)
	}
	if got := strings.TrimSpace(run("SELECT to_regclass('half')")); got != "" {
		t.Errorf("half-restored table present after failed restore: %q", got)
	}
}
