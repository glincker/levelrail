package backup

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// TestContainerRestorer_Restore_Postgres_Live is the strongest proof
// available that a Postgres restore actually restores, run against a
// real container: seed a marker row, dump it (ContainerDumper, already
// proven live by dump_live_test.go), overwrite that same table with
// different data to prove a real change happened, restore from the
// captured dump (ContainerRestorer, this file), then query the live
// database directly and check the original marker is back and the
// overwriting change is gone. Not a mock or a fake anywhere in the
// path between the dump bytes and the container psql actually runs
// against.
//
// This also exercises the drop-and-replace decision postgresRestoreCmd's
// own doc comment describes end to end: the overwritten data has to be
// genuinely gone afterward, not merged with the restored rows, for this
// test to pass, which a merge-shaped restore would fail.
func TestContainerRestorer_Restore_Postgres_Live(t *testing.T) {
	rt := liveRuntime(t)
	ctx := context.Background()

	const name = "levelrail-test-restore-postgres"
	removeContainerIfExists(ctx, t, rt, name)
	t.Cleanup(func() { removeContainerIfExists(context.Background(), t, rt, name) })

	id, err := rt.Create(ctx, docker.ContainerSpec{
		Name:  name,
		Image: "postgres:16",
		Env: map[string]string{
			"POSTGRES_USER":     "leveltest",
			"POSTGRES_PASSWORD": "leveltestpass",
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := rt.Start(ctx, id); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	waitReady(ctx, t, rt, name, []string{"psql", "-U", "leveltest", "-d", "leveltest", "-c", "SELECT 1"}, 30*time.Second)

	originalMarker := "levelrail-restore-live-original-6f1a"
	seed, err := rt.Exec(ctx, name, []string{"sh", "-c",
		`psql -U leveltest -d leveltest -c "CREATE TABLE restore_probe (val text); INSERT INTO restore_probe VALUES ('` + originalMarker + `');"`,
	})
	if err != nil {
		t.Fatalf("Exec() seed error = %v", err)
	}
	if _, err := io.Copy(io.Discard, seed); err != nil {
		t.Fatalf("seeding original data: %v", err)
	}
	_ = seed.Close()

	// Capture the dump while the original data is still the only data
	// present, exactly the artifact a real backup would have produced at
	// this point.
	d := &ContainerDumper{Runtime: rt}
	dumpStream, err := d.Dump(ctx, store.EnginePostgres, name)
	if err != nil {
		t.Fatalf("Dump() error = %v", err)
	}
	var dumpBuf bytes.Buffer
	if _, err := io.Copy(&dumpBuf, dumpStream); err != nil {
		t.Fatalf("reading Dump() stream error = %v", err)
	}
	_ = dumpStream.Close()
	if !strings.Contains(dumpBuf.String(), originalMarker) {
		t.Fatalf("captured dump does not contain the original marker %q, test setup is broken", originalMarker)
	}

	// Overwrite: drop the seeded row and table, replace with something
	// else entirely, proving a real, distinguishable change happened
	// before restore runs.
	overwriteMarker := "levelrail-restore-live-overwritten-9c3d"
	overwrite, err := rt.Exec(ctx, name, []string{"sh", "-c",
		`psql -U leveltest -d leveltest -c "DROP TABLE restore_probe; CREATE TABLE unrelated_table (val text); INSERT INTO unrelated_table VALUES ('` + overwriteMarker + `');"`,
	})
	if err != nil {
		t.Fatalf("Exec() overwrite error = %v", err)
	}
	if _, err := io.Copy(io.Discard, overwrite); err != nil {
		t.Fatalf("overwriting data: %v", err)
	}
	_ = overwrite.Close()

	// Restore from the captured dump.
	r := &ContainerRestorer{Runtime: rt}
	if err := r.Restore(ctx, store.EnginePostgres, name, bytes.NewReader(dumpBuf.Bytes())); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	// Verify directly against the live database, not against the dump
	// bytes: restore_probe (with the original marker) must be back, and
	// unrelated_table (the overwrite) must be gone, proving drop-and-
	// replace, not a merge that would have left both tables present.
	check, err := rt.Exec(ctx, name, []string{"sh", "-c", `psql -U leveltest -d leveltest -t -c "SELECT val FROM restore_probe;"`})
	if err != nil {
		t.Fatalf("Exec() post-restore check error = %v", err)
	}
	checkOut, err := io.ReadAll(check)
	_ = check.Close()
	if err != nil {
		t.Fatalf("reading post-restore check: %v", err)
	}
	if !strings.Contains(string(checkOut), originalMarker) {
		t.Errorf("restore_probe after restore = %q, want it to contain the original marker %q", checkOut, originalMarker)
	}

	goneCheck, err := rt.Exec(ctx, name, []string{"sh", "-c", `psql -U leveltest -d leveltest -t -c "SELECT to_regclass('unrelated_table');"`})
	if err != nil {
		t.Fatalf("Exec() overwrite-gone check error = %v", err)
	}
	goneOut, err := io.ReadAll(goneCheck)
	_ = goneCheck.Close()
	if err != nil {
		t.Fatalf("reading overwrite-gone check: %v", err)
	}
	if strings.Contains(string(goneOut), "unrelated_table") {
		t.Errorf("unrelated_table still exists after restore (output %q), want the overwrite fully replaced, not merged", goneOut)
	}
}

// TestContainerRestorer_Restore_MySQL_Live is
// TestContainerRestorer_Restore_Postgres_Live's exact MySQL counterpart:
// real container, real dump, a real overwriting change, a real restore,
// and a direct query against the live database afterward proving the
// original data is back. Unlike Postgres, mysqlRestoreCmd adds no
// explicit drop/recreate step (see its own doc comment): this test is
// what actually proves mysqldump's own default DROP TABLE IF EXISTS
// behavior is sufficient on its own, not just a claim taken on faith
// from reading mysqldump's documentation.
func TestContainerRestorer_Restore_MySQL_Live(t *testing.T) {
	rt := liveRuntime(t)
	ctx := context.Background()

	const name = "levelrail-test-restore-mysql"
	removeContainerIfExists(ctx, t, rt, name)
	t.Cleanup(func() { removeContainerIfExists(context.Background(), t, rt, name) })

	id, err := rt.Create(ctx, docker.ContainerSpec{
		Name:  name,
		Image: "mysql:8",
		Env: map[string]string{
			"MYSQL_ROOT_PASSWORD": "leveltestroot",
			"MYSQL_DATABASE":      "leveltest",
			"MYSQL_USER":          "leveltest",
			"MYSQL_PASSWORD":      "leveltestpass",
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := rt.Start(ctx, id); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	waitReady(ctx, t, rt, name, []string{"sh", "-c", `mysql -uroot -p"$MYSQL_ROOT_PASSWORD" leveltest -e "SELECT 1"`}, 60*time.Second)

	originalMarker := "levelrail-restore-live-original-mysql-2e7b"
	seed, err := rt.Exec(ctx, name, []string{"sh", "-c",
		`mysql -uroot -p"$MYSQL_ROOT_PASSWORD" leveltest -e "CREATE TABLE restore_probe (val varchar(128)); INSERT INTO restore_probe VALUES ('` + originalMarker + `');"`,
	})
	if err != nil {
		t.Fatalf("Exec() seed error = %v", err)
	}
	if _, err := io.Copy(io.Discard, seed); err != nil {
		t.Fatalf("seeding original data: %v", err)
	}
	_ = seed.Close()

	d := &ContainerDumper{Runtime: rt}
	dumpStream, err := d.Dump(ctx, store.EngineMySQL, name)
	if err != nil {
		t.Fatalf("Dump() error = %v", err)
	}
	var dumpBuf bytes.Buffer
	if _, err := io.Copy(&dumpBuf, dumpStream); err != nil {
		t.Fatalf("reading Dump() stream error = %v", err)
	}
	_ = dumpStream.Close()
	if !strings.Contains(dumpBuf.String(), originalMarker) {
		t.Fatalf("captured dump does not contain the original marker %q, test setup is broken", originalMarker)
	}

	overwriteMarker := "levelrail-restore-live-overwritten-mysql-8a4f"
	overwrite, err := rt.Exec(ctx, name, []string{"sh", "-c",
		`mysql -uroot -p"$MYSQL_ROOT_PASSWORD" leveltest -e "DROP TABLE restore_probe; CREATE TABLE unrelated_table (val varchar(128)); INSERT INTO unrelated_table VALUES ('` + overwriteMarker + `');"`,
	})
	if err != nil {
		t.Fatalf("Exec() overwrite error = %v", err)
	}
	if _, err := io.Copy(io.Discard, overwrite); err != nil {
		t.Fatalf("overwriting data: %v", err)
	}
	_ = overwrite.Close()

	r := &ContainerRestorer{Runtime: rt}
	if err := r.Restore(ctx, store.EngineMySQL, name, bytes.NewReader(dumpBuf.Bytes())); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	check, err := rt.Exec(ctx, name, []string{"sh", "-c", `mysql -uroot -p"$MYSQL_ROOT_PASSWORD" leveltest -N -e "SELECT val FROM restore_probe;"`})
	if err != nil {
		t.Fatalf("Exec() post-restore check error = %v", err)
	}
	checkOut, err := io.ReadAll(check)
	_ = check.Close()
	if err != nil {
		t.Fatalf("reading post-restore check: %v", err)
	}
	if !strings.Contains(string(checkOut), originalMarker) {
		t.Errorf("restore_probe after restore = %q, want it to contain the original marker %q", checkOut, originalMarker)
	}

	goneCheck, err := rt.Exec(ctx, name, []string{"sh", "-c", `mysql -uroot -p"$MYSQL_ROOT_PASSWORD" leveltest -N -e "SHOW TABLES LIKE 'unrelated_table';"`})
	if err != nil {
		t.Fatalf("Exec() overwrite-gone check error = %v", err)
	}
	goneOut, err := io.ReadAll(goneCheck)
	_ = goneCheck.Close()
	if err != nil {
		t.Fatalf("reading overwrite-gone check: %v", err)
	}
	if strings.Contains(string(goneOut), "unrelated_table") {
		t.Errorf("unrelated_table still exists after restore (output %q), want the overwrite fully replaced", goneOut)
	}
}
